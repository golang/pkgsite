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
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestShouldUpdateSymbolHistory(t *testing.T) {
	testSym := "Foo"
	for _, test := range []struct {
		name       string
		newVersion string
		oldHist    map[string]*internal.Symbol
		want       bool
	}{
		{
			name:    "should update when new version is older",
			oldHist: map[string]*internal.Symbol{testSym: {SinceVersion: "v1.2.3"}},
			want:    true,
		},
		{
			name:    "should update when symbol does not exist",
			oldHist: map[string]*internal.Symbol{},
			want:    true,
		},
		{
			name:    "should update when new version is the same",
			oldHist: map[string]*internal.Symbol{testSym: {SinceVersion: sample.VersionString}},
			want:    true,
		},
		{
			name:    "should not update when new version is newer",
			oldHist: map[string]*internal.Symbol{testSym: {SinceVersion: "v0.1.0"}},
			want:    false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldUpdateSymbolHistory(testSym, sample.VersionString, test.oldHist); got != test.want {
				t.Errorf("shouldUpdateSymbolHistory(%q, %q, %+v) = %t; want = %t",
					testSym, sample.VersionString, test.oldHist, got, test.want)
			}
		})
	}
}

func TestInsertSymbolNamesAndHistory(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	ctx = experiment.NewContext(ctx, internal.ExperimentInsertSymbolHistory)
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
		sample.FunctionNew,
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
		sample.FunctionNew.Name,
	}
	for _, c := range sample.Type.Children {
		want = append(want, c.Name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}

	gotHist, err := getSymbolHistory(ctx, testDB.db, mod.Packages()[0].Path, mod.ModulePath)
	if err != nil {
		t.Fatal(err)
	}
	wantHist := map[string]map[string]*internal.Symbol{
		goosgoarch(internal.All, internal.All): {
			"Constant": {
				Name:         "Constant",
				Synopsis:     "const Constant",
				Section:      "Constants",
				Kind:         "Constant",
				ParentName:   "Constant",
				SinceVersion: "v1.0.0",
			},
			"Function": {
				Name:         "Function",
				Synopsis:     "func Function() error",
				Section:      "Functions",
				Kind:         "Function",
				ParentName:   "Function",
				SinceVersion: "v1.0.0",
			},
			"Type": {
				Name:         "Type",
				Synopsis:     "type Type struct",
				Section:      "Types",
				Kind:         "Type",
				ParentName:   "Type",
				SinceVersion: "v1.0.0",
			},
			"Variable": {
				Name:         "Variable",
				Synopsis:     "var Variable",
				Section:      "Variables",
				Kind:         "Variable",
				ParentName:   "Variable",
				SinceVersion: "v1.0.0",
			},
			"Type.Field": {
				Name:         "Type.Field",
				Synopsis:     "field",
				Section:      "Types",
				Kind:         "Field",
				ParentName:   "Type",
				SinceVersion: "v1.0.0",
			},
			"Type.Method": {
				Name:         "Type.Method",
				Synopsis:     "method",
				Section:      "Types",
				Kind:         "Method",
				ParentName:   "Type",
				SinceVersion: "v1.0.0",
			},
			"New": {
				Name:         "New",
				Synopsis:     "func New() *Type",
				Section:      "Types",
				Kind:         "Function",
				ParentName:   "Type",
				SinceVersion: "v1.0.0",
			},
		},
	}
	if diff := cmp.Diff(wantHist, gotHist); diff != "" {
		t.Fatalf("mismatch (-want +got):\n%s", diff)
	}
}
