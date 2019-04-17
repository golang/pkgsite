// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"html/template"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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
		version         internal.Version
		wantDetailsPage *DetailsPage
	}{
		name: "want_expected_module_page",
		version: internal.Version{
			Module: &internal.Module{
				Path: "test.com/module",
				Series: &internal.Series{
					Path: "series",
				},
			},
			Version:    "v1.0.0",
			Synopsis:   "test synopsis",
			CommitTime: time.Now().Add(time.Hour * -8),
			ReadMe:     []byte("This is the readme text."),
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "pkg_name",
					Path:     "test.com/module/pkg_name",
					Synopsis: "Test package synopsis",
					Licenses: sampleLicenseInfos,
				},
			},
			VersionType: internal.VersionTypeRelease,
		},
		wantDetailsPage: &DetailsPage{
			Details: &OverviewDetails{
				ModulePath: "test.com/module",
				ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
			},
			PackageHeader: &Package{
				dbname:     "pkg_name",
				Version:    "v1.0.0",
				Path:       "test.com/module/pkg_name",
				Synopsis:   "Test package synopsis",
				Licenses:   sampleLicenseInfos,
				CommitTime: "today",
			},
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(ctx, &tc.version, sampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(%v): %v", tc.version, err)
	}

	got, err := fetchOverviewDetails(ctx, db, tc.version.Packages[0].Path, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchOverviewDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetailsPage)
	}

	if diff := cmp.Diff(tc.wantDetailsPage, got, cmp.AllowUnexported(Package{})); diff != "" {
		t.Errorf("fetchOverviewDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func getTestVersion(pkgPath, modulePath, version string, versionType internal.VersionType) *internal.Version {
	return &internal.Version{
		Module: &internal.Module{
			Path: modulePath,
			Series: &internal.Series{
				Path: "test.com/module",
			},
		},
		Version:    version,
		CommitTime: time.Now().Add(time.Hour * -8),
		ReadMe:     []byte("This is the readme text."),
		Packages: []*internal.Package{
			&internal.Package{
				Name:     "pkg_name",
				Path:     pkgPath,
				Synopsis: "test synopsis",
				Licenses: sampleLicenseInfos,
			},
		},
		VersionType: versionType,
	}
}

func TestFetchVersionsDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		module1  = "test.com/module"
		module2  = "test.com/module/v2"
		pkg1Path = "test.com/module/pkg_name"
		pkg2Path = "test.com/module/v2/pkg_name"
	)

	for _, tc := range []struct {
		name, path, version string
		versions            []*internal.Version
		wantDetailsPage     *DetailsPage
	}{
		{
			name:    "want_tagged_versions",
			path:    pkg1Path,
			version: "v1.2.1",
			versions: []*internal.Version{
				getTestVersion(pkg1Path, module1, "v1.2.3", internal.VersionTypeRelease),
				getTestVersion(pkg2Path, module2, "v2.0.0", internal.VersionTypeRelease),
				getTestVersion(pkg1Path, module1, "v1.3.0", internal.VersionTypeRelease),
				getTestVersion(pkg1Path, module1, "v1.2.1", internal.VersionTypeRelease),
				getTestVersion(pkg1Path, module1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo),
				getTestVersion(pkg2Path, module2, "v2.2.1-alpha.1", internal.VersionTypePrerelease),
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
					dbname:     "pkg_name",
					Version:    "v1.2.1",
					Path:       pkg1Path,
					Synopsis:   "test synopsis",
					Licenses:   []*internal.LicenseInfo{{Type: "MIT", FilePath: "LICENSE"}},
					CommitTime: "today",
				},
			},
		},
		{
			name:    "want_only_pseudo",
			path:    pkg1Path,
			version: "v0.0.0-20140414041501-3c2ca4d52544",
			versions: []*internal.Version{
				getTestVersion(pkg1Path, module1, "v0.0.0-20140414041501-3c2ca4d52544", internal.VersionTypePseudo),
				getTestVersion(pkg1Path, module1, "v0.0.0-20140414041502-3c2ca4d52544", internal.VersionTypePseudo),
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
					dbname:     "pkg_name",
					Version:    "v0.0.0-20140414041501-3c2ca4d52544",
					Path:       pkg1Path,
					Synopsis:   "test synopsis",
					Licenses:   []*internal.LicenseInfo{{Type: "MIT", FilePath: "LICENSE"}},
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

			if diff := cmp.Diff(tc.wantDetailsPage, got, cmp.AllowUnexported(Package{})); diff != "" {
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
		version         internal.Version
		wantDetailsPage *DetailsPage
	}{
		name: "want_expected_module_page",
		version: internal.Version{
			Module: &internal.Module{
				Path: "test.com/module",
				Series: &internal.Series{
					Path: "series",
				},
			},
			Version:    "v1.0.0",
			Synopsis:   "test synopsis",
			CommitTime: time.Now().Add(time.Hour * -8),
			ReadMe:     []byte("This is the readme text."),
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "pkg_name",
					Path:     "test.com/module/pkg_name",
					Synopsis: "Test package synopsis",
					Licenses: sampleLicenseInfos,
				},
			},
			VersionType: internal.VersionTypeRelease,
		},
		wantDetailsPage: &DetailsPage{
			Details: &ModuleDetails{
				ModulePath: "test.com/module",
				ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
				Version:    "v1.0.0",
				Packages: []*Package{
					&Package{
						Name:       "pkg_name",
						ModulePath: "test.com/module",
						Path:       "test.com/module/pkg_name",
						Version:    "v1.0.0",
						Synopsis:   "Test package synopsis",
						Licenses:   sampleLicenseInfos,
					},
				},
			},
			PackageHeader: &Package{
				dbname:     "pkg_name",
				Version:    "v1.0.0",
				Path:       "test.com/module/pkg_name",
				Synopsis:   "Test package synopsis",
				Licenses:   sampleLicenseInfos,
				CommitTime: "today",
			},
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(ctx, &tc.version, sampleLicenses); err != nil {
		t.Fatalf("db.InsertVersion(%v): %v", tc.version, err)
	}

	got, err := fetchModuleDetails(ctx, db, tc.version.Packages[0].Path, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchModuleDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetailsPage)
	}

	if diff := cmp.Diff(tc.wantDetailsPage, *got, cmp.AllowUnexported(Package{})); diff != "" {
		t.Errorf("fetchModuleDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestPackageMethods(t *testing.T) {
	for _, tc := range []struct {
		path, dbname, wantName, wantTitle string
		wantIsCommand                     bool
	}{
		{
			dbname:        "foo",
			path:          "pa.th/to/foo",
			wantName:      "foo",
			wantTitle:     "Package foo",
			wantIsCommand: false,
		},
		{
			dbname:        "main",
			path:          "pa.th/to/foo",
			wantName:      "foo",
			wantTitle:     "Command foo",
			wantIsCommand: true,
		},
		{
			dbname:        "main",
			path:          "pa.th/to/foo/v2",
			wantName:      "foo",
			wantTitle:     "Command foo",
			wantIsCommand: true,
		},
		{
			dbname:        "main",
			path:          "pa.th/to/foo/v1",
			wantName:      "foo",
			wantTitle:     "Command foo",
			wantIsCommand: true,
		},
	} {

		t.Run(tc.path, func(t *testing.T) {
			p := &Package{
				dbname: tc.dbname,
				Path:   tc.path,
			}
			if p.Name() != tc.wantName {
				t.Errorf("p.Name() = %q; want = %q", p.Name(), tc.wantName)
			}
			if p.IsCommand() != tc.wantIsCommand {
				t.Errorf("p.IsCommand() = %t; want = %t", p.IsCommand(), tc.wantIsCommand)
			}
			if p.Title() != tc.wantTitle {
				t.Errorf("p.Header() = %q; want = %q", p.Title(), tc.wantTitle)
			}
		})
	}
}
