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
)

func TestPostgres_GetLatestPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)
	var (
		pkg = &internal.Package{
			Path:     "path.to/foo/bar",
			Name:     "bar",
			Synopsis: "This is a package synopsis",
			Licenses: SampleLicenseMetadata,
		}
		testVersions = []*internal.Version{
			SampleVersion(func(v *internal.Version) {
				v.Version = "v1.0.0-alpha.1"
				v.VersionType = internal.VersionTypePrerelease
				v.Packages = []*internal.Package{pkg}
			}),
			SampleVersion(func(v *internal.Version) {
				v.Version = "v1.0.0"
				v.VersionType = internal.VersionTypeRelease
				v.Packages = []*internal.Package{pkg}
			}),
			SampleVersion(func(v *internal.Version) {
				v.Version = "v1.0.0-20190311183353-d8887717615a"
				v.VersionType = internal.VersionTypePseudo
				v.Packages = []*internal.Package{pkg}
			}),
		}
	)

	testCases := []struct {
		name, path  string
		versions    []*internal.Version
		wantPkg     *internal.VersionedPackage
		wantReadErr bool
	}{
		{
			name:     "want_second_package",
			path:     pkg.Path,
			versions: testVersions,
			wantPkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name:     pkg.Name,
					Path:     pkg.Path,
					Synopsis: pkg.Synopsis,
					Licenses: SampleLicenseMetadata,
				},
				VersionInfo: internal.VersionInfo{
					ModulePath:     testVersions[1].ModulePath,
					Version:        testVersions[1].Version,
					CommitTime:     testVersions[1].CommitTime,
					ReadmeContents: testVersions[1].ReadmeContents,
					ReadmeFilePath: testVersions[1].ReadmeFilePath,
				},
			},
		},
		{
			name:        "empty_path",
			path:        "",
			wantReadErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v, SampleLicenses); err != nil {
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
		now  = NowTruncated()
		pkg1 = &internal.Package{
			Name:     "bar",
			Path:     "path.to/foo/bar",
			Synopsis: "This is a package synopsis",
		}
		pkg2 = &internal.Package{
			Name:     "bar2",
			Path:     "path2.to/foo/bar2",
			Synopsis: "This is another package synopsis",
			Imports:  []string{pkg1.Path},
		}
		pkg3 = &internal.Package{
			Name:     "bar3",
			Path:     "path3.to/foo/bar3",
			Synopsis: "This is another package synopsis",
			Imports:  []string{pkg2.Path, pkg1.Path},
		}
		modulePath1  = "path.to/foo"
		modulePath2  = "path2.to/foo"
		modulePath3  = "path3.to/foo"
		testVersions = []*internal.Version{
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:     modulePath1,
					Version:        "v1.1.0",
					ReadmeContents: []byte("readme"),
					CommitTime:     now,
					VersionType:    internal.VersionTypePrerelease,
				},
				Packages: []*internal.Package{pkg1},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:     modulePath2,
					Version:        "v1.2.0",
					ReadmeContents: []byte("readme"),
					CommitTime:     now,
					VersionType:    internal.VersionTypePseudo,
				},
				Packages: []*internal.Package{pkg2},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:     modulePath3,
					Version:        "v1.3.0",
					ReadmeContents: []byte("readme"),
					CommitTime:     now,
					VersionType:    internal.VersionTypePseudo,
				},
				Packages: []*internal.Package{pkg3},
			},
		}
	)

	for _, tc := range []struct {
		path, version  string
		wantImports    []string
		wantImportedBy []string
	}{
		{
			path:           pkg3.Path,
			version:        "v1.3.0",
			wantImports:    pkg3.Imports,
			wantImportedBy: nil,
		},
		{
			path:           pkg2.Path,
			version:        "v1.2.0",
			wantImports:    pkg2.Imports,
			wantImportedBy: []string{pkg3.Path},
		},
		{
			path:           pkg1.Path,
			version:        "v1.1.0",
			wantImports:    nil,
			wantImportedBy: []string{pkg2.Path, pkg3.Path},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for _, v := range testVersions {
				if err := testDB.InsertVersion(ctx, v, SampleLicenses); err != nil {
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

			gotImportedBy, err := testDB.GetImportedBy(ctx, tc.path)
			if err != nil {
				t.Fatalf("testDB.GetImports(%q, %q): %v", tc.path, tc.version, err)
			}

			if diff := cmp.Diff(tc.wantImportedBy, gotImportedBy); diff != "" {
				t.Errorf("testDB.GetImportedBy(%q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}

func TestPostgres_GetTaggedAndPseudoVersionsForPackageSeries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		now  = NowTruncated()
		pkg1 = &internal.Package{
			Path:     "path.to/foo/bar",
			Name:     "bar",
			Synopsis: "This is a package synopsis",
			V1Path:   "bar",
		}
		pkg2 = &internal.Package{
			Path:     "path.to/foo/v2/bar",
			Name:     "bar",
			Synopsis: "This is another package synopsis",
			V1Path:   "bar",
		}
		pkg3 = &internal.Package{
			Path:     "path.to/some/thing/else",
			Name:     "else",
			Synopsis: "something else's package synopsis",
			V1Path:   "else",
		}
		modulePath1  = "path.to/foo"
		modulePath2  = "path.to/foo/v2"
		modulePath3  = "path.to/some/thing"
		testVersions = []*internal.Version{
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:  modulePath3,
					Version:     "v3.0.0",
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
				},
				Packages: []*internal.Package{pkg3},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:  modulePath1,
					Version:     "v1.0.0-alpha.1",
					CommitTime:  now,
					VersionType: internal.VersionTypePrerelease,
				},
				Packages: []*internal.Package{pkg1},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:  modulePath1,
					Version:     "v1.0.0",
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
				},
				Packages: []*internal.Package{pkg1},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:  modulePath2,
					Version:     "v2.0.1-beta",
					CommitTime:  now,
					VersionType: internal.VersionTypePrerelease,
				},
				Packages: []*internal.Package{pkg2},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					ModulePath:  modulePath2,
					Version:     "v2.1.0",
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
				},
				Packages: []*internal.Package{pkg2},
			},
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
				&internal.VersionInfo{
					ModulePath: modulePath2,
					Version:    "v2.1.0",
					CommitTime: now,
				},
				&internal.VersionInfo{
					ModulePath: modulePath2,
					Version:    "v2.0.1-beta",
					CommitTime: now,
				},
				&internal.VersionInfo{
					ModulePath: modulePath1,
					Version:    "v1.0.0",
					CommitTime: now,
				},
				&internal.VersionInfo{
					ModulePath: modulePath1,
					Version:    "v1.0.0-alpha.1",
					CommitTime: now,
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
				v := &internal.Version{
					VersionInfo: internal.VersionInfo{
						ModulePath: modulePath1,
						// %02d makes a string that is a width of 2 and left pads with zeroes
						Version:     fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", i+1),
						CommitTime:  now,
						VersionType: internal.VersionTypePseudo,
					},
					Packages: []*internal.Package{pkg1},
				}
				if err := testDB.InsertVersion(ctx, v, nil); err != nil {
					t.Errorf("testDB.InsertVersion(%v): %v", v, err)
				}

				// GetPseudoVersions should only return the 10 most recent pseudo versions,
				// if there are more than 10 in the database
				if i < 10 {
					wantPseudoVersions = append(wantPseudoVersions, &internal.VersionInfo{
						ModulePath: modulePath1,
						Version:    fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", tc.numPseudo-i),
						CommitTime: now,
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

func TestGetVersionForPackage(t *testing.T) {
	var (
		now         = NowTruncated()
		modulePath  = "test.module"
		testVersion = &internal.Version{
			VersionInfo: internal.VersionInfo{
				ModulePath:     modulePath,
				Version:        "v1.0.0",
				ReadmeContents: []byte("readme"),
				CommitTime:     now,
				VersionType:    internal.VersionTypeRelease,
			},
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "testmodule",
					Synopsis: "This is a package synopsis",
					Path:     "test.module",
				},
				&internal.Package{
					Name:     "foo",
					Synopsis: "This is a package synopsis",
					Path:     "test.module/foo",
					Licenses: SampleLicenseMetadata,
				},
			},
		}
	)

	for _, tc := range []struct {
		name, path, version string
		wantVersion         *internal.Version
	}{
		{
			name:        "version_with_multi_packages",
			path:        "test.module/foo",
			version:     testVersion.Version,
			wantVersion: testVersion,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			if err := testDB.InsertVersion(ctx, tc.wantVersion, SampleLicenses); err != nil {
				t.Errorf("testDB.InsertVersion(ctx, %q %q): %v", tc.path, tc.version, err)
			}

			got, err := testDB.GetVersionForPackage(ctx, tc.path, tc.version)
			if err != nil {
				t.Errorf("testDB.GetVersionForPackage(ctx, %q, %q): %v", tc.path, tc.version, err)
			}
			if diff := cmp.Diff(tc.wantVersion, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("testDB.GetVersionForPackage(ctx, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}

func TestGetLicenses(t *testing.T) {
	var (
		now         = NowTruncated()
		modulePath  = "test.module"
		testVersion = &internal.Version{
			VersionInfo: internal.VersionInfo{
				ModulePath:     modulePath,
				Version:        "v1.0.0",
				ReadmeContents: []byte("readme"),
				CommitTime:     now,
				VersionType:    internal.VersionTypeRelease,
			},
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Synopsis: "This is a package synopsis",
					Path:     "test.module/foo",
					Licenses: SampleLicenseMetadata,
				},
				&internal.Package{
					Name:     "testmodule",
					Synopsis: "This is a package synopsis",
					Path:     "test.module",
				},
			},
		}
	)

	tests := []struct {
		label, pkgPath string
		wantLicenses   []*license.License
	}{
		{
			label:        "package with licenses",
			pkgPath:      "test.module/foo",
			wantLicenses: SampleLicenses,
		}, {
			label:        "package with no licenses",
			pkgPath:      "test.module",
			wantLicenses: nil,
		},
	}

	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := testDB.InsertVersion(ctx, testVersion, SampleLicenses); err != nil {
		t.Errorf("testDB.InsertVersion(ctx, %q, licenses): %v", testVersion.Version, err)
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			got, err := testDB.GetLicenses(ctx, test.pkgPath, testVersion.Version)
			if err != nil {
				t.Fatalf("testDB.GetLicenses(ctx, %q, %q): %v", test.pkgPath, testVersion.Version, err)
			}
			if diff := cmp.Diff(test.wantLicenses, got); diff != "" {
				t.Errorf("testDB.GetLicenses(ctx, %q, %q) mismatch (-want +got):\n%s", test.pkgPath, testVersion.Version, diff)
			}
		})
	}
}
