// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestInsertSymbolNames(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	mod := sample.DefaultModule()
	if len(mod.Packages()) != 1 {
		t.Fatalf("len(mod.Packages()) = %d; want 1", len(mod.Packages()))
	}
	if len(mod.Packages()[0].Documentation) != 1 {
		t.Fatalf("len(mod.Packages()[0].Documentation) = %d; want 1", len(mod.Packages()[0].Documentation))
	}
	mod.Packages()[0].Documentation[0].API = []*internal.Symbol{
		sample.Constant,
		sample.Variable,
		sample.Function,
		sample.Type,
	}
	MustInsertModule(ctx, t, testDB, mod)

	var got []string
	if err := testDB.db.RunQuery(ctx, `SELECT name FROM symbols;`, func(rows *sql.Rows) error {
		var n string
		if err := rows.Scan(&n); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		got = append(got, n)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		sample.Constant.Name,
		sample.Variable.Name,
		sample.Function.Name,
		sample.Type.Name,
	}
	for _, c := range sample.Type.Children {
		want = append(want, c.Name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}
