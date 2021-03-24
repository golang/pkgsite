// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
)

func TestCopyUpsert(t *testing.T) {
	ctx := context.Background()
	conn, err := testDB.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn.Raw(func(c interface{}) error {
		if _, ok := c.(*stdlib.Conn); !ok {
			t.Skip("skipping; DB driver not pgx")
		}
		return nil
	})

	for _, stmt := range []string{
		`DROP TABLE IF EXISTS test_streaming_upsert`,
		`CREATE TABLE test_streaming_upsert (key INTEGER PRIMARY KEY, value TEXT)`,
		`INSERT INTO test_streaming_upsert (key, value) VALUES (1, 'foo'), (2, 'bar')`,
	} {
		if _, err := testDB.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	rows := [][]interface{}{
		{3, "baz"}, // new row
		{1, "moo"}, // replace "foo" with "moo"
	}
	err = testDB.Transact(ctx, sql.LevelDefault, func(tx *DB) error {
		return tx.CopyUpsert(ctx, "test_streaming_upsert", []string{"key", "value"}, pgx.CopyFromRows(rows), []string{"key"})
	})
	if err != nil {
		t.Fatal(err)
	}

	type row struct {
		Key   int
		Value string
	}

	wantRows := []row{
		{1, "moo"},
		{2, "bar"},
		{3, "baz"},
	}
	var gotRows []row
	if err := testDB.CollectStructs(ctx, &gotRows, `SELECT * FROM test_streaming_upsert ORDER BY key`); err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(gotRows, wantRows) {
		t.Errorf("got %v, want %v", gotRows, wantRows)
	}

}
