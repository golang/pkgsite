// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"path"
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
		sample.Module("cloud.google.com/go/v2", "v2.0.0", "storage", "spanner", "pubsub"),
	} {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		modulePath string
		want       []*NestedModule
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
			},
		},
	} {
		t.Run(tc.modulePath, func(t *testing.T) {
			got, err := getNestedModules(ctx, testDB, &internal.UnitMeta{
				Path:       tc.modulePath,
				ModulePath: tc.modulePath,
			})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetImportedByCount(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	newModule := func(modPath string, pkgs ...*internal.Unit) *internal.Module {
		m := sample.Module(modPath, sample.VersionString)
		for _, p := range pkgs {
			sample.AddUnit(m, p)
		}
		return m
	}

	pkg1 := sample.UnitForPackage("path.to/foo/bar", "path.to/foo", sample.VersionString, "bar", true)
	pkg2 := sample.UnitForPackage("path2.to/foo/bar2", "path.to/foo", sample.VersionString, "bar", true)
	pkg2.Imports = []string{pkg1.Path}

	pkg3 := sample.UnitForPackage("path3.to/foo/bar3", "path.to/foo", sample.VersionString, "bar3", true)
	pkg3.Imports = []string{pkg2.Path, pkg1.Path}

	testModules := []*internal.Module{
		newModule("path.to/foo", pkg1),
		newModule("path2.to/foo", pkg2),
		newModule("path3.to/foo", pkg3),
	}

	for _, m := range testModules {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	mainPageImportedByLimit = 2
	tabImportedByLimit = 3
	for _, tc := range []struct {
		pkg  *internal.Unit
		want string
	}{
		{
			pkg:  pkg3,
			want: "0",
		},
		{
			pkg:  pkg2,
			want: "1",
		},
		{
			pkg:  pkg1,
			want: "2+",
		},
	} {
		t.Run(tc.pkg.Path, func(t *testing.T) {
			otherVersion := newModule(path.Dir(tc.pkg.Path), tc.pkg)
			otherVersion.Version = "v1.0.5"
			pkg := otherVersion.Units[1]
			got, err := getImportedByCount(ctx, testDB, pkg)
			if err != nil {
				t.Fatalf("getImportedByCount(ctx, db, %q) = %v err = %v, want %v",
					tc.pkg.Path, got, err, tc.want)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("getImportedByCount(ctx, db, %q) mismatch (-want +got):\n%s", tc.pkg.Path, diff)
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
