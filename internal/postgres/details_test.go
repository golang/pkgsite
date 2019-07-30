// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/sample"
)

func TestPostgres_GetLatestPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	testCases := []struct {
		name, path  string
		versions    []*internal.Version
		wantPkg     *internal.VersionedPackage
		wantReadErr bool
	}{
		{
			name: "want latest package to be most recent release version",
			path: sample.PackagePath,
			versions: []*internal.Version{
				sample.Version(
					sample.WithVersion("v1.1.0-alpha.1"),
					sample.WithVersionType(internal.VersionTypePrerelease)),
				sample.Version(
					sample.WithVersion("v1.0.0"),
					sample.WithVersionType(internal.VersionTypeRelease)),
				sample.Version(
					sample.WithVersion("v1.0.0-20190311183353-d8887717615a"),
					sample.WithVersionType(internal.VersionTypePseudo)),
			},
			wantPkg: sample.VersionedPackage(func(p *internal.VersionedPackage) {
				p.Version = "v1.0.0"
				p.VersionType = internal.VersionTypeRelease
				// TODO(b/130367504): GetLatest does not return imports.
				p.Imports = nil
			}),
		},
		{
			name:        "empty path",
			path:        "",
			wantReadErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v, sample.Licenses); err != nil {
					t.Errorf("testDB.InsertVersion(ctx, %v): %v", v, err)
				}
			}

			gotPkg, err := testDB.GetLatestPackage(ctx, tc.path)
			if (err != nil) != tc.wantReadErr {
				t.Errorf("testDB.GetLatestPackage(ctx, %q): %v", tc.path, err)
			}

			if diff := cmp.Diff(tc.wantPkg, gotPkg, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("testDB.GetLatestPackage(ctx, %q) mismatch (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}

func TestPostgres_GetImportsAndImportedBy(t *testing.T) {
	var (
		modulePath1  = "path.to/foo"
		pkgPath1     = "path.to/foo/bar"
		modulePath2  = "path2.to/foo"
		pkgPath2     = "path2.to/foo/bar2"
		modulePath3  = "path3.to/foo"
		pkgPath3     = "path3.to/foo/bar3"
		pkg1         = sample.Package(sample.WithPath(pkgPath1), sample.WithImports())
		pkg2         = sample.Package(sample.WithPath(pkgPath2), sample.WithImports(pkgPath1))
		pkg3         = sample.Package(sample.WithPath(pkgPath3), sample.WithImports(pkgPath2, pkgPath1))
		testVersions = []*internal.Version{
			sample.Version(
				sample.WithModulePath(modulePath1),
				sample.WithVersion("v1.1.0"),
				sample.WithPackages(pkg1)),
			sample.Version(
				sample.WithModulePath(modulePath2),
				sample.WithVersion("v1.2.0"),
				sample.WithPackages(pkg2)),
			sample.Version(
				sample.WithModulePath(modulePath3),
				sample.WithVersion("v1.3.0"),
				sample.WithPackages(pkg3)),
		}
	)

	for _, tc := range []struct {
		path, modulePath, version string
		limit, offset             int
		wantImports               []string
		wantImportedBy            []string
		wantTotalImportedBy       int
	}{
		{
			path:                pkg3.Path,
			modulePath:          modulePath3,
			version:             "v1.3.0",
			limit:               10,
			offset:              0,
			wantImports:         pkg3.Imports,
			wantImportedBy:      nil,
			wantTotalImportedBy: 0,
		},
		{
			path:                pkg2.Path,
			modulePath:          modulePath2,
			limit:               10,
			offset:              0,
			version:             "v1.2.0",
			wantImports:         pkg2.Imports,
			wantImportedBy:      []string{pkg3.Path},
			wantTotalImportedBy: 1,
		},
		{
			path:                pkg1.Path,
			modulePath:          modulePath1,
			limit:               10,
			offset:              0,
			version:             "v1.1.0",
			wantImports:         nil,
			wantImportedBy:      []string{pkg2.Path, pkg3.Path},
			wantTotalImportedBy: 2,
		},
		{
			path:                pkg1.Path,
			modulePath:          modulePath1,
			version:             "v1.1.0",
			limit:               1,
			offset:              0,
			wantImports:         nil,
			wantImportedBy:      []string{pkg2.Path},
			wantTotalImportedBy: 2,
		},
		{
			path:                pkg1.Path,
			modulePath:          modulePath1,
			version:             "v1.1.0",
			limit:               1,
			offset:              1,
			wantImports:         nil,
			wantImportedBy:      []string{pkg3.Path},
			wantTotalImportedBy: 2,
		},
		{
			path:                pkg1.Path,
			modulePath:          modulePath2, // should cause pkg2 to be excluded.
			version:             "v1.1.0",
			limit:               10,
			offset:              0,
			wantImports:         nil,
			wantImportedBy:      []string{pkg3.Path},
			wantTotalImportedBy: 1,
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for _, v := range testVersions {
				if err := testDB.InsertVersion(ctx, v, sample.Licenses); err != nil {
					t.Errorf("testDB.InsertVersion(%v): %v", v, err)
				}
			}

			got, err := testDB.GetImports(ctx, tc.path, tc.version)
			if err != nil {
				t.Fatalf("testDB.GetImports(%q, %q): %v", tc.path, tc.version, err)
			}

			sort.Strings(got)
			sort.Strings(tc.wantImports)
			if diff := cmp.Diff(tc.wantImports, got); diff != "" {
				t.Errorf("testDB.GetImports(%q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}

			gotImportedBy, total, err := testDB.GetImportedBy(ctx, tc.path, tc.modulePath, tc.limit, tc.offset)
			if err != nil {
				t.Fatalf("testDB.GetImportedBy(%q, %q, %d, %d): %v", tc.path, tc.modulePath, tc.limit, tc.offset, err)
			}
			if total != tc.wantTotalImportedBy {
				t.Errorf("testDB.GetImportedBy(%q, %q, %d, %d): total = %d, want %d", tc.path, tc.modulePath, tc.limit, tc.offset, total, tc.wantTotalImportedBy)
			}

			if diff := cmp.Diff(tc.wantImportedBy, gotImportedBy); diff != "" {
				t.Errorf("testDB.GetImportedBy(%q, %q, %d, %d) mismatch (-want +got):\n%s", tc.path, tc.modulePath, tc.limit, tc.offset, diff)
			}
		})
	}
}

func TestPostgres_GetTaggedAndPseudoVersionsForPackageSeries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		sampleVersion = func(modulePath, version string, suffixes ...string) *internal.Version {
			return sample.Version(
				sample.WithModulePath(modulePath),
				sample.WithVersion(version),
				sample.WithSuffixes(suffixes...))
		}
		modulePath1  = "path.to/foo"
		modulePath2  = "path.to/foo/v2"
		modulePath3  = "path.to/some/thing"
		testVersions = []*internal.Version{
			sampleVersion(modulePath3, "v3.0.0", "else"),
			sampleVersion(modulePath1, "v1.0.0-alpha.1", "bar"),
			sampleVersion(modulePath1, "v1.0.0", "bar"),
			sampleVersion(modulePath2, "v2.0.1-beta", "bar"),
			sampleVersion(modulePath2, "v2.1.0", "bar"),
		}
	)

	testCases := []struct {
		name, path         string
		numPseudo          int
		versions           []*internal.Version
		wantTaggedVersions []*internal.VersionInfo
	}{
		{
			name:      "want_releases_and_prereleases_only",
			path:      "path.to/foo/bar",
			numPseudo: 12,
			versions:  testVersions,
			wantTaggedVersions: []*internal.VersionInfo{
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
			name:     "want_zero_results_in_non_empty_db",
			path:     "not.a/real/path",
			versions: testVersions,
		},
		{
			name: "want_zero_results_in_empty_db",
			path: "not.a/real/path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			wantPseudoVersions := []*internal.VersionInfo{}
			for i := 0; i < tc.numPseudo; i++ {

				pseudo := fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", i+1)
				v := sampleVersion(modulePath1, pseudo, "bar")
				// TODO: move this handling into SimpleVersion once ParseVersionType is
				// factored out of fetch.go
				v.VersionType = internal.VersionTypePseudo
				if err := testDB.InsertVersion(ctx, v, nil); err != nil {
					t.Errorf("testDB.InsertVersion(%v): %v", v, err)
				}

				// GetPseudoVersions should only return the 10 most recent pseudo versions,
				// if there are more than 10 in the database
				if i < 10 {
					wantPseudoVersions = append(wantPseudoVersions, &internal.VersionInfo{
						ModulePath: modulePath1,
						Version:    fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", tc.numPseudo-i),
						CommitTime: sample.CommitTime,
					})
				}
			}

			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v, nil); err != nil {
					t.Errorf("testDB.InsertVersion(%v): %v", v, err)
				}
			}

			var (
				got []*internal.VersionInfo
				err error
			)

			got, err = testDB.GetPseudoVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatalf("testDB.GetPseudoVersionsForPackageSeries(%q) error: %v", tc.path, err)
			}

			if len(got) != len(wantPseudoVersions) {
				t.Fatalf("testDB.GetPseudoVersionsForPackageSeries(%q) returned list of length %v, wanted %v", tc.path, len(got), len(wantPseudoVersions))
			}

			for i, v := range got {
				if diff := cmp.Diff(wantPseudoVersions[i], v); diff != "" {
					t.Errorf("testDB.GetPseudoVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
				}
			}

			got, err = testDB.GetTaggedVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatalf("testDB.GetTaggedVersionsForPackageSeries(%q) error: %v", tc.path, err)
			}

			if len(got) != len(tc.wantTaggedVersions) {
				t.Fatalf("testDB.GetTaggedVersionsForPackageSeries(%q) returned list of length %v, wanted %v", tc.path, len(got), len(tc.wantTaggedVersions))
			}

			for i, v := range got {

				if diff := cmp.Diff(tc.wantTaggedVersions[i], v); diff != "" {
					t.Errorf("testDB.GetTaggedVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
				}
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	testVersion := sample.Version(sample.WithModulePath("test.module"), sample.WithSuffixes("", "foo"))

	for _, tc := range []struct {
		name, path, version string
		want                *internal.Version
	}{
		{
			name:    "version with multiple packages",
			path:    "test.module",
			version: sample.VersionString,
			want:    testVersion,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			if err := testDB.InsertVersion(ctx, tc.want, sample.Licenses); err != nil {
				t.Errorf("testDB.InsertVersion(ctx, %q %q): %v", tc.path, tc.version, err)
			}

			got, err := testDB.GetVersion(ctx, tc.path, tc.version)
			if err != nil {
				t.Errorf("testDB.GetVersion(ctx, %q, %q): %v", tc.path, tc.version, err)
			}

			// TODO(b/130367504): remove this ignore once imports are not asymmetric
			if diff := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(internal.Package{}, "Imports")); diff != "" {
				t.Errorf("testDB.GetVersion(ctx, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}

func TestGetLicenses(t *testing.T) {
	modulePath := "test.module"
	testVersion := sample.Version(sample.WithModulePath(modulePath), sample.WithSuffixes("", "foo"))
	testVersion.Packages[0].Licenses = nil
	testVersion.Packages[1].Licenses = sample.LicenseMetadata

	tests := []struct {
		label, pkgPath string
		wantLicenses   []*license.License
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

	if err := testDB.InsertVersion(ctx, testVersion, sample.Licenses); err != nil {
		t.Errorf("testDB.InsertVersion(ctx, %q, licenses): %v", testVersion.Version, err)
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			got, err := testDB.GetLicenses(ctx, test.pkgPath, modulePath, testVersion.Version)
			if err != nil {
				t.Fatalf("testDB.GetLicenses(ctx, %q, %q): %v", test.pkgPath, testVersion.Version, err)
			}
			if diff := cmp.Diff(test.wantLicenses, got); diff != "" {
				t.Errorf("testDB.GetLicenses(ctx, %q, %q) mismatch (-want +got):\n%s", test.pkgPath, testVersion.Version, diff)
			}
		})
	}
}
