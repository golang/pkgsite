// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// CopyUpsert upserts rows into table using the pgx driver's CopyFrom method.
// It returns an error if the underlying driver is not pgx.
// columns is the list of columns to upsert.
// src is the source of the rows to upsert.
// conflictColumns are the columns that might conflict (i.e. that have a UNIQUE
// constraint).
// If dropColumn is non-empty, that column will be dropped from the temporary
// table before copying. Use dropColumn for generated ID columns.
//
// CopyUpsert works by first creating a temporary table, populating it with
// CopyFrom, and then running an INSERT...SELECT...ON CONFLICT to upsert its
// rows into the original table.
func (db *DB) CopyUpsert(ctx context.Context, table string, columns []string, src pgx.CopyFromSource, conflictColumns []string, dropColumn string) (err error) {
	defer derrors.Wrap(&err, "CopyUpsert(%q)", table)

	if !db.InTransaction() {
		return errors.New("not in a transaction")
	}

	return db.WithPGXConn(func(conn *pgx.Conn) error {
		tempTable := fmt.Sprintf("__%s_copy", table)
		stmt := fmt.Sprintf(`
			DROP TABLE IF EXISTS %s;
			CREATE TEMP TABLE %[1]s (LIKE %s) ON COMMIT DROP;
		`, tempTable, table)
		if dropColumn != "" {
			stmt += fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", tempTable, dropColumn)
		}
		_, err = conn.Exec(ctx, stmt)
		if err != nil {
			return err
		}
		start := time.Now()
		n, err := conn.CopyFrom(ctx, []string{tempTable}, columns, src)
		if err != nil {
			return fmt.Errorf("CopyFrom: %w", err)
		}
		if !QueryLoggingDisabled {
			log.Debugf(ctx, "CopyUpsert(%q): copied %d rows in %s", table, n, time.Since(start))
		}
		conflictAction := buildUpsertConflictAction(columns, conflictColumns)
		cols := strings.Join(columns, ", ")
		query := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s %s", table, cols, cols, tempTable, conflictAction)
		defer logQuery(ctx, query, nil, db.instanceID, db.IsRetryable())(&err)
		start = time.Now()
		ctag, err := conn.Exec(ctx, query)
		if err != nil {
			return err
		}
		if !QueryLoggingDisabled {
			log.Debugf(ctx, "CopyUpsert(%q): upserted %d rows in %s", table, ctag.RowsAffected(), time.Since(start))
		}
		return nil
	})
}

func (db *DB) WithPGXConn(f func(conn *pgx.Conn) error) error {
	if !db.InTransaction() {
		return errors.New("not in a transaction")
	}
	return db.conn.Raw(func(c interface{}) error {
		if w, ok := c.(*wrapConn); ok {
			c = w.underlying
		}
		stdConn, ok := c.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("DB driver is not pgx or wrapper; conn type is %T", c)
		}
		return f(stdConn.Conn())
	})
}

// A RowItem is a row of values or an error.
type RowItem struct {
	Values []interface{}
	Err    error
}

// CopyFromChan returns a CopyFromSource that gets its rows from a channel.
func CopyFromChan(c <-chan RowItem) pgx.CopyFromSource {
	return &chanCopySource{c: c}
}

type chanCopySource struct {
	c    <-chan RowItem
	next RowItem
}

// Next implements CopyFromSource.Next.
func (cs *chanCopySource) Next() bool {
	if cs.next.Err != nil {
		return false
	}
	var ok bool
	cs.next, ok = <-cs.c
	return ok
}

// Values implements CopyFromSource.Values.
func (cs *chanCopySource) Values() ([]interface{}, error) {
	return cs.next.Values, cs.next.Err
}

// Err implements CopyFromSource.Err.
func (cs *chanCopySource) Err() error {
	return cs.next.Err
}
