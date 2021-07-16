// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"golang.org/x/pkgsite/internal/log"
)

func BenchmarkBulkInsert(b *testing.B) {
	ctx := context.Background()
	log.SetLevel("INFO")
	pgxDB, err := Open("pgx", DBConnURI(testDBName), "test")
	if err != nil {
		b.Fatal(err)
	}
	defer pgxDB.Close()

	if _, err := testDB.Exec(ctx, `DROP TABLE IF EXISTS test_large_bulk; CREATE TABLE test_large_bulk (i BIGINT);`); err != nil {
		b.Fatal(err)
	}
	const size = 15000
	vals := make([]interface{}, size)
	for i := 0; i < size; i++ {
		vals[i] = i + 1
	}
	b.Run("pq BulkInsert", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := testDB.Transact(ctx, sql.LevelDefault, func(tx *DB) error {
				return tx.BulkInsert(ctx, "test_large_bulk", []string{"i"}, vals, "")
			}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("pgx BulkInsert", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := pgxDB.Transact(ctx, sql.LevelDefault, func(tx *DB) error {
				return tx.BulkInsert(ctx, "test_large_bulk", []string{"i"}, vals, "")
			}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("pgx CopyFrom", func(b *testing.B) {
		conn, err := pgxDB.db.Conn(ctx)
		if err != nil {
			b.Fatal(err)
		}
		rows := make([][]interface{}, len(vals))
		for i, v := range vals {
			rows[i] = []interface{}{v}
		}
		src := pgx.CopyFromRows(rows)
		for i := 0; i < b.N; i++ {
			err = conn.Raw(func(driverConn interface{}) error {
				pgxConn := driverConn.(*stdlib.Conn).Conn()
				_, err := pgxConn.CopyFrom(ctx, []string{"test_large_bulk"}, []string{"i"}, src)
				return err
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
