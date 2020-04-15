// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal/testing/dbtest"
)

const testTimeout = 5 * time.Second

var testDB *DB

func TestMain(m *testing.M) {
	const dbName = "discovery_postgres_test"

	if err := dbtest.CreateDBIfNotExists(dbName); err != nil {
		log.Fatal(err)
	}
	var err error
	testDB, err = Open("postgres", dbtest.DBConnURI(dbName))
	if err != nil {
		log.Fatal(err)
	}
	code := m.Run()
	if err := testDB.Close(); err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}

func TestBulkInsert(t *testing.T) {
	table := "test_bulk_insert"

	for _, tc := range []struct {
		name           string
		columns        []string
		values         []interface{}
		conflictAction string
		wantErr        bool
		wantCount      int
	}{
		{

			name:      "test-one-row",
			columns:   []string{"colA"},
			values:    []interface{}{"valueA"},
			wantCount: 1,
		},
		{

			name:      "test-multiple-rows",
			columns:   []string{"colA"},
			values:    []interface{}{"valueA1", "valueA2", "valueA3"},
			wantCount: 3,
		},
		{

			name:    "test-invalid-column-name",
			columns: []string{"invalid_col"},
			values:  []interface{}{"valueA"},
			wantErr: true,
		},
		{

			name:    "test-mismatch-num-cols-and-vals",
			columns: []string{"colA", "colB"},
			values:  []interface{}{"valueA1", "valueB1", "valueA2"},
			wantErr: true,
		},
		{

			name:           "test-conflict-no-action-true",
			columns:        []string{"colA"},
			values:         []interface{}{"valueA", "valueA"},
			conflictAction: OnConflictDoNothing,
			wantCount:      1,
		},
		{

			name:    "test-conflict-no-action-false",
			columns: []string{"colA"},
			values:  []interface{}{"valueA", "valueA"},
			wantErr: true,
		},
		{

			// This should execute the statement
			// INSERT INTO series (path) VALUES ('''); TRUNCATE series CASCADE;)');
			// which will insert a row with path value:
			// '); TRUNCATE series CASCADE;)
			// Rather than the statement
			// INSERT INTO series (path) VALUES (''); TRUNCATE series CASCADE;));
			// which would truncate most tables in the database.
			name:           "test-sql-injection",
			columns:        []string{"colA"},
			values:         []interface{}{fmt.Sprintf("''); TRUNCATE %s CASCADE;))", table)},
			conflictAction: OnConflictDoNothing,
			wantCount:      1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			createQuery := fmt.Sprintf(`CREATE TABLE %s (
					colA TEXT NOT NULL,
					colB TEXT,
					PRIMARY KEY (colA)
				);`, table)
			if _, err := testDB.Exec(ctx, createQuery); err != nil {
				t.Fatal(err)
			}
			defer func() {
				dropTableQuery := fmt.Sprintf("DROP TABLE %s;", table)
				if _, err := testDB.Exec(ctx, dropTableQuery); err != nil {
					t.Fatal(err)
				}
			}()

			if err := testDB.Transact(ctx, func(db *DB) error {
				return db.BulkInsert(ctx, table, tc.columns, tc.values, tc.conflictAction)
			}); tc.wantErr && err == nil || !tc.wantErr && err != nil {
				t.Errorf("testDB.Transact: %v | wantErr = %t", err, tc.wantErr)
			}

			if tc.wantCount != 0 {
				var count int
				query := "SELECT COUNT(*) FROM " + table
				row := testDB.QueryRow(ctx, query)
				err := row.Scan(&count)
				if err != nil {
					t.Fatalf("testDB.queryRow(%q): %v", query, err)
				}
				if count != tc.wantCount {
					t.Errorf("testDB.queryRow(%q) = %d; want = %d", query, count, tc.wantCount)
				}
			}
		})
	}
}

func TestLargeBulkInsert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := testDB.Exec(ctx, `CREATE TEMPORARY TABLE test_large_bulk (i BIGINT);`); err != nil {
		t.Fatal(err)
	}
	const size = 150000
	vals := make([]interface{}, size)
	for i := 0; i < size; i++ {
		vals[i] = i + 1
	}
	if err := testDB.Transact(ctx, func(db *DB) error {
		return db.BulkInsert(ctx, "test_large_bulk", []string{"i"}, vals, "")
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := testDB.Query(ctx, `SELECT i FROM test_large_bulk;`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	sum := int64(0)
	for rows.Next() {
		var i int64
		if err := rows.Scan(&i); err != nil {
			t.Fatal(err)
		}
		sum += i
	}
	var want int64 = size * (size + 1) / 2
	if sum != want {
		t.Errorf("sum = %d, want %d", sum, want)
	}
}

func TestDBAfterTransactFails(t *testing.T) {
	ctx := context.Background()
	var tx *DB
	err := testDB.Transact(ctx, func(d *DB) error {
		tx = d
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var i int
	err = tx.QueryRow(ctx, `SELECT 1`).Scan(&i)
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

func TestBuildBulkUpdateQuery(t *testing.T) {
	q := buildBulkUpdateQuery("tab", []string{"K", "C1", "C2"}, []string{"TEXT", "INT", "BOOL"})
	got := strings.Join(strings.Fields(q), " ")
	w := `
		UPDATE tab
		SET C1 = data.C1, C2 = data.C2
		FROM (SELECT UNNEST($1::TEXT[]) AS K, UNNEST($2::INT[]) AS C1, UNNEST($3::BOOL[]) AS C2) AS data
		WHERE tab.K = data.K`
	want := strings.Join(strings.Fields(w), " ")
	if got != want {
		t.Errorf("\ngot\n%s\nwant\n%s", got, want)
	}
}

func TestBulkUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer func(old int) { maxBulkUpdateArrayLen = old }(maxBulkUpdateArrayLen)
	maxBulkUpdateArrayLen = 5

	if _, err := testDB.Exec(ctx, `CREATE TABLE bulk_update (a INT, b INT)`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := testDB.Exec(ctx, `DROP TABLE bulk_update`); err != nil {
			t.Fatal(err)
		}
	}()

	cols := []string{"a", "b"}
	var values []interface{}
	for i := 0; i < 50; i++ {
		values = append(values, i, i)
	}
	err := testDB.Transact(ctx, func(tx *DB) error {
		return tx.BulkInsert(ctx, "bulk_update", cols, values, "")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Update all even values of column a.
	updateVals := make([][]interface{}, 2)
	for i := 0; i < len(values)/2; i += 2 {
		updateVals[0] = append(updateVals[0], i)
		updateVals[1] = append(updateVals[1], -i)
	}

	err = testDB.Transact(ctx, func(tx *DB) error {
		return tx.BulkUpdate(ctx, "bulk_update", cols, []string{"INT", "INT"}, updateVals)
	})
	if err != nil {
		t.Fatal(err)
	}

	err = testDB.RunQuery(ctx, `SELECT a, b FROM bulk_update`, func(rows *sql.Rows) error {
		var a, b int
		if err := rows.Scan(&a, &b); err != nil {
			return err
		}
		want := a
		if a%2 == 0 {
			want = -a
		}
		if b != want {
			t.Fatalf("a=%d: got %d, want %d", a, b, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
