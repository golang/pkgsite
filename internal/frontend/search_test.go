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
	"golang.org/x/discovery/internal/sample"
)

func TestFetchSearchPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		now        = sample.NowTruncated()
		versionFoo = &internal.Version{
			VersionInfo: internal.VersionInfo{
				ModulePath:     "github.com/mod/foo",
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
					Licenses: sample.LicenseMetadata,
				},
			},
		}
		versionBar = &internal.Version{
			VersionInfo: internal.VersionInfo{
				ModulePath:     "github.com/mod/bar",
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
					Licenses: sample.LicenseMetadata,
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
			name:     "want expected search page",
			query:    "foo bar",
			versions: []*internal.Version{versionFoo, versionBar},
			wantSearchPage: &SearchPage{
				Pagination: pagination{
					TotalCount:  1,
					ResultCount: 1,
					PageCount:   1,
					PrevPage:    0,
					NextPage:    0,
					PerPage:     20,
					Page:        1,
					Pages:       []int{1},
				},
				Results: []*SearchResult{
					&SearchResult{
						Name:          versionBar.Packages[0].Name,
						PackagePath:   versionBar.Packages[0].Path,
						ModulePath:    versionBar.ModulePath,
						Synopsis:      versionBar.Packages[0].Synopsis,
						Version:       versionBar.Version,
						Licenses:      []string{"MIT"},
						CommitTime:    elapsedTime(versionBar.CommitTime),
						NumImportedBy: 0,
					},
				},
			},
		},
		{
			name:     "want only foo search page",
			query:    "package",
			versions: []*internal.Version{versionFoo, versionBar},
			wantSearchPage: &SearchPage{
				Pagination: pagination{
					TotalCount:  1,
					ResultCount: 1,
					PageCount:   1,
					PrevPage:    0,
					NextPage:    0,
					PerPage:     20,
					Page:        1,
					Pages:       []int{1},
				},
				Results: []*SearchResult{
					&SearchResult{
						Name:          versionFoo.Packages[0].Name,
						PackagePath:   versionFoo.Packages[0].Path,
						ModulePath:    versionFoo.ModulePath,
						Synopsis:      versionFoo.Packages[0].Synopsis,
						Version:       versionFoo.Version,
						Licenses:      []string{"MIT"},
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
				if err := testDB.InsertVersion(ctx, v, sample.Licenses); err != nil {
					t.Fatalf("db.InsertVersion(%+v): %v", v, err)
				}
				if err := testDB.InsertDocuments(ctx, v); err != nil {
					t.Fatalf("testDB.InsertDocument(%+v): %v", v, err)
				}
				if err := testDB.RefreshSearchDocuments(ctx); err != nil {
					t.Fatalf("testDB.RefreshSearchDocuments(ctx): %v", err)
				}
			}

			got, err := fetchSearchPage(ctx, testDB, tc.query, paginationParams{limit: 20, page: 1})
			if err != nil {
				t.Fatalf("fetchSearchPage(db, %q): %v", tc.query, err)
			}

			if diff := cmp.Diff(tc.wantSearchPage, got, cmp.AllowUnexported(SearchPage{}), cmpopts.IgnoreFields(license.Metadata{}, "FilePath")); diff != "" {
				t.Errorf("fetchSearchPage(db, %q) mismatch (-want +got):\n%s", tc.query, diff)
			}
		})
	}
}
