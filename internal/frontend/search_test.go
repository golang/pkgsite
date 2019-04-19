// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

func TestFetchSearchPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		now        = postgres.NowTruncated()
		seriesPath = "myseries"
		modulePath = "github.com/valid_module_name"
		versionFoo = &internal.Version{
			SeriesPath:  seriesPath,
			ModulePath:  modulePath,
			Version:     "v1.0.0",
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Path:     "/path/to/foo",
					Synopsis: "foo is a package.",
					Licenses: sampleLicenseInfos,
				},
			},
		}
		versionBar = &internal.Version{
			SeriesPath:  seriesPath,
			ModulePath:  modulePath,
			Version:     "v1.0.0",
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
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
						Name:          versionBar.Packages[0].Name,
						PackagePath:   versionBar.Packages[0].Path,
						ModulePath:    versionBar.ModulePath,
						Synopsis:      versionBar.Packages[0].Synopsis,
						Version:       versionBar.Version,
						Licenses:      versionBar.Packages[0].Licenses,
						CommitTime:    elapsedTime(versionBar.CommitTime),
						NumImportedBy: 0,
					},
					&SearchResult{
						Name:          versionFoo.Packages[0].Name,
						PackagePath:   versionFoo.Packages[0].Path,
						ModulePath:    versionFoo.ModulePath,
						Synopsis:      versionFoo.Packages[0].Synopsis,
						Version:       versionFoo.Version,
						Licenses:      versionFoo.Packages[0].Licenses,
						CommitTime:    elapsedTime(versionFoo.CommitTime),
						NumImportedBy: 0,
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
						Name:          versionFoo.Packages[0].Name,
						PackagePath:   versionFoo.Packages[0].Path,
						ModulePath:    versionFoo.ModulePath,
						Synopsis:      versionFoo.Packages[0].Synopsis,
						Version:       versionFoo.Version,
						Licenses:      versionFoo.Packages[0].Licenses,
						CommitTime:    elapsedTime(versionFoo.CommitTime),
						NumImportedBy: 0,
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := postgres.SetupCleanDB(t)
			defer teardownTestCase(t)

			for _, v := range tc.versions {
				if err := db.InsertVersion(ctx, v, sampleLicenses); err != nil {
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
