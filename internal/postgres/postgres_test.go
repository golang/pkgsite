// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

var testDB *DB

func TestMain(m *testing.M) {
	RunDBTests("discovery_postgres_test", m, &testDB)
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
			conflictAction: onConflictDoNothing,
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
			conflictAction: onConflictDoNothing,
			wantCount:      1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			createQuery := fmt.Sprintf(`CREATE TABLE %s (
					colA TEXT NOT NULL,
					colB TEXT,
					PRIMARY KEY (colA)
				);`, table)
			if _, err := testDB.exec(ctx, createQuery); err != nil {
				t.Fatalf("testDB.exec(ctx, %q): %v", createQuery, err)
			}
			defer func() {
				dropTableQuery := fmt.Sprintf("DROP TABLE %s;", table)
				if _, err := testDB.exec(ctx, dropTableQuery); err != nil {
					t.Fatalf("testDB.exec(ctx, %q): %v", dropTableQuery, err)
				}
			}()

			if err := testDB.Transact(func(tx *sql.Tx) error {
				return bulkInsert(ctx, tx, table, tc.columns, tc.values, tc.conflictAction)
			}); tc.wantErr && err == nil || !tc.wantErr && err != nil {
				t.Errorf("testDB.Transact: %v | wantErr = %t", err, tc.wantErr)
			}

			if tc.wantCount != 0 {
				var count int
				query := "SELECT COUNT(*) FROM " + table
				row := testDB.queryRow(ctx, query)
				err := row.Scan(&count)
				if err != nil {
					t.Fatalf("testDB.QueryRow(%q): %v", query, err)
				}
				if count != tc.wantCount {
					t.Errorf("testDB.QueryRow(%q) = %d; want = %d", query, count, tc.wantCount)
				}
			}
		})
	}
}

func TestLargeBulkInsert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := testDB.exec(ctx, `CREATE TEMPORARY TABLE test_large_bulk (i BIGINT);`); err != nil {
		t.Fatal(err)
	}
	const size = 150000
	vals := make([]interface{}, size)
	for i := 0; i < size; i++ {
		vals[i] = i + 1
	}
	if err := testDB.Transact(func(tx *sql.Tx) error {
		return bulkInsert(ctx, tx, "test_large_bulk", []string{"i"}, vals, "")
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := testDB.query(ctx, `SELECT i FROM test_large_bulk;`)
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
