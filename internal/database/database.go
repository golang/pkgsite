// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package database adds some useful functionality to a sql.DB.
// It is independent of the database driver and the
// DB schema.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// DB wraps a sql.DB. The methods it exports correspond closely to those of
// sql.DB. They enhance the original by requiring a context argument, and by
// logging the query and any resulting errors.
//
// A DB may represent a transaction. If so, its execution and query methods
// operate within the transaction.
type DB struct {
	db         *sql.DB
	instanceID string
	tx         *sql.Tx
	conn       *sql.Conn     // the Conn of the Tx, when tx != nil
	opts       sql.TxOptions // valid when tx != nil
	mu         sync.Mutex
	maxRetries int // max times a single transaction was retried
}

// Open creates a new DB  for the given connection string.
func Open(driverName, dbinfo, instanceID string) (_ *DB, err error) {
	defer derrors.Wrap(&err, "database.Open(%q, %q)",
		driverName, redactPassword(dbinfo))

	db, err := sql.Open(driverName, dbinfo)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return New(db, instanceID), nil
}

// New creates a new DB from a sql.DB.
func New(db *sql.DB, instanceID string) *DB {
	return &DB{db: db, instanceID: instanceID}
}

func (db *DB) Ping() error {
	return db.db.Ping()
}

func (db *DB) InTransaction() bool {
	return db.tx != nil
}

func (db *DB) IsRetryable() bool {
	return db.tx != nil && isRetryable(db.opts.Isolation)
}

var passwordRegexp = regexp.MustCompile(`password=\S+`)

func redactPassword(dbinfo string) string {
	return passwordRegexp.ReplaceAllLiteralString(dbinfo, "password=REDACTED")
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.db.Close()
}

// Exec executes a SQL statement and returns the number of rows it affected.
func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (_ int64, err error) {
	defer logQuery(ctx, query, args, db.instanceID, db.IsRetryable())(&err)
	res, err := db.execResult(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("RowsAffected: %v", err)
	}
	return n, nil
}

// execResult executes a SQL statement and returns a sql.Result.
func (db *DB) execResult(ctx context.Context, query string, args ...interface{}) (res sql.Result, err error) {
	if db.tx != nil {
		return db.tx.ExecContext(ctx, query, args...)
	}
	return db.db.ExecContext(ctx, query, args...)
}

// Query runs the DB query.
func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (_ *sql.Rows, err error) {
	defer logQuery(ctx, query, args, db.instanceID, db.IsRetryable())(&err)
	if db.tx != nil {
		return db.tx.QueryContext(ctx, query, args...)
	}
	return db.db.QueryContext(ctx, query, args...)
}

// QueryRow runs the query and returns a single row.
func (db *DB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	defer logQuery(ctx, query, args, db.instanceID, db.IsRetryable())(nil)
	start := time.Now()
	defer func() {
		if ctx.Err() != nil {
			d, _ := ctx.Deadline()
			msg := fmt.Sprintf("args=%v; elapsed=%q, start=%q, deadline=%q", args, time.Since(start), start, d)
			log.Errorf(ctx, "QueryRow context error: %v "+msg, ctx.Err())
		}
	}()
	if db.tx != nil {
		return db.tx.QueryRowContext(ctx, query, args...)
	}
	return db.db.QueryRowContext(ctx, query, args...)
}

func (db *DB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	defer logQuery(ctx, "preparing "+query, nil, db.instanceID, db.IsRetryable())
	if db.tx != nil {
		return db.tx.PrepareContext(ctx, query)
	}
	return db.db.PrepareContext(ctx, query)
}

// RunQuery executes query, then calls f on each row.
func (db *DB) RunQuery(ctx context.Context, query string, f func(*sql.Rows) error, params ...interface{}) error {
	rows, err := db.Query(ctx, query, params...)
	if err != nil {
		return err
	}
	return processRows(rows, f)
}

func processRows(rows *sql.Rows, f func(*sql.Rows) error) error {
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Transact executes the given function in the context of a SQL transaction at
// the given isolation level, rolling back the transaction if the function
// panics or returns an error.
//
// The given function is called with a DB that is associated with a transaction.
// The DB should be used only inside the function; if it is used to access the
// database after the function returns, the calls will return errors.
//
// If the isolation level requires it, Transact will retry the transaction upon
// serialization failure, so txFunc may be called more than once.
func (db *DB) Transact(ctx context.Context, iso sql.IsolationLevel, txFunc func(*DB) error) (err error) {
	defer derrors.Wrap(&err, "Transact(%s)", iso)
	// For the levels which require retry, see
	// https://www.postgresql.org/docs/11/transaction-iso.html.
	opts := &sql.TxOptions{Isolation: iso}
	if isRetryable(iso) {
		return db.transactWithRetry(ctx, opts, txFunc)
	}
	return db.transact(ctx, opts, txFunc)
}

func isRetryable(iso sql.IsolationLevel) bool {
	return iso == sql.LevelRepeatableRead || iso == sql.LevelSerializable
}

// serializationFailureCode is the Postgres error code returned when a serializable
// transaction fails because it would violate serializability.
// See https://www.postgresql.org/docs/current/errcodes-appendix.html.
const serializationFailureCode = "40001"

func (db *DB) transactWithRetry(ctx context.Context, opts *sql.TxOptions, txFunc func(*DB) error) (err error) {
	defer derrors.Wrap(&err, "transactWithRetry(%v)", opts)
	// Retry on serialization failure, up to some max.
	// See https://www.postgresql.org/docs/11/transaction-iso.html.
	const maxRetries = 30
	for i := 0; i <= maxRetries; i++ {
		err = db.transact(ctx, opts, txFunc)
		if isSerializationFailure(err) {
			db.mu.Lock()
			if i > db.maxRetries {
				db.maxRetries = i
			}
			db.mu.Unlock()
			log.Debugf(ctx, "serialization failure; retrying")
			continue
		}
		if err != nil {
			log.Debugf(ctx, "transactWithRetry: error type %T: %[1]v", err)
			if strings.Contains(err.Error(), serializationFailureCode) {
				return fmt.Errorf("error text has %q but not recognized as serialization failure: type %T, err %v",
					serializationFailureCode, err, err)
			}
		}
		if i > 0 {
			log.Debugf(ctx, "retried serializable transaction %d time(s)", i)
		}
		return err
	}
	return fmt.Errorf("reached max number of tries due to serialization failure (%d)", maxRetries)
}

func isSerializationFailure(err error) bool {
	// The underlying error type depends on the driver. Try both pq and pgx types.
	var perr *pq.Error
	if errors.As(err, &perr) && perr.Code == serializationFailureCode {
		return true
	}
	var gerr *pgconn.PgError
	if errors.As(err, &gerr) && gerr.Code == serializationFailureCode {
		return true
	}
	return false
}

func (db *DB) transact(ctx context.Context, opts *sql.TxOptions, txFunc func(*DB) error) (err error) {
	if db.InTransaction() {
		return errors.New("a DB Transact function was called on a DB already in a transaction")
	}
	conn, err := db.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	tx, err := conn.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("conn.BeginTx(): %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			if txErr := tx.Commit(); txErr != nil {
				err = fmt.Errorf("tx.Commit(): %w", txErr)
			}
		}
	}()

	dbtx := New(db.db, db.instanceID)
	dbtx.tx = tx
	dbtx.conn = conn
	dbtx.opts = *opts
	defer dbtx.logTransaction(ctx)(&err)
	if err := txFunc(dbtx); err != nil {
		return fmt.Errorf("txFunc(tx): %w", err)
	}
	return nil
}

// MaxRetries returns the maximum number of times thata  serializable transaction was retried.
func (db *DB) MaxRetries() int {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.maxRetries
}

const OnConflictDoNothing = "ON CONFLICT DO NOTHING"

// BulkInsert constructs and executes a multi-value insert statement. The
// query is constructed using the format:
//   INSERT INTO <table> (<columns>) VALUES (<placeholders-for-each-item-in-values>)
// If conflictAction is not empty, it is appended to the statement.
//
// The query is executed using a PREPARE statement with the provided values.
func (db *DB) BulkInsert(ctx context.Context, table string, columns []string, values []interface{}, conflictAction string) (err error) {
	defer derrors.Wrap(&err, "DB.BulkInsert(ctx, %q, %v, [%d values], %q)",
		table, columns, len(values), conflictAction)

	return db.bulkInsert(ctx, table, columns, nil, values, conflictAction, nil)
}

// BulkInsertReturning is like BulkInsert, but supports returning values from the INSERT statement.
// In addition to the arguments of BulkInsert, it takes a list of columns to return and a function
// to scan those columns. To get the returned values, provide a function that scans them as if
// they were the selected columns of a query. See TestBulkInsert for an example.
func (db *DB) BulkInsertReturning(ctx context.Context, table string, columns []string, values []interface{}, conflictAction string, returningColumns []string, scanFunc func(*sql.Rows) error) (err error) {
	defer derrors.Wrap(&err, "DB.BulkInsertReturning(ctx, %q, %v, [%d values], %q, %v, scanFunc)",
		table, columns, len(values), conflictAction, returningColumns)

	if returningColumns == nil || scanFunc == nil {
		return errors.New("need returningColumns and scan function")
	}
	return db.bulkInsert(ctx, table, columns, returningColumns, values, conflictAction, scanFunc)
}

// BulkUpsert is like BulkInsert, but instead of a conflict action, a list of
// conflicting columns is provided. An "ON CONFLICT (conflict_columns) DO
// UPDATE" clause is added to the statement, with assignments "c=excluded.c" for
// every column c.
func (db *DB) BulkUpsert(ctx context.Context, table string, columns []string, values []interface{}, conflictColumns []string) error {
	conflictAction := buildUpsertConflictAction(columns, conflictColumns)
	return db.BulkInsert(ctx, table, columns, values, conflictAction)
}

// BulkUpsertReturning is like BulkInsertReturning, but performs an upsert like BulkUpsert.
func (db *DB) BulkUpsertReturning(ctx context.Context, table string, columns []string, values []interface{}, conflictColumns, returningColumns []string, scanFunc func(*sql.Rows) error) error {
	conflictAction := buildUpsertConflictAction(columns, conflictColumns)
	return db.BulkInsertReturning(ctx, table, columns, values, conflictAction, returningColumns, scanFunc)
}

func (db *DB) bulkInsert(ctx context.Context, table string, columns, returningColumns []string, values []interface{}, conflictAction string, scanFunc func(*sql.Rows) error) (err error) {
	if remainder := len(values) % len(columns); remainder != 0 {
		return fmt.Errorf("modulus of len(values) and len(columns) must be 0: got %d", remainder)
	}

	// Postgres supports up to 65535 parameters, but stop well before that
	// so we don't construct humongous queries.
	const maxParameters = 1000
	stride := (maxParameters / len(columns)) * len(columns)
	if stride == 0 {
		// This is a pathological case (len(columns) > maxParameters), but we
		// handle it cautiously.
		return fmt.Errorf("too many columns to insert: %d", len(columns))
	}

	prepare := func(n int) (*sql.Stmt, error) {
		return db.Prepare(ctx, buildInsertQuery(table, columns, returningColumns, n, conflictAction))
	}

	var stmt *sql.Stmt
	for leftBound := 0; leftBound < len(values); leftBound += stride {
		rightBound := leftBound + stride
		if rightBound <= len(values) && stmt == nil {
			stmt, err = prepare(stride)
			if err != nil {
				return err
			}
			defer stmt.Close()
		} else if rightBound > len(values) {
			rightBound = len(values)
			stmt, err = prepare(rightBound - leftBound)
			if err != nil {
				return err
			}
			defer stmt.Close()
		}
		valueSlice := values[leftBound:rightBound]
		var err error
		if returningColumns == nil {
			_, err = stmt.ExecContext(ctx, valueSlice...)
		} else {
			var rows *sql.Rows
			rows, err = stmt.QueryContext(ctx, valueSlice...)
			if err != nil {
				return err
			}
			err = processRows(rows, scanFunc)
		}
		if err != nil {
			return fmt.Errorf("running bulk insert query, values[%d:%d]): %w", leftBound, rightBound, err)
		}
	}
	return nil
}

// buildInsertQuery builds an multi-value insert query, following the format:
// INSERT TO <table> (<columns>) VALUES (<placeholders-for-each-item-in-values>) <conflictAction>
// If returningColumns is not empty, it appends a RETURNING clause to the query.
//
// When calling buildInsertQuery, it must be true that nvalues % len(columns) == 0.
func buildInsertQuery(table string, columns, returningColumns []string, nvalues int, conflictAction string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s", table)
	fmt.Fprintf(&b, "(%s) VALUES", strings.Join(columns, ", "))

	var placeholders []string
	for i := 1; i <= nvalues; i++ {
		// Construct the full query by adding placeholders for each
		// set of values that we want to insert.
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		if i%len(columns) != 0 {
			continue
		}

		// When the end of a set is reached, write it to the query
		// builder and reset placeholders.
		fmt.Fprintf(&b, "(%s)", strings.Join(placeholders, ", "))
		placeholders = nil

		// Do not add a comma delimiter after the last set of values.
		if i == nvalues {
			break
		}
		b.WriteString(", ")
	}
	if conflictAction != "" {
		b.WriteString(" " + conflictAction)
	}
	if len(returningColumns) > 0 {
		fmt.Fprintf(&b, " RETURNING %s", strings.Join(returningColumns, ", "))
	}
	return b.String()
}

func buildUpsertConflictAction(columns, conflictColumns []string) string {
	var sets []string
	for _, c := range columns {
		sets = append(sets, fmt.Sprintf("%s=excluded.%[1]s", c))
	}
	return fmt.Sprintf("ON CONFLICT (%s) DO UPDATE SET %s",
		strings.Join(conflictColumns, ", "),
		strings.Join(sets, ", "))
}

// maxBulkUpdateArrayLen is the maximum size of an array that BulkUpdate will send to
// Postgres. (Postgres has no size limit on arrays, but we want to keep the statements
// to a reasonable size.)
// It is a variable for testing.
var maxBulkUpdateArrayLen = 10000

// BulkUpdate executes multiple UPDATE statements in a transaction.
//
// Columns must contain the names of some of table's columns. The first is treated
// as a key; that is, the values to update are matched with existing rows by comparing
// the values of the first column.
//
// Types holds the database type of each column. For example,
//    []string{"INT", "TEXT"}
//
// Values contains one slice of values per column. (Note that this is unlike BulkInsert, which
// takes a single slice of interleaved values.)
func (db *DB) BulkUpdate(ctx context.Context, table string, columns, types []string, values [][]interface{}) (err error) {
	defer derrors.Wrap(&err, "DB.BulkUpdate(ctx, tx, %q, %v, [%d values])",
		table, columns, len(values))

	if len(columns) < 2 {
		return errors.New("need at least two columns")
	}
	if len(columns) != len(values) {
		return errors.New("len(values) != len(columns)")
	}
	nRows := len(values[0])
	for _, v := range values[1:] {
		if len(v) != nRows {
			return errors.New("all values slices must be the same length")
		}
	}
	query := buildBulkUpdateQuery(table, columns, types)
	for left := 0; left < nRows; left += maxBulkUpdateArrayLen {
		right := left + maxBulkUpdateArrayLen
		if right > nRows {
			right = nRows
		}
		var args []interface{}
		for _, vs := range values {
			args = append(args, pq.Array(vs[left:right]))
		}
		if _, err := db.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("db.Exec(%q, values[%d:%d]): %w", query, left, right, err)
		}
	}
	return nil
}

func buildBulkUpdateQuery(table string, columns, types []string) string {
	var sets, unnests []string
	// Build "c = data.c" for each non-key column.
	for _, c := range columns[1:] {
		sets = append(sets, fmt.Sprintf("%s = data.%[1]s", c))
	}
	// Build "UNNEST($1::TYPE) AS c" for each column.
	// We need the type, or Postgres complains that UNNEST is not unique.
	for i, c := range columns {
		unnests = append(unnests, fmt.Sprintf("UNNEST($%d::%s[]) AS %s", i+1, types[i], c))
	}
	return fmt.Sprintf(`
		UPDATE %[1]s
		SET %[2]s
		FROM (SELECT %[3]s) AS data
		WHERE %[1]s.%[4]s = data.%[4]s`,
		table,                       // 1
		strings.Join(sets, ", "),    // 2
		strings.Join(unnests, ", "), // 3
		columns[0],                  // 4
	)
}

// emptyStringScanner wraps the functionality of sql.NullString to just write
// an empty string if the value is NULL.
type emptyStringScanner struct {
	ptr *string
}

func (e emptyStringScanner) Scan(value interface{}) error {
	var ns sql.NullString
	if err := ns.Scan(value); err != nil {
		return err
	}
	*e.ptr = ns.String
	return nil
}

// NullIsEmpty returns a sql.Scanner that writes the empty string to s if the
// sql.Value is NULL.
func NullIsEmpty(s *string) sql.Scanner {
	return emptyStringScanner{s}
}
