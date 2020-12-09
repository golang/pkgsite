// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetNestedModules(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	for _, m := range []*internal.Module{
		sample.Module("cloud.google.com/go", "v0.46.2", "storage", "spanner", "pubsub"),
		sample.Module("cloud.google.com/go/pubsub", "v1.6.1", sample.Suffix),
		sample.Module("cloud.google.com/go/spanner", "v1.9.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage", "v1.10.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/v11", "v11.0.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/v9", "v9.0.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/module", "v1.10.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/v9/module", "v9.0.0", sample.Suffix),
		sample.Module("cloud.google.com/go/v2", "v2.0.0", "storage", "spanner", "pubsub"),
	} {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		modulePath     string
		subdirectories []*Subdirectory
		want           []*NestedModule
	}{
		{
			modulePath: "cloud.google.com/go",
			want: []*NestedModule{
				{
					Suffix: "pubsub",
					URL:    "/cloud.google.com/go/pubsub",
				},
				{
					Suffix: "spanner",
					URL:    "/cloud.google.com/go/spanner",
				},
				{
					Suffix: "storage",
					URL:    "/cloud.google.com/go/storage/v11",
				},
				{
					Suffix: "storage/module",
					URL:    "/cloud.google.com/go/storage/module",
				},
				{
					Suffix: "storage/v9/module",
					URL:    "/cloud.google.com/go/storage/v9/module",
				},
			},
		},
		{
			modulePath: "cloud.google.com/go/spanner",
		},
		{
			modulePath: "cloud.google.com/go/storage",
			want: []*NestedModule{
				{
					Suffix: "module",
					URL:    "/cloud.google.com/go/storage/module",
				},
				{
					Suffix: "v9/module",
					URL:    "/cloud.google.com/go/storage/v9/module",
				},
			},
		},
		{
			modulePath: "cloud.google.com/go/storage",
			subdirectories: []*Subdirectory{
				{
					Suffix: "module",
					URL:    "/cloud.google.com/go/storage/module",
				},
				{
					Suffix: "v9/module",
					URL:    "/cloud.google.com/go/storage/v9/module",
				},
			},
		},
		{
			modulePath: "cloud.google.com/go/storage/v9",
			subdirectories: []*Subdirectory{
				{
					Suffix: "module",
					URL:    "/cloud.google.com/go/storage/v9/module",
				},
			},
		},
	} {
		t.Run(test.modulePath, func(t *testing.T) {
			got, err := getNestedModules(ctx, testDB, &internal.UnitMeta{
				Path:       test.modulePath,
				ModulePath: test.modulePath,
			}, test.subdirectories)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetImportedByCount(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	newModule := func(modPath string, imports []string, numImportedBy int) *internal.Module {
		m := sample.Module(modPath, sample.VersionString, "")
		m.Packages()[0].Imports = imports
		m.Packages()[0].NumImportedBy = numImportedBy
		return m
	}

	p1 := "path.to/foo"
	p2 := "path2.to/foo"
	p3 := "path3.to/foo"
	mod1 := newModule(p1, nil, 2)
	mod2 := newModule(p2, []string{p1}, 1)
	mod3 := newModule(p3, []string{p1, p2}, 0)
	for _, m := range []*internal.Module{mod1, mod2, mod3} {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	mainPageImportedByLimit = 2
	tabImportedByLimit = 3
	for _, test := range []struct {
		mod  *internal.Module
		want string
	}{
		{
			mod:  mod3,
			want: "0",
		},
		{
			mod:  mod2,
			want: "1",
		},
		{
			mod:  mod1,
			want: "2+",
		},
	} {
		pkg := test.mod.Packages()[0]
		t.Run(test.mod.ModulePath, func(t *testing.T) {
			got, err := getImportedByCount(ctx, testDB, pkg)
			if err != nil {
				t.Fatalf("getImportedByCount(ctx, db, %q) = %v err = %v, want %v",
					pkg.Path, got, err, test.want)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("getImportedByCount(ctx, db, %q) mismatch (-want +got):\n%s", pkg.Path, diff)
			}
		})
	}
}

func TestApproximateLowerBound(t *testing.T) {
	for _, test := range []struct {
		in, want int
	}{
		{0, 0},
		{1, 1},
		{5, 5},
		{10, 10},
		{11, 10},
		{23, 20},
		{57, 50},
		{124, 100},
		{2593, 2000},
	} {
		got := approximateLowerBound(test.in)
		if got != test.want {
			t.Errorf("approximateLowerBound(%d) = %d, want %d", test.in, got, test.want)
		}
	}
}
