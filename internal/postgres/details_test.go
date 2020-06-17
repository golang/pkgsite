// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestPostgres_GetVersionInfo_Latest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	testCases := []struct {
		name, path string
		modules    []*internal.Module
		wantIndex  int // index into versions
		wantErr    error
	}{
		{
			name: "largest release",
			path: "mod.1",
			modules: []*internal.Module{
				sample.Module("mod.1", "v1.1.0-alpha.1", sample.Suffix),
				sample.Module("mod.1", "v1.0.0", sample.Suffix),
				sample.Module("mod.1", "v1.0.0-20190311183353-d8887717615a", sample.Suffix),
			},
			wantIndex: 1,
		},
		{
			name: "largest prerelease",
			path: "mod.2",
			modules: []*internal.Module{
				sample.Module("mod.2", "v1.1.0-beta.10", sample.Suffix),
				sample.Module("mod.2", "v1.1.0-beta.2", sample.Suffix),
				sample.Module("mod.2", "v1.0.0-20190311183353-d8887717615a", sample.Suffix),
			},
			wantIndex: 0,
		},
		{
			name:    "no versions",
			path:    "mod3",
			wantErr: derrors.NotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.modules {
				if err := testDB.InsertModule(ctx, v); err != nil {
					t.Error(err)
				}
			}

			gotVI, err := testDB.LegacyGetModuleInfo(ctx, tc.path, internal.LatestVersion)
			if err != nil {
				if tc.wantErr == nil {
					t.Fatalf("got unexpected error %v", err)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got error %v, want Is(%v)", err, tc.wantErr)
				}
				return
			}
			if tc.wantIndex >= len(tc.modules) {
				t.Fatal("wantIndex too large")
			}
			wantVI := &tc.modules[tc.wantIndex].LegacyModuleInfo
			if diff := cmp.Diff(wantVI, gotVI, cmpopts.EquateEmpty(), cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPostgres_GetImportsAndImportedBy(t *testing.T) {
	var (
		m1          = sample.Module("path.to/foo", "v1.1.0", "bar")
		m2          = sample.Module("path2.to/foo", "v1.2.0", "bar2")
		m3          = sample.Module("path3.to/foo", "v1.3.0", "bar3")
		testModules = []*internal.Module{m1, m2, m3}

		pkg1 = m1.LegacyPackages[0]
		pkg2 = m2.LegacyPackages[0]
		pkg3 = m3.LegacyPackages[0]
	)
	pkg1.Imports = nil
	pkg2.Imports = []string{pkg1.Path}
	pkg3.Imports = []string{pkg2.Path, pkg1.Path}

	for _, tc := range []struct {
		path, modulePath, version string
		wantImports               []string
		wantImportedBy            []string
	}{
		{
			path:           pkg3.Path,
			modulePath:     m3.ModulePath,
			version:        "v1.3.0",
			wantImports:    pkg3.Imports,
			wantImportedBy: nil,
		},
		{
			path:           pkg2.Path,
			modulePath:     m2.ModulePath,
			version:        "v1.2.0",
			wantImports:    pkg2.Imports,
			wantImportedBy: []string{pkg3.Path},
		},
		{
			path:           pkg1.Path,
			modulePath:     m1.ModulePath,
			version:        "v1.1.0",
			wantImports:    nil,
			wantImportedBy: []string{pkg2.Path, pkg3.Path},
		},
		{
			path:           pkg1.Path,
			modulePath:     m2.ModulePath, // should cause pkg2 to be excluded.
			version:        "v1.1.0",
			wantImports:    nil,
			wantImportedBy: []string{pkg3.Path},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for _, v := range testModules {
				if err := testDB.InsertModule(ctx, v); err != nil {
					t.Error(err)
				}
			}

			got, err := testDB.GetImports(ctx, tc.path, tc.modulePath, tc.version)
			if err != nil {
				t.Fatal(err)
			}

			sort.Strings(got)
			sort.Strings(tc.wantImports)
			if diff := cmp.Diff(tc.wantImports, got); diff != "" {
				t.Errorf("testDB.GetImports(%q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}

			gotImportedBy, err := testDB.GetImportedBy(ctx, tc.path, tc.modulePath, 100)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantImportedBy, gotImportedBy); diff != "" {
				t.Errorf("testDB.GetImportedBy(%q, %q) mismatch (-want +got):\n%s", tc.path, tc.modulePath, diff)
			}
		})
	}
}

func TestPostgres_GetTaggedAndPseudoVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		modulePath1 = "path.to/foo"
		modulePath2 = "path.to/foo/v2"
		modulePath3 = "path.to/some/thing"
		testModules = []*internal.Module{
			sample.Module(modulePath3, "v3.0.0", "else"),
			sample.Module(modulePath1, "v1.0.0-alpha.1", "bar"),
			sample.Module(modulePath1, "v1.0.0", "bar"),
			sample.Module(modulePath2, "v2.0.1-beta", "bar"),
			sample.Module(modulePath2, "v2.1.0", "bar"),
		}
	)

	testCases := []struct {
		name, path, modulePath string
		numPseudo              int
		modules                []*internal.Module
		wantTaggedVersions     []*internal.ModuleInfo
	}{
		{
			name:       "want_releases_and_prereleases_only",
			path:       "path.to/foo/bar",
			modulePath: modulePath1,
			numPseudo:  12,
			modules:    testModules,
			wantTaggedVersions: []*internal.ModuleInfo{
				{
					ModulePath: modulePath2,
					Version:    "v2.1.0",
					CommitTime: sample.CommitTime,
				},
				{
					ModulePath: modulePath2,
					Version:    "v2.0.1-beta",
					CommitTime: sample.CommitTime,
				},
				{
					ModulePath: modulePath1,
					Version:    "v1.0.0",
					CommitTime: sample.CommitTime,
				},
				{
					ModulePath: modulePath1,
					Version:    "v1.0.0-alpha.1",
					CommitTime: sample.CommitTime,
				},
			},
		},
		{
			name:       "want_zero_results_in_non_empty_db",
			path:       "not.a/real/path",
			modulePath: "not.a/real/path",
			modules:    testModules,
		},
		{
			name:       "want_zero_results_in_empty_db",
			path:       "not.a/real/path",
			modulePath: "not.a/real/path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			var wantPseudoVersions []*internal.ModuleInfo
			for i := 0; i < tc.numPseudo; i++ {

				pseudo := fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", i+1)
				m := sample.Module(modulePath1, pseudo, "bar")
				if err := testDB.InsertModule(ctx, m); err != nil {
					t.Fatal(err)
				}

				// GetPseudoVersions should only return the 10 most recent pseudo versions,
				// if there are more than 10 in the database
				if i < 10 {
					wantPseudoVersions = append(wantPseudoVersions, &internal.ModuleInfo{
						ModulePath: modulePath1,
						Version:    fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", tc.numPseudo-i),
						CommitTime: sample.CommitTime,
					})
				}
			}

			for _, m := range tc.modules {
				if err := testDB.InsertModule(ctx, m); err != nil {
					t.Fatal(err)
				}
			}

			got, err := testDB.GetPseudoVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantPseudoVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetPseudoVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}

			got, err = testDB.GetTaggedVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantTaggedVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetTaggedVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}

			got, err = testDB.GetPseudoVersionsForModule(ctx, tc.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantPseudoVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetPseudoVersionsForModule(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}

			got, err = testDB.GetTaggedVersionsForModule(ctx, tc.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantTaggedVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetTaggedVersionsForModule(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}

func TestGetPackagesInVersion(t *testing.T) {
	testVersion := sample.Module("test.module", "v1.2.3", "", "foo")

	for _, tc := range []struct {
		name, pkgPath string
		module        *internal.Module
	}{
		{
			name:    "version with multiple packages",
			pkgPath: "test.module",
			module:  testVersion,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			if err := testDB.InsertModule(ctx, tc.module); err != nil {
				t.Error(err)
			}

			got, err := testDB.LegacyGetPackagesInModule(ctx, tc.pkgPath, tc.module.Version)
			if err != nil {
				t.Fatal(err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(internal.LegacyPackage{}, "Imports"),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			}
			if diff := cmp.Diff(tc.module.LegacyPackages, got, opts...); diff != "" {
				t.Errorf("testDB.GetPackageInVersion(ctx, %q, %q) mismatch (-want +got):\n%s", tc.pkgPath, tc.module.Version, diff)
			}
		})
	}
}

func TestJSONBScanner(t *testing.T) {
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
