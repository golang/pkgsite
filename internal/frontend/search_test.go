// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/license"
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
			VersionInfo: internal.VersionInfo{
				SeriesPath:     seriesPath,
				ModulePath:     modulePath,
				Version:        "v1.0.0",
				ReadmeContents: []byte("readme"),
				CommitTime:     now,
				VersionType:    internal.VersionTypeRelease,
			},
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Path:     "/path/to/foo",
					Synopsis: "foo is a package.",
					Licenses: sampleLicenseMetadata,
				},
			},
		}
		versionBar = &internal.Version{
			VersionInfo: internal.VersionInfo{
				SeriesPath:     seriesPath,
				ModulePath:     modulePath,
				Version:        "v1.0.0",
				ReadmeContents: []byte("readme"),
				CommitTime:     now,
				VersionType:    internal.VersionTypeRelease,
			},
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "bar",
					Path:     "/path/to/bar",
					Synopsis: "bar is used by foo.",
					Licenses: sampleLicenseMetadata,
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
				basePageData: basePageData{
					Query: "foo bar",
					Title: "foo bar",
				},
				NumResults: 2,
				NumPages:   1,
				Prev:       0,
				Next:       0,
				Page:       1,
				Pages:      []int{1},
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
				basePageData: basePageData{
					Query: "package",
					Title: "package",
				},
				NumResults: 1,
				NumPages:   1,
				Prev:       0,
				Next:       0,
				Page:       1,
				Pages:      []int{1},
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
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.versions {
				if err := testDB.InsertVersion(ctx, v, sampleLicenses); err != nil {
					t.Fatalf("db.InsertVersion(%+v): %v", v, err)
				}
				if err := testDB.RefreshSearchDocuments(ctx); err != nil {
					t.Fatalf("testDB.RefreshSearchDocuments(ctx): %v", err)
				}
			}

			got, err := fetchSearchPage(ctx, testDB, tc.query, 20, 1)
			if err != nil {
				t.Fatalf("fetchSearchPage(db, %q): %v", tc.query, err)
			}

			if diff := cmp.Diff(tc.wantSearchPage, got, cmp.AllowUnexported(SearchPage{}), cmpopts.IgnoreFields(license.Metadata{}, "FilePath")); diff != "" {
				t.Errorf("fetchSearchPage(db, %q) mismatch (-want +got):\n%s", tc.query, diff)
			}
		})
	}
}

func TestSearchPageMethods(t *testing.T) {
	for _, tc := range []struct {
		page, numResults, wantNumPages, wantOffset, wantPrev, wantNext int
		name                                                           string
	}{
		{
			name:         "single page of results with numResults below limit",
			page:         1,
			numResults:   7,
			wantNumPages: 1,
			wantOffset:   0,
			wantPrev:     0,
			wantNext:     0,
		},
		{
			name:         "single page of results with numResults exactly limit",
			page:         1,
			numResults:   10,
			wantNumPages: 1,
			wantOffset:   0,
			wantPrev:     0,
			wantNext:     0,
		},
		{
			name:         "first page of results for total of 5 pages",
			page:         1,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   0,
			wantPrev:     0,
			wantNext:     2,
		},
		{
			name:         "second page of results for total of 5 pages",
			page:         2,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   10,
			wantPrev:     1,
			wantNext:     3,
		},
		{
			name:         "last page of results for total of 5 pages",
			page:         5,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   40,
			wantPrev:     4,
			wantNext:     0,
		},
		{
			name:         "page out of range",
			page:         8,
			numResults:   47,
			wantNumPages: 5,
			wantOffset:   70,
			wantPrev:     7,
			wantNext:     0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testLimit := 10
			if got := numPages(testLimit, tc.numResults); got != tc.wantNumPages {
				t.Errorf("numPages(%d, %d) = %d; want = %d",
					testLimit, tc.numResults, got, tc.wantNumPages)
			}
			if got := offset(tc.page, testLimit); got != tc.wantOffset {
				t.Errorf("offset(%d, %d) = %d; want = %d",
					tc.page, testLimit, got, tc.wantOffset)
			}
			if got := prev(tc.page); got != tc.wantPrev {
				t.Errorf("prev(%d) = %d; want = %d", tc.page, got, tc.wantPrev)
			}
			if got := next(tc.page, testLimit, tc.numResults); got != tc.wantNext {
				t.Errorf("next(%d, %d, %d) = %d; want = %d",
					tc.page, testLimit, tc.numResults, got, tc.wantNext)
			}
		})
	}
}

func TestPagesToDisplay(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		page, numPages, numToDisplay int
		wantPages                    []int
	}{
		{
			name:         "page 1 of 10- first in range",
			page:         1,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{1, 2, 3, 4, 5},
		},
		{
			name:         "page 3 of 10 - last in range to include 1 in wantPages ",
			page:         3,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{1, 2, 3, 4, 5},
		},
		{
			name:         "page 4 of 10 - first in range to include 1 in wantPages",
			page:         4,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{2, 3, 4, 5, 6},
		},
		{
			name:         "page 7 of 10 - page in the middle",
			page:         7,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{5, 6, 7, 8, 9},
		},
		{
			name:         "page 8 of 10- first in range to include page 10",
			page:         8,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{6, 7, 8, 9, 10},
		},
		{
			name:         "page 10 of 10 - last page in range",
			page:         10,
			numPages:     10,
			numToDisplay: 5,
			wantPages:    []int{6, 7, 8, 9, 10},
		},
		{
			name:         "page 1 of 11, displaying 4 pages - first in range",
			page:         1,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{1, 2, 3, 4},
		},
		{
			name:         "page 3 of 11, display 4 pages - last in range to include 1 in wantPages ",
			page:         3,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{1, 2, 3, 4},
		},
		{
			name:         "page 4 of 11, displaying 4 pages - first in range to include 1 in wantPages",
			page:         4,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{2, 3, 4, 5},
		},
		{
			name:         "page 7 of 11, displaying 4 pages - page in the middle",
			page:         7,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{5, 6, 7, 8},
		},
		{
			name:         "page 8 of 11, displaying 4 pages",
			page:         8,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{6, 7, 8, 9},
		},
		{
			name:         "page 10 of 11, displaying 4 pages - second to last page in range",
			page:         10,
			numPages:     11,
			numToDisplay: 4,
			wantPages:    []int{8, 9, 10, 11},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pagesToLink(tc.page, tc.numPages, tc.numToDisplay)
			if diff := cmp.Diff(got, tc.wantPages); diff != "" {
				t.Errorf("pagesToLink(%d, %d, %d) = %v; want = %v", tc.page, tc.numPages, tc.numToDisplay, got, tc.wantPages)
			}
		})
	}
}
