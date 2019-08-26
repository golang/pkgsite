// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/sample"
)

var (
	modulePath1 = "test.com/module"
	modulePath2 = "test.com/module/v2"
	commitTime  = "0 hours ago"
)

func sampleVersion(modulePath, version string, versionType internal.VersionType, packages ...*internal.Package) *internal.Version {
	v := sample.Version()
	v.ModulePath = modulePath
	v.Version = version
	v.VersionType = versionType
	if len(packages) > 0 {
		v.Packages = packages
	}
	return v
}

func versionSummaries(path string, versions [][]string, linkify func(path, version string) string) [][]*VersionSummary {
	vs := make([][]*VersionSummary, len(versions))
	for i, pointVersions := range versions {
		vs[i] = make([]*VersionSummary, len(pointVersions))
		for j, version := range pointVersions {
			vs[i][j] = &VersionSummary{
				Version:          version,
				FormattedVersion: formatVersion(version),
				Link:             linkify(path, version),
				CommitTime:       commitTime,
			}
		}
	}
	return vs
}

func TestFetchModuleVersionDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	info1 := sample.VersionInfo()
	info1.ModulePath = modulePath1
	info1.Version = "v1.2.1"

	info2 := sample.VersionInfo()
	info2.ModulePath = modulePath2
	info2.Version = "v2.2.1-alpha.1"

	moduleVersionSummaries := func(path string, versions [][]string) [][]*VersionSummary {
		return versionSummaries(path, versions, func(path, version string) string {
			return fmt.Sprintf("/mod/%s@%s", path, version)
		})
	}

	for _, tc := range []struct {
		name        string
		info        *internal.VersionInfo
		versions    []*internal.Version
		wantDetails *VersionsDetails
	}{
		{
			name: "want v1 first",
			info: info1,
			versions: []*internal.Version{
				sampleVersion(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo),
				sampleVersion(modulePath1, "v1.2.3", internal.VersionTypeRelease),
				sampleVersion(modulePath2, "v2.0.0", internal.VersionTypeRelease),
				sampleVersion(modulePath1, "v1.3.0", internal.VersionTypeRelease),
				sampleVersion(modulePath1, "v1.2.1", internal.VersionTypeRelease),
				sampleVersion(modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*MajorVersionGroup{
					{
						Major:      "v1",
						ModulePath: "test.com/module",
						Versions: moduleVersionSummaries(modulePath1, [][]string{
							{"v1.3.0"},
							{"v1.2.3", "v1.2.1"},
						}),
					},
				},
				OtherModules: []*MajorVersionGroup{
					{
						Major:      "v2",
						ModulePath: "test.com/module/v2",
						Versions: moduleVersionSummaries(modulePath2, [][]string{
							{"v2.2.1-alpha.1"},
							{"v2.0.0"},
						}),
					},
				},
			},
		},
		{
			name: "want v2 first",
			info: info2,
			versions: []*internal.Version{
				sampleVersion(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo),
				sampleVersion(modulePath1, "v1.2.1", internal.VersionTypeRelease),
				sampleVersion(modulePath1, "v1.2.3", internal.VersionTypeRelease),
				sampleVersion(modulePath1, "v2.1.0+incompatible", internal.VersionTypeRelease),
				sampleVersion(modulePath2, "v2.0.0", internal.VersionTypeRelease),
				sampleVersion(modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*MajorVersionGroup{
					{
						Major:      "v2",
						ModulePath: "test.com/module/v2",
						Versions: moduleVersionSummaries(modulePath2, [][]string{
							{"v2.2.1-alpha.1"},
							{"v2.0.0"},
						}),
					},
				},
				OtherModules: []*MajorVersionGroup{
					{
						Major:      "v1",
						ModulePath: "test.com/module",
						Versions: moduleVersionSummaries(modulePath1, [][]string{
							{"v2.1.0+incompatible"},
							{"v1.2.3", "v1.2.1"},
						}),
					},
				},
			},
		},
		{
			name: "want only pseudo",
			info: info2,
			versions: []*internal.Version{
				sampleVersion(modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", internal.VersionTypePseudo),
				sampleVersion(modulePath1, "v0.0.0-20140414041502-4c2ca4d52544", internal.VersionTypePseudo),
			},
			wantDetails: &VersionsDetails{
				OtherModules: []*MajorVersionGroup{
					{
						Major:      "v0",
						ModulePath: "test.com/module",
						Versions: moduleVersionSummaries(modulePath1, [][]string{
							{"v0.0.0-20140414041502-4c2ca4d52544", "v0.0.0-20140414041501-3c2ca4d52544"},
						}),
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v); err != nil {
					t.Fatal(err)
				}
			}

			got, err := fetchModuleVersionsDetails(ctx, testDB, tc.info)
			if err != nil {
				t.Fatalf("fetchModuleVersionsDetails(ctx, db, %v): %v", tc.info, err)
			}
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchModuleVersionsDetails(ctx, db, %v) mismatch (-want +got):\n%s", tc.info, diff)
			}
		})
	}
}

func TestFetchPackageVersionsDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		v2Path = "test.com/module/v2/foo"
		v1Path = "test.com/module/foo"
	)

	pkg1 := sample.VersionedPackage()
	pkg1.Path = v1Path
	pkg1.V1Path = v1Path
	pkg1.ModulePath = modulePath1
	pkg1.Version = "v1.2.1"

	pkg2 := sample.VersionedPackage()
	pkg2.Path = v2Path
	pkg2.V1Path = v1Path
	pkg2.ModulePath = modulePath2
	pkg2.Version = "v2.2.1-alpha.1"

	nethttpPkg := sample.VersionedPackage()
	nethttpPkg.Path = "net/http"
	nethttpPkg.V1Path = "net/http"
	nethttpPkg.ModulePath = "std"
	nethttpPkg.Version = "v1.12.5"

	packageVersionSummaries := func(path string, versions [][]string) [][]*VersionSummary {
		return versionSummaries(path, versions, func(path, version string) string {
			return fmt.Sprintf("/pkg/%s@%s", path, version)
		})
	}

	for _, tc := range []struct {
		name        string
		pkg         *internal.VersionedPackage
		versions    []*internal.Version
		wantDetails *VersionsDetails
	}{
		{
			name: "want stdlib versions",
			pkg:  nethttpPkg,
			versions: []*internal.Version{
				sampleVersion("std", "v1.12.5", internal.VersionTypeRelease, &nethttpPkg.Package),
				sampleVersion("std", "v1.11.6", internal.VersionTypeRelease, &nethttpPkg.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*MajorVersionGroup{
					{
						Major:      "v1",
						ModulePath: "std",
						Versions: packageVersionSummaries("net/http", [][]string{
							{"v1.12.5"},
							{"v1.11.6"},
						}),
					},
				},
			},
		},
		{
			name: "want v1 first",
			pkg:  pkg1,
			versions: []*internal.Version{
				sampleVersion(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo, &pkg2.Package),
				sampleVersion(modulePath1, "v1.2.3", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(modulePath2, "v2.0.0", internal.VersionTypeRelease, &pkg2.Package),
				sampleVersion(modulePath1, "v1.3.0", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(modulePath1, "v1.2.1", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease, &pkg2.Package),
				sampleVersion("test.com", "v1.2.1", internal.VersionTypeRelease, &pkg1.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*MajorVersionGroup{
					{
						Major:      "v1",
						ModulePath: "test.com/module",
						Versions: packageVersionSummaries(v1Path, [][]string{
							{"v1.3.0"},
							{"v1.2.3", "v1.2.1"},
						}),
					},
				},
				OtherModules: []*MajorVersionGroup{
					{
						Major:      "v2",
						ModulePath: "test.com/module/v2",
						Versions: packageVersionSummaries(v2Path, [][]string{
							{"v2.2.1-alpha.1"},
							{"v2.0.0"},
						}),
					},
					{
						Major:      "v1",
						ModulePath: "test.com",
						Versions: packageVersionSummaries(v1Path, [][]string{
							{"v1.2.1"},
						}),
					},
				},
			},
		},
		{
			name: "want v2 first",
			pkg:  pkg2,
			versions: []*internal.Version{
				sampleVersion(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo, &pkg1.Package),
				sampleVersion(modulePath1, "v1.2.1", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(modulePath1, "v1.2.3", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(modulePath1, "v2.1.0+incompatible", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(modulePath2, "v2.0.0", internal.VersionTypeRelease, &pkg2.Package),
				sampleVersion(modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease, &pkg2.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*MajorVersionGroup{
					{
						Major:      "v2",
						ModulePath: "test.com/module/v2",
						Versions: packageVersionSummaries(v2Path, [][]string{
							{"v2.2.1-alpha.1"},
							{"v2.0.0"},
						}),
					},
				},
				OtherModules: []*MajorVersionGroup{
					{
						Major:      "v1",
						ModulePath: "test.com/module",
						Versions: packageVersionSummaries(v1Path, [][]string{
							{"v2.1.0+incompatible"},
							{"v1.2.3", "v1.2.1"},
						}),
					},
				},
			},
		},
		{
			name: "want only pseudo",
			pkg:  pkg2,
			versions: []*internal.Version{
				sampleVersion(modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", internal.VersionTypePseudo, &pkg2.Package),
				sampleVersion(modulePath1, "v0.0.0-20140414041502-4c2ca4d52544", internal.VersionTypePseudo, &pkg2.Package),
			},
			wantDetails: &VersionsDetails{
				OtherModules: []*MajorVersionGroup{
					{
						Major:      "v0",
						ModulePath: "test.com/module",
						Versions: packageVersionSummaries(v1Path, [][]string{
							{"v0.0.0-20140414041502-4c2ca4d52544", "v0.0.0-20140414041501-3c2ca4d52544"},
						}),
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v); err != nil {
					t.Fatal(err)
				}
			}

			got, err := fetchPackageVersionsDetails(ctx, testDB, tc.pkg)
			if err != nil {
				t.Fatalf("fetchPackageVersionsDetails(ctx, db, %v): %v", tc.pkg, err)
			}
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchPackageVersionsDetails(ctx, db, %v) mismatch (-want +got):\n%s", tc.pkg, diff)
			}
		})
	}
}

func TestPathInVersion(t *testing.T) {
	tests := []struct {
		v1Path, modulePath, want string
	}{
		{"foo.com/bar/baz", "foo.com/bar", "foo.com/bar/baz"},
		{"foo.com/bar/baz", "foo.com/bar/v2", "foo.com/bar/v2/baz"},
		{"foo.com/bar/baz", "foo.com/v3", "foo.com/v3/bar/baz"},
		{"foo.com/bar/baz", "foo.com/bar/baz/v3", "foo.com/bar/baz/v3"},
	}

	for _, test := range tests {
		vi := sample.VersionInfo()
		vi.ModulePath = test.modulePath
		if got := pathInVersion(test.v1Path, vi); got != test.want {
			t.Errorf("pathInVersion(%q, VersionInfo{...ModulePath:%q}) = %s, want %v",
				test.v1Path, vi.ModulePath, got, test.want)
		}
	}
}

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		version, want string
	}{
		{"v1.2.3", "v1.2.3"},
		{"v2.0.0", "v2.0.0"},
		{"v1.2.3-alpha.1", "v1.2.3 (alpha.1)"},
		{"v1.0.0-20190311183353-d8887717615a", "v1.0.0 (d888771)"},
		{"v1.2.3-pre.0.20190311183353-d8887717615a", "v1.2.3 (d888771)"},
		{"v1.2.4-0.20190311183353-d8887717615a", "v1.2.4 (d888771)"},
		{"v1.0.0-20190311183353-d88877", "v1.0.0 (d88877)"},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			if got := formatVersion(test.version); got != test.want {
				t.Errorf("formatVersion(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}
