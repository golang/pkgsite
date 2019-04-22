// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

const testTimeout = 5 * time.Second

var (
	sampleLicenseInfos = []*internal.LicenseInfo{
		{Type: "MIT", FilePath: "LICENSE"},
	}
	sampleLicenses = []*internal.License{
		{LicenseInfo: *sampleLicenseInfos[0], Contents: []byte("Lorem Ipsum")},
	}

	samplePackageHeader = &Package{
		Name:       "pkg_name",
		Title:      "Package pkg_name",
		Version:    "v1.0.0",
		Path:       "test.com/module/pkg_name",
		Synopsis:   "Test package synopsis",
		Licenses:   transformLicenseInfos(sampleLicenseInfos),
		CommitTime: "today",
	}
	sampleInternalPackage = &internal.Package{
		Name:     "pkg_name",
		Path:     "test.com/module/pkg_name",
		Synopsis: "Test package synopsis",
		Licenses: sampleLicenseInfos,
	}
	sampleInternalVersion = &internal.Version{
		VersionInfo: internal.VersionInfo{
			SeriesPath:  "series",
			ModulePath:  "test.com/module",
			Version:     "v1.0.0",
			CommitTime:  time.Now().Add(time.Hour * -8),
			ReadMe:      []byte("This is the readme text."),
			VersionType: internal.VersionTypeRelease,
		},
		Packages: []*internal.Package{sampleInternalPackage},
	}
	samplePackage = &Package{
		Name:       "pkg_name",
		ModulePath: "test.com/module",
		Version:    "v1.0.0",
		Path:       "test.com/module/pkg_name",
		Synopsis:   "Test package synopsis",
		Licenses:   transformLicenseInfos(sampleLicenseInfos),
	}
)

func TestElapsedTime(t *testing.T) {
	now := postgres.NowTruncated()
	testCases := []struct {
		name        string
		date        time.Time
		elapsedTime string
	}{
		{
			name:        "one_hour_ago",
			date:        now.Add(time.Hour * -1),
			elapsedTime: "1 hour ago",
		},
		{
			name:        "hours_ago",
			date:        now.Add(time.Hour * -2),
			elapsedTime: "2 hours ago",
		},
		{
			name:        "today",
			date:        now.Add(time.Hour * -8),
			elapsedTime: "today",
		},
		{
			name:        "one_day_ago",
			date:        now.Add(time.Hour * 24 * -1),
			elapsedTime: "1 day ago",
		},
		{
			name:        "days_ago",
			date:        now.Add(time.Hour * 24 * -5),
			elapsedTime: "5 days ago",
		},
		{
			name:        "more_than_6_days_ago",
			date:        now.Add(time.Hour * 24 * -14),
			elapsedTime: now.Add(time.Hour * 24 * -14).Format("Jan _2, 2006"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			elapsedTime := elapsedTime(tc.date)

			if elapsedTime != tc.elapsedTime {
				t.Errorf("elapsedTime(%q) = %s, want %s", tc.date, elapsedTime, tc.elapsedTime)
			}
		})
	}
}

func TestFetchOverviewDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name            string
		version         *internal.Version
		wantDetailsPage *DetailsPage
	}{
		name:    "want expected overview details",
		version: sampleInternalVersion,
		wantDetailsPage: &DetailsPage{
			Details: &OverviewDetails{
				ModulePath: "test.com/module",
				ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
			},
			PackageHeader: samplePackageHeader,
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(ctx, tc.version, sampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(%v): %v", tc.version, err)
	}

	got, err := fetchOverviewDetails(ctx, db, tc.version.Packages[0].Path, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchOverviewDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetailsPage)
	}

	if diff := cmp.Diff(tc.wantDetailsPage, got); diff != "" {
		t.Errorf("fetchOverviewDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func getTestVersion(pkgPath, modulePath, version string, versionType internal.VersionType) *internal.Version {
	suffix := strings.TrimPrefix(strings.TrimPrefix(pkgPath, modulePath), "/")
	return &internal.Version{
		VersionInfo: internal.VersionInfo{
			SeriesPath:  "test.com/module",
			ModulePath:  modulePath,
			Version:     version,
			CommitTime:  time.Now().Add(time.Hour * -8),
			ReadMe:      []byte("This is the readme text."),
			VersionType: versionType,
		},
		Packages: []*internal.Package{
			&internal.Package{
				Name:     "pkg_name",
				Path:     pkgPath,
				Synopsis: "test synopsis",
				Licenses: sampleLicenseInfos,
				Suffix:   suffix,
			},
		},
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
	)

	for _, tc := range []struct {
		name, path, version string
		versions            []*internal.Version
		wantDetailsPage     *DetailsPage
	}{
		{
			name:    "want tagged versions",
			path:    pkg1Path,
			version: "v1.2.1",
			versions: []*internal.Version{
				getTestVersion(pkg1Path, modulePath1, "v1.2.3", internal.VersionTypeRelease),
				getTestVersion(pkg2Path, modulePath2, "v2.0.0", internal.VersionTypeRelease),
				getTestVersion(pkg1Path, modulePath1, "v1.3.0", internal.VersionTypeRelease),
				getTestVersion(pkg1Path, modulePath1, "v1.2.1", internal.VersionTypeRelease),
				getTestVersion(pkg1Path, modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo),
				getTestVersion(pkg2Path, modulePath2, "v2.2.1-alpha.1", internal.VersionTypePrerelease),
			},
			wantDetailsPage: &DetailsPage{
				Details: &VersionsDetails{
					Versions: []*MajorVersionGroup{
						&MajorVersionGroup{
							Level: "v2",
							Latest: &Package{
								Version:    "2.2.1-alpha.1",
								Path:       pkg2Path,
								CommitTime: "today",
							},
							Versions: []*MinorVersionGroup{
								&MinorVersionGroup{
									Level: "2.2",
									Latest: &Package{
										Version:    "2.2.1-alpha.1",
										Path:       pkg2Path,
										CommitTime: "today",
									},
									Versions: []*Package{
										&Package{
											Version:    "2.2.1-alpha.1",
											Path:       pkg2Path,
											CommitTime: "today",
										},
									},
								},
								&MinorVersionGroup{
									Level: "2.0",
									Latest: &Package{
										Version:    "2.0.0",
										Path:       pkg2Path,
										CommitTime: "today",
									},
									Versions: []*Package{
										&Package{
											Version:    "2.0.0",
											Path:       pkg2Path,
											CommitTime: "today",
										},
									},
								},
							},
						},
						&MajorVersionGroup{
							Level: "v1",
							Latest: &Package{
								Version:    "1.3.0",
								Path:       pkg1Path,
								CommitTime: "today",
							},
							Versions: []*MinorVersionGroup{
								&MinorVersionGroup{
									Level: "1.3",
									Latest: &Package{
										Version:    "1.3.0",
										Path:       pkg1Path,
										CommitTime: "today",
									},
									Versions: []*Package{
										&Package{
											Version:    "1.3.0",
											Path:       pkg1Path,
											CommitTime: "today",
										},
									},
								},
								&MinorVersionGroup{
									Level: "1.2",
									Latest: &Package{
										Version:    "1.2.3",
										Path:       pkg1Path,
										CommitTime: "today",
									},
									Versions: []*Package{
										&Package{
											Version:    "1.2.3",
											Path:       pkg1Path,
											CommitTime: "today",
										},
										&Package{
											Version:    "1.2.1",
											Path:       pkg1Path,
											CommitTime: "today",
										},
									},
								},
							},
						},
					},
				},
				PackageHeader: &Package{
					Name:       "pkg_name",
					Title:      "Package pkg_name",
					Version:    "v1.2.1",
					Path:       pkg1Path,
					Synopsis:   "test synopsis",
					Licenses:   transformLicenseInfos([]*internal.LicenseInfo{{Type: "MIT", FilePath: "LICENSE"}}),
					CommitTime: "today",
				},
			},
		},
		{
			name:    "want only pseudo",
			path:    pkg1Path,
			version: "v0.0.0-20140414041501-3c2ca4d52544",
			versions: []*internal.Version{
				getTestVersion(pkg1Path, modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", internal.VersionTypePseudo),
				getTestVersion(pkg1Path, modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo),
			},
			wantDetailsPage: &DetailsPage{
				Details: &VersionsDetails{
					Versions: []*MajorVersionGroup{
						&MajorVersionGroup{
							Level: "v0",
							Latest: &Package{
								Version:    "0.0.0-20140414041502-3c2ca4d52544",
								Path:       pkg1Path,
								CommitTime: "today",
							},
							Versions: []*MinorVersionGroup{
								&MinorVersionGroup{
									Level: "0.0",
									Latest: &Package{
										Version:    "0.0.0-20140414041502-3c2ca4d52544",
										Path:       pkg1Path,
										CommitTime: "today",
									},
									Versions: []*Package{
										&Package{
											Version:    "0.0.0-20140414041502-3c2ca4d52544",
											Path:       pkg1Path,
											CommitTime: "today",
										},
										&Package{
											Version:    "0.0.0-20140414041501-3c2ca4d52544",
											Path:       pkg1Path,
											CommitTime: "today",
										},
									},
								},
							},
						},
					},
				},
				PackageHeader: &Package{
					Name:       "pkg_name",
					Title:      "Package pkg_name",
					Version:    "v0.0.0-20140414041501-3c2ca4d52544",
					Path:       pkg1Path,
					Synopsis:   "test synopsis",
					Licenses:   transformLicenseInfos([]*internal.LicenseInfo{{Type: "MIT", FilePath: "LICENSE"}}),
					CommitTime: "today",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownDB, db := postgres.SetupCleanDB(t)
			defer teardownDB(t)

			for _, v := range tc.versions {
				if err := db.InsertVersion(ctx, v, sampleLicenses); err != nil {
					t.Fatalf("db.InsertVersion(%v): %v", v, err)
				}
			}

			got, err := fetchVersionsDetails(ctx, db, tc.path, tc.version)
			if err != nil {
				t.Fatalf("fetchVersionsDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					tc.path, tc.version, got, err, tc.wantDetailsPage)
			}

			if diff := cmp.Diff(tc.wantDetailsPage, got); diff != "" {
				t.Errorf("fetchVersionsDetails(ctx, db, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}

func TestReadmeHTML(t *testing.T) {
	testCases := []struct {
		name, readme string
		want         template.HTML
	}{
		{
			name: "valid_markdown_readme",
			readme: "This package collects pithy sayings.\n\n" +
				"It's part of a demonstration of\n" +
				"[package versioning in Go](https://research.swtch.com/vgo1).",
			want: template.HTML("<p>This package collects pithy sayings.</p>\n\n" +
				"<p>It’s part of a demonstration of\n" +
				`<a href="https://research.swtch.com/vgo1" rel="nofollow">package versioning in Go</a>.</p>` + "\n"),
		},
		{
			name:   "empty_readme",
			readme: "",
			want:   template.HTML(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := readmeHTML([]byte(tc.readme))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("readmeHTML(%q) mismatch (-want +got):\n%s", tc.readme, diff)
			}
		})
	}
}

func TestFetchModuleDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name            string
		version         *internal.Version
		wantDetailsPage *DetailsPage
	}{
		name:    "want expected module details",
		version: sampleInternalVersion,
		wantDetailsPage: &DetailsPage{
			Details: &ModuleDetails{
				ModulePath: "test.com/module",
				ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
				Version:    "v1.0.0",
				Packages:   []*Package{samplePackage},
			},
			PackageHeader: samplePackageHeader,
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(ctx, tc.version, sampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", tc.version, sampleLicenses, err)
	}

	got, err := fetchModuleDetails(ctx, db, tc.version.Packages[0].Path, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchModuleDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetailsPage)
	}

	if diff := cmp.Diff(tc.wantDetailsPage, got); diff != "" {
		t.Errorf("fetchModuleDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestCreatePackageHeader(t *testing.T) {
	versionInfo := internal.VersionInfo{
		Version: "v1.0.0",
	}
	for _, tc := range []struct {
		pkg     *internal.VersionedPackage
		wantPkg *Package
	}{
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "foo",
					Path: "pa.th/to/foo",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Package foo",
				Path:       "pa.th/to/foo",
				CommitTime: "Jan  1, 0001",
				IsCommand:  false,
			},
		},
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "main",
					Path: "pa.th/to/foo",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Command foo",
				Path:       "pa.th/to/foo",
				CommitTime: "Jan  1, 0001",
				IsCommand:  true,
			},
		},
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "main",
					Path: "pa.th/to/foo/v2",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Command foo",
				Path:       "pa.th/to/foo/v2",
				CommitTime: "Jan  1, 0001",
				IsCommand:  true,
			},
		},
		{
			pkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name: "main",
					Path: "pa.th/to/foo/v1",
				},
				VersionInfo: versionInfo,
			},
			wantPkg: &Package{
				Version:    versionInfo.Version,
				Name:       "foo",
				Title:      "Command foo",
				Path:       "pa.th/to/foo/v1",
				CommitTime: "Jan  1, 0001",
				IsCommand:  true,
			},
		},
	} {

		t.Run(tc.pkg.Path, func(t *testing.T) {
			got, err := createPackageHeader(tc.pkg)
			if err != nil {
				t.Fatalf("createPackageHeader(%v): %v", tc.pkg, err)
			}

			if diff := cmp.Diff(tc.wantPkg, got); diff != "" {
				t.Errorf("createPackageHeader(%v) mismatch (-want +got):\n%s", tc.pkg, diff)
			}

		})
	}
}

func TestFetchImportsDetails(t *testing.T) {
	for _, tc := range []struct {
		name            string
		imports         []*internal.Import
		wantDetailsPage *DetailsPage
	}{
		{
			name: "want imports details with standard",
			imports: []*internal.Import{
				{Name: "import1", Path: "pa.th/import/1"},
				{Name: "context", Path: "context"},
			},
			wantDetailsPage: &DetailsPage{
				Details: &ImportsDetails{
					Imports: []*internal.Import{
						{
							Name: "import1",
							Path: "pa.th/import/1",
						},
					},
					StdLib: []*internal.Import{
						{
							Name: "context",
							Path: "context",
						},
					},
				},
				PackageHeader: samplePackageHeader,
			},
		},
		{
			name: "want expected imports details with multiple",
			imports: []*internal.Import{
				{Name: "import1", Path: "pa.th/import/1"},
				{Name: "import2", Path: "pa.th/import/2"},
				{Name: "import3", Path: "pa.th/import/3"},
			},
			wantDetailsPage: &DetailsPage{
				Details: &ImportsDetails{
					Imports: []*internal.Import{
						{
							Name: "import1",
							Path: "pa.th/import/1",
						},
						{
							Name: "import2",
							Path: "pa.th/import/2",
						},
						{
							Name: "import3",
							Path: "pa.th/import/3",
						},
					},
					StdLib: nil,
				},
				PackageHeader: samplePackageHeader,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := sampleInternalVersion
			version.Packages[0].Imports = tc.imports

			teardownDB, db := postgres.SetupCleanDB(t)
			defer teardownDB(t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			if err := db.InsertVersion(ctx, version, sampleLicenses); err != nil {
				t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", version, sampleLicenses, err)
			}

			got, err := fetchImportsDetails(ctx, db, version.Packages[0].Path, version.Version)
			if err != nil {
				t.Fatalf("fetchModuleDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					version.Packages[0].Path, version.Version, got, err, tc.wantDetailsPage)
			}

			if diff := cmp.Diff(tc.wantDetailsPage, got); diff != "" {
				t.Errorf("fetchModuleDetails(ctx, %q, %q) mismatch (-want +got):\n%s", version.Packages[0].Path, version.Version, diff)
			}
		})
	}
}

func TestFetchImportedByDetails(t *testing.T) {
	var (
		pkg1 = &internal.Package{
			Name: "bar",
			Path: "path.to/foo/bar",
		}
		pkg2 = &internal.Package{
			Name: "bar2",
			Path: "path2.to/foo/bar2",
			Imports: []*internal.Import{
				&internal.Import{
					Name: pkg1.Name,
					Path: pkg1.Path,
				},
			},
		}
		pkg3 = &internal.Package{
			Name: "bar3",
			Path: "path3.to/foo/bar3",
			Imports: []*internal.Import{
				&internal.Import{
					Name: pkg2.Name,
					Path: pkg2.Path,
				},
				&internal.Import{
					Name: pkg1.Name,
					Path: pkg1.Path,
				},
			},
		}
	)

	for _, tc := range []struct {
		path, version string
		wantDetails   *DetailsPage
	}{
		{
			path:    pkg3.Path,
			version: sampleInternalVersion.Version,
			wantDetails: &DetailsPage{
				Details: &ImportedByDetails{},
			},
		},
		{
			path:    pkg2.Path,
			version: sampleInternalVersion.Version,
			wantDetails: &DetailsPage{
				Details: &ImportedByDetails{
					ImportedBy: []string{pkg3.Path},
				},
			},
		},
		{

			path:    pkg1.Path,
			version: sampleInternalVersion.Version,
			wantDetails: &DetailsPage{
				Details: &ImportedByDetails{
					ImportedBy: []string{pkg2.Path, pkg3.Path},
				},
			},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			teardownTestCase, db := postgres.SetupCleanDB(t)
			defer teardownTestCase(t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for _, p := range []*internal.Package{pkg1, pkg2, pkg3} {
				v := sampleInternalVersion
				v.Packages = []*internal.Package{p}
				if err := db.InsertVersion(ctx, v, sampleLicenses); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}
			}

			got, err := fetchImportedByDetails(ctx, db, tc.path, tc.version)
			if err != nil {
				t.Fatalf("fetchImportedByDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					tc.path, tc.version, got, err, tc.wantDetails)
			}

			if diff := cmp.Diff(tc.wantDetails, got, cmpopts.IgnoreFields(DetailsPage{}, "PackageHeader")); diff != "" {
				t.Errorf("fetchImportedByDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}
