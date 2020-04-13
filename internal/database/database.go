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
	"sync/atomic"
	"time"
	"unicode"

	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
)

// DB wraps a sql.DB. The methods it exports correspond closely to those of
// sql.DB. They enhance the original by requiring a context argument, and by
// logging the query and any resulting errors.
//
// A DB may represent a transaction. If so, its execution and query methods
// operate within the transaction.
type DB struct {
	db *sql.DB
	tx *sql.Tx
}

// Open creates a new DB  for the given connection string.
func Open(driverName, dbinfo string) (_ *DB, err error) {
	defer derrors.Wrap(&err, "database.Open(%q, %q)",
		driverName, redactPassword(dbinfo))

	db, err := sql.Open(driverName, dbinfo)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return New(db), nil
}

// New creates a new DB from a sql.DB.
func New(db *sql.DB) *DB {
	return &DB{db: db}
}

func (db *DB) InTransaction() bool {
	return db.tx != nil
}

var passwordRegexp = regexp.MustCompile(`password=\S+`)

func redactPassword(dbinfo string) string {
	return passwordRegexp.ReplaceAllLiteralString(dbinfo, "password=REDACTED")
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.db.Close()
}

// Exec executes a SQL statement.
func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (res sql.Result, err error) {
	defer logQuery(ctx, query, args)(&err)

	if db.tx != nil {
		return db.tx.ExecContext(ctx, query, args...)
	}
	return db.db.ExecContext(ctx, query, args...)
}

// Query runs the DB query.
func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (_ *sql.Rows, err error) {
	defer logQuery(ctx, query, args)(&err)
	if db.tx != nil {
		return db.tx.QueryContext(ctx, query, args...)
	}
	return db.db.QueryContext(ctx, query, args...)
}

// QueryRow runs the query and returns a single row.
func (db *DB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	defer logQuery(ctx, query, args)(nil)
	if db.tx != nil {
		return db.tx.QueryRowContext(ctx, query, args...)
	}
	return db.db.QueryRowContext(ctx, query, args...)
}

// RunQuery executes query, then calls f on each row.
func (db *DB) RunQuery(ctx context.Context, query string, f func(*sql.Rows) error, params ...interface{}) error {
	rows, err := db.Query(ctx, query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Transact executes the given function in the context of a SQL transaction,
// rolling back the transaction if the function panics or returns an error.
//
// The given function is called with a DB that is associated with a transaction.
// The DB should be used only inside the function; if it is used to access the
// database after the function returns, the calls will return errors.
func (db *DB) Transact(txFunc func(*DB) error) (err error) {
	if db.InTransaction() {
		return errors.New("DB.Transact called on a DB already in a transaction")
	}
	tx, err := db.db.Begin()
	if err != nil {
		return fmt.Errorf("db.Begin(): %v", err)
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			if err = tx.Commit(); err != nil {
				err = fmt.Errorf("tx.Commit(): %v", err)
			}
		}
	}()

	dbtx := New(db.db)
	dbtx.tx = tx
	if err := txFunc(dbtx); err != nil {
		return fmt.Errorf("txFunc(tx): %v", err)
	}
	return nil
}

const OnConflictDoNothing = "ON CONFLICT DO NOTHING"

// BulkInsert constructs and executes a multi-value insert statement. The
// query is constructed using the format: INSERT TO <table> (<columns>) VALUES
// (<placeholders-for-each-item-in-values>) If conflictNoAction is true, it
// append ON CONFLICT DO NOTHING to the end of the query. The query is executed
// using a PREPARE statement with the provided values.
func (db *DB) BulkInsert(ctx context.Context, table string, columns []string, values []interface{}, conflictAction string) (err error) {
	defer derrors.Wrap(&err, "DB.BulkInsert(ctx, %q, %v, [%d values], %q)",
		table, columns, len(values), conflictAction)

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
	for leftBound := 0; leftBound < len(values); leftBound += stride {
		rightBound := leftBound + stride
		if rightBound > len(values) {
			rightBound = len(values)
		}
		valueSlice := values[leftBound:rightBound]
		query := buildInsertQuery(table, columns, valueSlice, conflictAction)
		if _, err := db.Exec(ctx, query, valueSlice...); err != nil {
			return fmt.Errorf("tx.ExecContext(ctx, [bulk insert query], values[%d:%d]): %v", leftBound, rightBound, err)
		}
	}
	return nil
}

// buildInsertQuery builds an multi-value insert query, following the format:
// INSERT TO <table> (<columns>) VALUES
// (<placeholders-for-each-item-in-values>) If conflictNoAction is true, it
// append ON CONFLICT DO NOTHING to the end of the query.
//
// When calling buildInsertQuery, it must be true that
//	len(values) % len(columns) == 0
func buildInsertQuery(table string, columns []string, values []interface{}, conflictAction string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s", table)
	fmt.Fprintf(&b, "(%s) VALUES", strings.Join(columns, ", "))

	var placeholders []string
	for i := 1; i <= len(values); i++ {
		// Construct the full query by adding placeholders for each
		// set of values that we want to insert.
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		if i%len(columns) != 0 {
			continue
		}

		// When the end of a set is reached, write it to the query
		// builder and reset placeholders.
		fmt.Fprintf(&b, "(%s)", strings.Join(placeholders, ", "))
		placeholders = []string{}

		// Do not add a comma delimiter after the last set of values.
		if i == len(values) {
			break
		}
		b.WriteString(", ")
	}
	if conflictAction != "" {
		b.WriteString(" " + conflictAction)
	}

	return b.String()
}

// QueryLoggingDisabled stops logging of queries when true.
// For use in tests only: not concurrency-safe.
var QueryLoggingDisabled bool

var queryCounter int64 // atomic: per-process counter for unique query IDs

type queryEndLogEntry struct {
	ID              string
	Query           string
	Args            string
	DurationSeconds float64
	Error           string `json:",omitempty"`
}

func logQuery(ctx context.Context, query string, args []interface{}) func(*error) {
	if QueryLoggingDisabled {
		return func(*error) {}
	}
	const maxlen = 300 // maximum length of displayed query

	// To make the query more compact and readable, replace newlines with spaces
	// and collapse adjacent whitespace.
	var r []rune
	for _, c := range query {
		if c == '\n' {
			c = ' '
		}
		if len(r) == 0 || !unicode.IsSpace(r[len(r)-1]) || !unicode.IsSpace(c) {
			r = append(r, c)
		}
	}
	query = string(r)
	if len(query) > maxlen {
		query = query[:maxlen] + "..."
	}

	instanceID := config.InstanceID()
	if instanceID == "" {
		instanceID = "local"
	} else {
		// Instance IDs are long strings. The low-order part seems quite random, so
		// shortening the ID will still likely result in something unique.
		instanceID = instanceID[len(instanceID)-4:]
	}
	n := atomic.AddInt64(&queryCounter, 1)
	uid := fmt.Sprintf("%s-%d", instanceID, n)

	// Construct a short string of the args.
	const (
		maxArgs   = 20
		maxArgLen = 50
	)
	var argStrings []string
	for i := 0; i < len(args) && i < maxArgs; i++ {
		s := fmt.Sprint(args[i])
		if len(s) > maxArgLen {
			s = s[:maxArgLen] + "..."
		}
		argStrings = append(argStrings, s)
	}
	if len(args) > maxArgs {
		argStrings = append(argStrings, "...")
	}
	argString := strings.Join(argStrings, ", ")

	log.Debugf(ctx, "%s %s args=%s", uid, query, argString)
	start := time.Now()
	return func(errp *error) {
		dur := time.Since(start)
		if errp == nil { // happens with queryRow
			log.Debugf(ctx, "%s done", uid)
		} else {
			derrors.Wrap(errp, "DB running query %s", uid)
			entry := queryEndLogEntry{
				ID:              uid,
				Query:           query,
				Args:            argString,
				DurationSeconds: dur.Seconds(),
			}
			if *errp == nil {
				log.Debug(ctx, entry)
			} else {
				entry.Error = (*errp).Error()
				log.Error(ctx, entry)
			}
		}
	}
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
