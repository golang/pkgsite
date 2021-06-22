// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetNestedModules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, test := range []struct {
		name            string
		path            string
		modules         []*internal.Module
		wantModulePaths []string
	}{
		{
			name: "Nested Modules in cloud.google.com/go that have the same module prefix path",
			path: "cloud.google.com/go",
			modules: []*internal.Module{
				sample.Module("cloud.google.com/go", "v0.46.2", "storage", "spanner", "pubsub"),
				sample.Module("cloud.google.com/go/storage", "v1.10.0", sample.Suffix),
				sample.Module("cloud.google.com/go/spanner", "v1.9.0", sample.Suffix),
				sample.Module("cloud.google.com/go/pubsub", "v1.6.1", sample.Suffix),
			},
			wantModulePaths: []string{
				"cloud.google.com/go/pubsub",
				"cloud.google.com/go/spanner",
				"cloud.google.com/go/storage",
			},
		},
		{
			name: "Nested Modules in cloud.google.com/go that have multiple major versions",
			path: "cloud.google.com/go",
			modules: []*internal.Module{
				sample.Module("cloud.google.com/go", "v0.46.2", "storage", "spanner", "pubsub"),
				sample.Module("cloud.google.com/go/storage", "v1.10.0", sample.Suffix),
				sample.Module("cloud.google.com/go/storage/v9", "v9.0.0", sample.Suffix),
				sample.Module("cloud.google.com/go/storage/v11", "v11.0.0", sample.Suffix),
			},
			wantModulePaths: []string{
				"cloud.google.com/go/storage/v11",
			},
		},
		{
			name: "Nested Modules in golang.org/x/tools/v2 that have the same module prefix path",
			path: "golang.org/x/tools/v2",
			modules: []*internal.Module{
				sample.Module("golang.org/x/tools", "v0.0.1", sample.Suffix),
				sample.Module("golang.org/x/tools/gopls", "v0.5.1", sample.Suffix),
			},
			wantModulePaths: []string{
				"golang.org/x/tools/gopls",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()

			for _, v := range test.modules {
				MustInsertModule(ctx, t, testDB, v)
			}

			gotModules, err := testDB.GetNestedModules(ctx, test.path)
			if err != nil {
				t.Fatal(err)
			}
			var gotModulePaths []string
			for _, mod := range gotModules {
				gotModulePaths = append(gotModulePaths, mod.ModulePath)
			}

			if diff := cmp.Diff(test.wantModulePaths, gotModulePaths); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetNestedModules_Excluded(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	test := struct {
		name            string
		path            string
		modules         []*internal.Module
		wantModulePaths []string
	}{
		name: "Nested Modules in cloud.google.com/go that have the same module prefix path",
		path: "cloud.google.com/go",
		modules: []*internal.Module{
			sample.Module("cloud.google.com/go", "v0.46.2", "storage", "spanner", "pubsub"),
			// cloud.google.com/storage will be excluded below.
			sample.Module("cloud.google.com/go/storage", "v1.10.0", sample.Suffix),
			sample.Module("cloud.google.com/go/pubsub", "v1.6.1", sample.Suffix),
			sample.Module("cloud.google.com/go/spanner", "v1.9.0", sample.Suffix),
		},
		wantModulePaths: []string{
			"cloud.google.com/go/pubsub",
			"cloud.google.com/go/spanner",
		},
	}
	for _, m := range test.modules {
		MustInsertModule(ctx, t, testDB, m)
	}
	if err := testDB.InsertExcludedPrefix(ctx, "cloud.google.com/go/storage", "postgres", "test"); err != nil {
		t.Fatal(err)
	}

	gotModules, err := testDB.GetNestedModules(ctx, "cloud.google.com/go")
	if err != nil {
		t.Fatal(err)
	}
	var gotModulePaths []string
	for _, mod := range gotModules {
		gotModulePaths = append(gotModulePaths, mod.ModulePath)
	}
	if diff := cmp.Diff(test.wantModulePaths, gotModulePaths); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestPostgres_GetModuleInfo(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testCases := []struct {
		name, path, version string
		modules             []*internal.Module
		wantIndex           int // index into versions
		wantErr             error
	}{
		{
			name:    "version present",
			path:    "mod.1",
			version: "v1.0.2",
			modules: []*internal.Module{
				sample.Module("mod.1", "v1.1.0", sample.Suffix),
				sample.Module("mod.1", "v1.0.2", sample.Suffix),
				sample.Module("mod.1", "v1.0.0", sample.Suffix),
			},
			wantIndex: 1,
		},
		{
			name:    "version not present",
			path:    "mod.2",
			version: "v1.0.3",
			modules: []*internal.Module{
				sample.Module("mod.2", "v1.1.0", sample.Suffix),
				sample.Module("mod.2", "v1.0.2", sample.Suffix),
				sample.Module("mod.2", "v1.0.0", sample.Suffix),
			},
			wantErr: derrors.NotFound,
		},
		{
			name:    "no versions",
			path:    "mod3",
			version: "v1.2.3",
			wantErr: derrors.NotFound,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			for _, v := range test.modules {
				MustInsertModule(ctx, t, testDB, v)
			}

			gotVI, err := testDB.GetModuleInfo(ctx, test.path, test.version)
			if err != nil {
				if test.wantErr == nil {
					t.Fatalf("got unexpected error %v", err)
				}
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("got error %v, want Is(%v)", err, test.wantErr)
				}
				return
			}
			if test.wantIndex >= len(test.modules) {
				t.Fatal("wantIndex too large")
			}
			wantVI := &test.modules[test.wantIndex].ModuleInfo
			if diff := cmp.Diff(wantVI, gotVI, cmpopts.EquateEmpty(), cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetImportedBy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var (
		m1          = sample.Module("path.to/foo", "v1.1.0", "bar")
		m2          = sample.Module("path2.to/foo", "v1.2.0", "bar2")
		m3          = sample.Module("path3.to/foo", "v1.3.0", "bar3")
		testModules = []*internal.Module{m1, m2, m3}

		pkg1 = m1.Packages()[0]
		pkg2 = m2.Packages()[0]
		pkg3 = m3.Packages()[0]
	)
	pkg1.Imports = nil
	pkg2.Imports = []string{pkg1.Path}
	pkg3.Imports = []string{pkg2.Path, pkg1.Path}

	for _, test := range []struct {
		name, path, modulePath, version string
		wantImports                     []string
		wantImportedBy                  []string
	}{
		{
			name:           "multiple imports no imported by",
			path:           pkg3.Path,
			modulePath:     m3.ModulePath,
			version:        "v1.3.0",
			wantImports:    pkg3.Imports,
			wantImportedBy: nil,
		},
		{
			name:           "one import one imported by",
			path:           pkg2.Path,
			modulePath:     m2.ModulePath,
			version:        "v1.2.0",
			wantImports:    pkg2.Imports,
			wantImportedBy: []string{pkg3.Path},
		},
		{
			name:           "no imports two imported by",
			path:           pkg1.Path,
			modulePath:     m1.ModulePath,
			version:        "v1.1.0",
			wantImports:    nil,
			wantImportedBy: []string{pkg2.Path, pkg3.Path},
		},
		{
			name:           "no imports one imported by",
			path:           pkg1.Path,
			modulePath:     m2.ModulePath, // should cause pkg2 to be excluded.
			version:        "v1.1.0",
			wantImports:    nil,
			wantImportedBy: []string{pkg3.Path},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()

			for _, v := range testModules {
				MustInsertModule(ctx, t, testDB, v)
			}

			gotImportedBy, err := testDB.GetImportedBy(ctx, test.path, test.modulePath, 100)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.wantImportedBy, gotImportedBy); diff != "" {
				t.Errorf("testDB.GetImportedBy(%q, %q) mismatch (-want +got):\n%s", test.path, test.modulePath, diff)
			}
		})
	}
}

func TestJSONBScanner(t *testing.T) {
	t.Parallel()
	type S struct{ A int }

	want := &S{1}
	val, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	var got *S
	js := jsonbScanner{&got}
	if err := js.Scan(val); err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Errorf("got %+v, want %+v", *got, *want)
	}

	var got2 *S
	js = jsonbScanner{&got2}
	if err := js.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if got2 != nil {
		t.Errorf("got %#v, want nil", got2)
	}
}
