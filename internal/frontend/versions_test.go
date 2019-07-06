// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

func sampleVersion(pkgPath, modulePath, version string, versionType internal.VersionType, packages ...*internal.Package) *internal.Version {
	return &internal.Version{
		VersionInfo: internal.VersionInfo{
			ModulePath:     modulePath,
			Version:        version,
			CommitTime:     time.Now().Add(time.Hour * -8),
			ReadmeContents: []byte("This is the readme text."),
			VersionType:    versionType,
		},
		Packages: packages,
	}
}

func TestFetchVersionsDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		modulePath1 = "test.com/module"
		modulePath2 = "test.com/module/v2"
		pkg1Path    = "test.com/module/pkg_name"
		pkg2Path    = "test.com/module/v2/pkg_name"
		pkg1        = &internal.VersionedPackage{
			Package: internal.Package{
				Name:     "pkg_name",
				Path:     pkg2Path,
				Synopsis: "test synopsis",
				Licenses: postgres.SampleLicenseMetadata,
				V1Path:   pkg1Path,
			},
			VersionInfo: internal.VersionInfo{
				ModulePath: "test.com/module",
				Version:    "v1.2.1",
			},
		}
		pkg2 = &internal.VersionedPackage{
			Package: internal.Package{
				Name:     "pkg_name",
				Path:     pkg2Path,
				Synopsis: "test synopsis",
				Licenses: postgres.SampleLicenseMetadata,
				V1Path:   pkg1Path,
			},
			VersionInfo: internal.VersionInfo{
				ModulePath: "test.com/module/v2",
				Version:    "v2.2.1-alpha.1",
			},
		}
		nethttpPkg = &internal.VersionedPackage{
			Package: internal.Package{
				Name:     "http",
				Path:     "net/http",
				Synopsis: "test synopsis",
				Licenses: postgres.SampleLicenseMetadata,
				V1Path:   "http",
			},
			VersionInfo: internal.VersionInfo{
				ModulePath: "std",
				Version:    "v1.12.5",
			},
		}
	)

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
				sampleVersion("net/http", "std", "v1.12.5", internal.VersionTypeRelease, &nethttpPkg.Package),
				sampleVersion("net/http", "std", "v1.11.6", internal.VersionTypeRelease, &nethttpPkg.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*ModuleVersion{
					{
						Major:      "v1",
						ModulePath: "std",
						Versions: [][]*PackageVersion{
							{
								{
									Version:          "v1.12.5",
									FormattedVersion: "v1.12.5",
									Path:             "net/http",
									CommitTime:       "today",
								},
							},
							{
								{
									Version:          "v1.11.6",
									FormattedVersion: "v1.11.6",
									Path:             "net/http",
									CommitTime:       "today",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "want v1 first",
			pkg:  pkg1,
			versions: []*internal.Version{
				sampleVersion(pkg1Path, modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo, &pkg2.Package),
				sampleVersion(pkg1Path, modulePath1, "v1.2.1", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(pkg1Path, modulePath1, "v1.2.3", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(pkg1Path, modulePath1, "v1.3.0", internal.VersionTypeRelease, &pkg1.Package),
				sampleVersion(pkg2Path, modulePath2, "v2.0.0", internal.VersionTypeRelease, &pkg2.Package),
				sampleVersion(pkg2Path, modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease, &pkg2.Package),
				&internal.Version{
					VersionInfo: internal.VersionInfo{
						ModulePath:  "test.com",
						Version:     "v1.2.1",
						CommitTime:  time.Now().Add(time.Hour * -7),
						VersionType: internal.VersionTypeRelease,
					},
					Packages: []*internal.Package{&pkg1.Package},
				},
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*ModuleVersion{
					{
						Major:      "v1",
						ModulePath: "test.com/module",
						Versions: [][]*PackageVersion{
							{
								{
									Version:          "v1.3.0",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v1.3.0",
								},
							},
							{
								{
									Version:          "v1.2.3",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v1.2.3",
								},
								{
									Version:          "v1.2.1",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v1.2.1",
								},
							},
						},
					},
				},
				OtherModules: []*ModuleVersion{
					{
						Major:      "v2",
						ModulePath: "test.com/module/v2",
						Versions: [][]*PackageVersion{
							{
								{
									Version:          "v2.2.1-alpha.1",
									FormattedVersion: "v2.2.1 (alpha.1)",
									Path:             "test.com/module/v2/pkg_name",
									CommitTime:       "today",
								},
							},
							{
								{
									Version:          "v2.0.0",
									FormattedVersion: "v2.0.0",
									Path:             "test.com/module/v2/pkg_name",
									CommitTime:       "today",
								},
							},
						},
					},
					{
						Major:      "v1",
						ModulePath: "test.com",
						Versions: [][]*PackageVersion{
							{
								{
									Version:          "v1.2.1",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v1.2.1",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "want v2 first",
			pkg:  pkg2,
			versions: []*internal.Version{
				sampleVersion(pkg1Path, modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo, &pkg2.Package),
				sampleVersion(pkg1Path, modulePath1, "v1.2.1", internal.VersionTypeRelease, &pkg2.Package),
				sampleVersion(pkg1Path, modulePath1, "v1.2.3", internal.VersionTypeRelease, &pkg2.Package),
				sampleVersion(pkg1Path, modulePath1, "v1.3.0", internal.VersionTypeRelease, &pkg2.Package),
				sampleVersion(pkg2Path, modulePath2, "v2.0.0", internal.VersionTypeRelease, &pkg2.Package),
				sampleVersion(pkg2Path, modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease, &pkg2.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*ModuleVersion{
					{
						Major:      "v2",
						ModulePath: "test.com/module/v2",
						Versions: [][]*PackageVersion{
							{
								{
									Version:          "v2.2.1-alpha.1",
									FormattedVersion: "v2.2.1 (alpha.1)",
									Path:             "test.com/module/v2/pkg_name",
									CommitTime:       "today",
								},
							},
							{
								{
									Version:          "v2.0.0",
									FormattedVersion: "v2.0.0",
									Path:             "test.com/module/v2/pkg_name",
									CommitTime:       "today",
								},
							},
						},
					},
				},
				OtherModules: []*ModuleVersion{
					{
						Major:      "v1",
						ModulePath: "test.com/module",
						Versions: [][]*PackageVersion{
							{
								{
									Version:          "v1.3.0",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v1.3.0",
								},
							},
							{
								{
									Version:          "v1.2.3",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v1.2.3",
								},
								{
									Version:          "v1.2.1",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v1.2.1",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "want only pseudo",
			pkg:  pkg2,
			versions: []*internal.Version{
				sampleVersion(pkg1Path, modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", internal.VersionTypePseudo, &pkg2.Package),
				sampleVersion(pkg1Path, modulePath1, "v0.0.0-20140414041502-4c2ca4d52544", internal.VersionTypePseudo, &pkg2.Package),
			},
			wantDetails: &VersionsDetails{
				OtherModules: []*ModuleVersion{
					{
						Major:      "v0",
						ModulePath: "test.com/module",
						Versions: [][]*PackageVersion{
							{
								{
									Version:          "v0.0.0-20140414041502-4c2ca4d52544",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v0.0.0 (4c2ca4d)",
								},
								{
									Version:          "v0.0.0-20140414041501-3c2ca4d52544",
									Path:             "test.com/module/pkg_name",
									CommitTime:       "today",
									FormattedVersion: "v0.0.0 (3c2ca4d)",
								},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v, postgres.SampleLicenses); err != nil {
					t.Fatalf("db.InsertVersion(%v): %v", v, err)
				}
			}

			got, err := fetchVersionsDetails(ctx, testDB, tc.pkg)
			if err != nil {
				t.Fatalf("fetchVersionsDetails(ctx, db, %v): %v", tc.pkg, err)
			}
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchVersionsDetails(ctx, db, %v) mismatch (-want +got):\n%s", tc.pkg, diff)
			}
		})
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
