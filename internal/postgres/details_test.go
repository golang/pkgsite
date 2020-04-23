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
	"golang.org/x/pkgsite/internal/version"
)

func TestPostgres_GetVersionInfo_Latest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	sampleModule := func(path, version string, vtype version.Type) *internal.Module {
		m := sample.Module()
		m.ModulePath = path
		m.Version = version
		m.VersionType = vtype
		return m
	}

	testCases := []struct {
		name, path string
		modules    []*internal.Module
		wantIndex  int // index into versions
		wantErr    error
	}{
		{
			name: "largest release",
			path: "mod1",
			modules: []*internal.Module{
				sampleModule("mod1", "v1.1.0-alpha.1", version.TypePrerelease),
				sampleModule("mod1", "v1.0.0", version.TypeRelease),
				sampleModule("mod1", "v1.0.0-20190311183353-d8887717615a", version.TypePseudo),
			},
			wantIndex: 1,
		},
		{
			name: "largest prerelease",
			path: "mod2",
			modules: []*internal.Module{
				sampleModule("mod2", "v1.1.0-beta.10", version.TypePrerelease),
				sampleModule("mod2", "v1.1.0-beta.2", version.TypePrerelease),
				sampleModule("mod2", "v1.0.0-20190311183353-d8887717615a", version.TypePseudo),
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
				if err := saveModule(ctx, testDB.db, v); err != nil {
					t.Error(err)
				}
			}

			gotVI, err := testDB.GetModuleInfo(ctx, tc.path, internal.LatestVersion)
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
			wantVI := &tc.modules[tc.wantIndex].ModuleInfo
			if diff := cmp.Diff(wantVI, gotVI, cmpopts.EquateEmpty(), cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPostgres_GetImportsAndImportedBy(t *testing.T) {
	pkg := func(path string, imports []string) *internal.Package {
		p := sample.Package()
		p.Path = path
		p.Imports = imports
		return p
	}

	sampleModule := func(mpath, version string, pkg *internal.Package) *internal.Module {
		m := sample.Module()
		m.ModulePath = mpath
		m.Version = version
		m.Packages = []*internal.Package{pkg}
		return m
	}

	var (
		modulePath1 = "path.to/foo"
		pkgPath1    = "path.to/foo/bar"
		modulePath2 = "path2.to/foo"
		pkgPath2    = "path2.to/foo/bar2"
		modulePath3 = "path3.to/foo"
		pkgPath3    = "path3.to/foo/bar3"
		pkg1        = pkg(pkgPath1, nil)
		pkg2        = pkg(pkgPath2, []string{pkgPath1})
		pkg3        = pkg(pkgPath3, []string{pkgPath2, pkgPath1})
		testModules = []*internal.Module{
			sampleModule(modulePath1, "v1.1.0", pkg1),
			sampleModule(modulePath2, "v1.2.0", pkg2),
			sampleModule(modulePath3, "v1.3.0", pkg3),
		}
	)

	for _, tc := range []struct {
		path, modulePath, version string
		wantImports               []string
		wantImportedBy            []string
	}{
		{
			path:           pkg3.Path,
			modulePath:     modulePath3,
			version:        "v1.3.0",
			wantImports:    pkg3.Imports,
			wantImportedBy: nil,
		},
		{
			path:           pkg2.Path,
			modulePath:     modulePath2,
			version:        "v1.2.0",
			wantImports:    pkg2.Imports,
			wantImportedBy: []string{pkg3.Path},
		},
		{
			path:           pkg1.Path,
			modulePath:     modulePath1,
			version:        "v1.1.0",
			wantImports:    nil,
			wantImportedBy: []string{pkg2.Path, pkg3.Path},
		},
		{
			path:           pkg1.Path,
			modulePath:     modulePath2, // should cause pkg2 to be excluded.
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
				if err := saveModule(ctx, testDB.db, v); err != nil {
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
		sampleModule = func(modulePath, version string, suffixes ...string) *internal.Module {
			m := sample.Module()
			m.ModulePath = modulePath
			m.Version = version
			sample.SetSuffixes(m, suffixes...)
			return m
		}
		modulePath1 = "path.to/foo"
		modulePath2 = "path.to/foo/v2"
		modulePath3 = "path.to/some/thing"
		testModules = []*internal.Module{
			sampleModule(modulePath3, "v3.0.0", "else"),
			sampleModule(modulePath1, "v1.0.0-alpha.1", "bar"),
			sampleModule(modulePath1, "v1.0.0", "bar"),
			sampleModule(modulePath2, "v2.0.1-beta", "bar"),
			sampleModule(modulePath2, "v2.1.0", "bar"),
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
				m := sampleModule(modulePath1, pseudo, "bar")
				// TODO: move this handling into SimpleVersion once ParseVersionType is
				// factored out of fetch.go
				m.VersionType = version.TypePseudo
				if err := saveModule(ctx, testDB.db, m); err != nil {
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
				if err := saveModule(ctx, testDB.db, m); err != nil {
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
	testVersion := sample.Module()
	testVersion.ModulePath = "test.module"
	sample.SetSuffixes(testVersion, "", "foo")

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

			if err := saveModule(ctx, testDB.db, tc.module); err != nil {
				t.Error(err)
			}

			got, err := testDB.GetPackagesInModule(ctx, tc.pkgPath, tc.module.Version)
			if err != nil {
				t.Fatal(err)
			}

			opts := []cmp.Option{
				// TODO(b/130367504): remove this ignore once imports are not asymmetric
				cmpopts.IgnoreFields(internal.Package{}, "Imports"),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			}
			if diff := cmp.Diff(tc.module.Packages, got, opts...); diff != "" {
				t.Errorf("testDB.GetPackageInVersion(ctx, %q, %q) mismatch (-want +got):\n%s", tc.pkgPath, tc.module.Version, diff)
			}
		})
	}
}

func TestGetPackageLicenses(t *testing.T) {
	modulePath := "test.module"
	testModule := sample.Module()
	testModule.ModulePath = modulePath
	sample.SetSuffixes(testModule, "", "foo")
	testModule.Packages[0].Licenses = nil
	testModule.Packages[1].Licenses = sample.LicenseMetadata

	tests := []struct {
		label, pkgPath string
		wantLicenses   []*licenses.License
	}{
		{
			label:        "package with licenses",
			pkgPath:      "test.module/foo",
			wantLicenses: sample.Licenses,
		}, {
			label:        "package with no licenses",
			pkgPath:      "test.module",
			wantLicenses: nil,
		},
	}

	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := saveModule(ctx, testDB.db, testModule); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			got, err := testDB.GetPackageLicenses(ctx, test.pkgPath, modulePath, testModule.Version)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.wantLicenses, got); diff != "" {
				t.Errorf("testDB.GetLicenses(ctx, %q, %q) mismatch (-want +got):\n%s", test.pkgPath, testModule.Version, diff)
			}
		})
	}
}

func TestGetModuleLicenses(t *testing.T) {
	modulePath := "test.module"
	testModule := sample.Module()
	testModule.ModulePath = modulePath
	sample.SetSuffixes(testModule, "", "foo", "bar")
	testModule.Packages[0].Licenses = []*licenses.Metadata{{Types: []string{"ISC"}, FilePath: "LICENSE"}}
	testModule.Packages[1].Licenses = []*licenses.Metadata{{Types: []string{"MIT"}, FilePath: "foo/LICENSE"}}
	testModule.Packages[2].Licenses = []*licenses.Metadata{{Types: []string{"GPL2"}, FilePath: "bar/LICENSE.txt"}}

	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, p := range testModule.Packages {
		testModule.Licenses = append(testModule.Licenses, &licenses.License{
			Metadata: p.Licenses[0],
			Contents: []byte(`Lorem Ipsum`),
		})
	}

	if err := testDB.InsertModule(ctx, testModule); err != nil {
		t.Fatal(err)
	}

	got, err := testDB.GetModuleLicenses(ctx, modulePath, testModule.Version)
	if err != nil {
		t.Fatal(err)
	}
	// We only want the top-level license.
	wantLicenses := []*licenses.License{testModule.Licenses[0]}
	if diff := cmp.Diff(wantLicenses, got); diff != "" {
		t.Errorf("testDB.GetModuleLicenses(ctx, %q, %q) mismatch (-want +got):\n%s", modulePath, testModule.Version, diff)
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
