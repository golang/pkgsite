// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"unicode"

	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
)

// DB wraps a sql.DB to provide an API for interacting with discovery data
// stored in Postgres.
type DB struct {
	db *sql.DB
}

func (db *DB) exec(ctx context.Context, query string, args ...interface{}) (res sql.Result, err error) {
	defer logQuery(query, args)(&err)

	return db.db.ExecContext(ctx, query, args...)
}

func execTx(ctx context.Context, tx *sql.Tx, query string, args ...interface{}) (res sql.Result, err error) {
	defer logQuery(query, args)(&err)

	return tx.ExecContext(ctx, query, args...)
}

func (db *DB) query(ctx context.Context, query string, args ...interface{}) (_ *sql.Rows, err error) {
	defer logQuery(query, args)(&err)
	return db.db.QueryContext(ctx, query, args...)
}

func (db *DB) queryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	defer logQuery(query, args)(nil)
	return db.db.QueryRowContext(ctx, query, args...)
}

var queryCounter int64 // atomic: per-process counter for unique query IDs

func logQuery(query string, args []interface{}) func(*error) {
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

	const maxargs = 20 // maximum displayed args
	var moreargs string
	if len(args) > maxargs {
		args = args[:maxargs]
		moreargs = "..."
	}

	log.Printf("%s %s %v%s", uid, query, args, moreargs)
	return func(errp *error) {
		if errp == nil { // happens with queryRow
			log.Printf("%s done", uid)
		} else {
			log.Printf("%s err=%v", uid, *errp)
			derrors.Wrap(errp, "DB running query %s", uid)
		}
	}
}

// Open creates a new DB for the given Postgres connection string.
func Open(driverName, dbinfo string) (*DB, error) {
	db, err := sql.Open(driverName, dbinfo)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

// Transact executes the given function in the context of a SQL transaction,
// rolling back the transaction if the function panics or returns an error.
func (db *DB) Transact(txFunc func(*sql.Tx) error) (err error) {
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

	if err := txFunc(tx); err != nil {
		return fmt.Errorf("txFunc(tx): %v", err)
	}
	return nil
}

// prepareAndExec prepares a query statement and executes it insde the provided
// transaction.
func prepareAndExec(tx *sql.Tx, query string, stmtFunc func(*sql.Stmt) error) (err error) {
	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(%q): %v", query, err)
	}

	defer func() {
		cerr := stmt.Close()
		if err == nil {
			err = cerr
		}
	}()
	if err := stmtFunc(stmt); err != nil {
		return fmt.Errorf("stmtFunc(stmt): %v", err)
	}
	return nil
}

const onConflictDoNothing = "ON CONFLICT DO NOTHING"

// bulkInsert constructs and executes a multi-value insert statement. The
// query is constructed using the format: INSERT TO <table> (<columns>) VALUES
// (<placeholders-for-each-item-in-values>) If conflictNoAction is true, it
// append ON CONFLICT DO NOTHING to the end of the query. The query is executed
// using a PREPARE statement with the provided values.
func bulkInsert(ctx context.Context, tx *sql.Tx, table string, columns []string, values []interface{}, conflictAction string) (err error) {
	defer derrors.Wrap(&err, "bulkInsert(ctx, tx, %q, %v, [%d values], %q)",
		table, columns, len(values), conflictAction)

	if remainder := len(values) % len(columns); remainder != 0 {
		return fmt.Errorf("modulus of len(values) and len(columns) must be 0: got %d", remainder)
	}

	const maxParameters = 65535 // maximum number of parameters allowed by Postgres
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
		query, err := buildInsertQuery(table, columns, valueSlice, conflictAction)
		if err != nil {
			return fmt.Errorf("buildInsertQuery(%q, %v, values[%d:%d], %q): %v", table, columns, leftBound, rightBound, conflictAction, err)
		}

		defer logQuery(query, valueSlice)(&err)
		if _, err := tx.ExecContext(ctx, query, valueSlice...); err != nil {
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
func buildInsertQuery(table string, columns []string, values []interface{}, conflictAction string) (string, error) {
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

	return b.String(), nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.db.Close()
}

// runQuery executes query, then calls f on each row.
func (db *DB) runQuery(ctx context.Context, query string, f func(*sql.Rows) error, params ...interface{}) error {
	rows, err := db.query(ctx, query, params...)
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

// nullIsEmpty returns a sql.Scanner that writes the empty string to s if the
// sql.Value is NULL.
func nullIsEmpty(s *string) sql.Scanner {
	return emptyStringScanner{s}
}
