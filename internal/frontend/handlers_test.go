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

func TestFetchOverviewPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name             string
		version          internal.Version
		wantOverviewPage OverviewPage
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
			License:    "MIT",
			ReadMe:     []byte("This is the readme text."),
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "pkg_name",
					Path:     "test.com/module/pkg_name",
					Synopsis: "Test package synopsis",
				},
			},
			VersionType: internal.VersionTypeRelease,
		},
		wantOverviewPage: OverviewPage{
			ModulePath: "test.com/module",
			ReadMe:     template.HTML("<p>This is the readme text.</p>\n"),
			PackageHeader: &Package{
				Name:       "pkg_name",
				Version:    "v1.0.0",
				Path:       "test.com/module/pkg_name",
				Synopsis:   "Test package synopsis",
				License:    "MIT",
				CommitTime: "today",
			},
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(ctx, &tc.version); err != nil {
		t.Fatalf("db.InsertVersion(%v): %v", tc.version, err)
	}

	got, err := fetchOverviewPage(ctx, db, tc.version.Packages[0].Path, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchOverviewPage(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantOverviewPage)
	}

	if diff := cmp.Diff(tc.wantOverviewPage, *got); diff != "" {
		t.Errorf("fetchOverviewPage(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
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
		License:    "MIT",
		ReadMe:     []byte("This is the readme text."),
		Packages: []*internal.Package{
			&internal.Package{
				Name:     "pkg_name",
				Path:     pkgPath,
				Synopsis: "test synopsis",
			},
		},
		VersionType: versionType,
	}
}

func TestFetchVersionsPage(t *testing.T) {
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
		wantVersionsPage    *VersionsPage
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
			wantVersionsPage: &VersionsPage{
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
				PackageHeader: &Package{
					Name:       "pkg_name",
					Version:    "v1.2.1",
					Path:       pkg1Path,
					Synopsis:   "test synopsis",
					License:    "MIT",
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
			wantVersionsPage: &VersionsPage{
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
				PackageHeader: &Package{
					Name:       "pkg_name",
					Version:    "v0.0.0-20140414041501-3c2ca4d52544",
					Path:       pkg1Path,
					Synopsis:   "test synopsis",
					License:    "MIT",
					CommitTime: "today",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownDB, db := postgres.SetupCleanDB(t)
			defer teardownDB(t)

			for _, v := range tc.versions {
				if err := db.InsertVersion(ctx, v); err != nil {
					t.Fatalf("db.InsertVersion(%v): %v", v, err)
				}
			}

			got, err := fetchVersionsPage(ctx, db, tc.path, tc.version)
			if err != nil {
				t.Fatalf("fetchVersionsPage(ctx, db, %q, %q) = %v err = %v, want %v",
					tc.path, tc.version, got, err, tc.wantVersionsPage)
			}

			if diff := cmp.Diff(tc.wantVersionsPage, got); diff != "" {
				t.Errorf("fetchVersionsPage(ctx, db, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
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

func TestFetchSearchPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		now     = postgres.NowTruncated()
		series1 = &internal.Series{
			Path: "myseries",
		}
		module1 = &internal.Module{
			Path:   "github.com/valid_module_name",
			Series: series1,
		}
		versionFoo = &internal.Version{
			Version:     "v1.0.0",
			License:     "licensename",
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
			Module:      module1,
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Path:     "/path/to/foo",
					Synopsis: "foo is a package.",
				},
			},
		}
		versionBar = &internal.Version{
			Version:     "v1.0.0",
			License:     "licensename",
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
			Module:      module1,
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "bar",
					Path:     "/path/to/bar",
					Synopsis: "bar is used by foo.",
				},
			},
		}
	)

	for _, tc := range []struct {
		name, query    string
		versions       []*internal.Version
		wantSearchPage *SearchPage
	}{
		{
			name:     "want_expected_search_page",
			query:    "foo bar",
			versions: []*internal.Version{versionFoo, versionBar},
			wantSearchPage: &SearchPage{
				Query: "foo bar",
				Results: []*SearchResult{
					&SearchResult{
						Name:         versionBar.Packages[0].Name,
						PackagePath:  versionBar.Packages[0].Path,
						ModulePath:   versionBar.Module.Path,
						Synopsis:     versionBar.Packages[0].Synopsis,
						Version:      versionBar.Version,
						License:      versionBar.License,
						CommitTime:   elapsedTime(versionBar.CommitTime),
						NumImporters: 0,
					},
					&SearchResult{
						Name:         versionFoo.Packages[0].Name,
						PackagePath:  versionFoo.Packages[0].Path,
						ModulePath:   versionFoo.Module.Path,
						Synopsis:     versionFoo.Packages[0].Synopsis,
						Version:      versionFoo.Version,
						License:      versionFoo.License,
						CommitTime:   elapsedTime(versionFoo.CommitTime),
						NumImporters: 0,
					},
				},
			},
		},
		{
			name:     "want_only_foo_search_page",
			query:    "package",
			versions: []*internal.Version{versionFoo, versionBar},
			wantSearchPage: &SearchPage{
				Query: "package",
				Results: []*SearchResult{
					&SearchResult{
						Name:         versionFoo.Packages[0].Name,
						PackagePath:  versionFoo.Packages[0].Path,
						ModulePath:   versionFoo.Module.Path,
						Synopsis:     versionFoo.Packages[0].Synopsis,
						Version:      versionFoo.Version,
						License:      versionFoo.License,
						CommitTime:   elapsedTime(versionFoo.CommitTime),
						NumImporters: 0,
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := postgres.SetupCleanDB(t)
			defer teardownTestCase(t)

			for _, v := range tc.versions {
				if err := db.InsertVersion(ctx, v); err != nil {
					t.Fatalf("db.InsertVersion(%+v): %v", v, err)
				}
				if err := db.InsertDocuments(ctx, v); err != nil {
					t.Fatalf("db.InsertDocuments(%+v): %v", v, err)
				}
			}

			got, err := fetchSearchPage(ctx, db, tc.query)
			if err != nil {
				t.Fatalf("fetchSearchPage(db, %q): %v", tc.query, err)
			}

			if diff := cmp.Diff(tc.wantSearchPage, got); diff != "" {
				t.Errorf("fetchSearchPage(db, %q) mismatch (-want +got):\n%s", tc.query, diff)
			}
		})
	}
}

func TestFetchModulePage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tc := struct {
		name           string
		version        internal.Version
		wantModulePage ModulePage
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
			License:    "MIT",
			ReadMe:     []byte("This is the readme text."),
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "pkg_name",
					Path:     "test.com/module/pkg_name",
					Synopsis: "Test package synopsis",
				},
			},
			VersionType: internal.VersionTypeRelease,
		},
		wantModulePage: ModulePage{
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
				},
			},
			PackageHeader: &Package{
				Name:       "pkg_name",
				Version:    "v1.0.0",
				Path:       "test.com/module/pkg_name",
				Synopsis:   "Test package synopsis",
				License:    "MIT",
				CommitTime: "today",
			},
		},
	}

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	if err := db.InsertVersion(ctx, &tc.version); err != nil {
		t.Fatalf("db.InsertVersion(%v): %v", tc.version, err)
	}

	got, err := fetchModulePage(ctx, db, tc.version.Packages[0].Path, tc.version.Version)
	if err != nil {
		t.Fatalf("fetchModulePage(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantModulePage)
	}

	if diff := cmp.Diff(tc.wantModulePage, *got); diff != "" {
		t.Errorf("fetchModulePage(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}
