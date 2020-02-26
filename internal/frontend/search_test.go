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
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/testing/sample"
	"golang.org/x/discovery/internal/version"
)

func TestFetchSearchPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		now       = sample.NowTruncated()
		moduleFoo = &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "github.com/mod/foo",
				Version:           "v1.0.0",
				ReadmeContents:    "readme",
				CommitTime:        now,
				VersionType:       version.TypeRelease,
				IsRedistributable: true,
			},
			Packages: []*internal.Package{
				{
					Name:              "foo",
					Path:              "/path/to/foo",
					Synopsis:          "foo is a package.",
					Licenses:          sample.LicenseMetadata,
					IsRedistributable: true,
				},
			},
		}
		moduleBar = &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "github.com/mod/bar",
				Version:           "v1.0.0",
				ReadmeContents:    "readme",
				CommitTime:        now,
				VersionType:       version.TypeRelease,
				IsRedistributable: true,
			},
			Packages: []*internal.Package{
				{
					Name:              "bar",
					Path:              "/path/to/bar",
					Synopsis:          "bar is used by foo.",
					Licenses:          sample.LicenseMetadata,
					IsRedistributable: true,
				},
			},
		}
	)

	for _, tc := range []struct {
		name, query    string
		modules        []*internal.Module
		wantSearchPage *SearchPage
	}{
		{
			name:    "want expected search page",
			query:   "foo bar",
			modules: []*internal.Module{moduleFoo, moduleBar},
			wantSearchPage: &SearchPage{
				Pagination: pagination{
					TotalCount:  1,
					ResultCount: 1,
					PrevPage:    0,
					NextPage:    0,
					limit:       20,
					Page:        1,
					Pages:       []int{1},
				},
				Results: []*SearchResult{
					{
						Name:           moduleBar.Packages[0].Name,
						PackagePath:    moduleBar.Packages[0].Path,
						ModulePath:     moduleBar.ModulePath,
						Synopsis:       moduleBar.Packages[0].Synopsis,
						DisplayVersion: moduleBar.Version,
						Licenses:       []string{"MIT"},
						CommitTime:     elapsedTime(moduleBar.CommitTime),
						NumImportedBy:  0,
					},
				},
			},
		},
		{
			name:    "want only foo search page",
			query:   "package",
			modules: []*internal.Module{moduleFoo, moduleBar},
			wantSearchPage: &SearchPage{
				Pagination: pagination{
					TotalCount:  1,
					ResultCount: 1,
					PrevPage:    0,
					NextPage:    0,
					limit:       20,
					Page:        1,
					Pages:       []int{1},
				},
				Results: []*SearchResult{
					{
						Name:           moduleFoo.Packages[0].Name,
						PackagePath:    moduleFoo.Packages[0].Path,
						ModulePath:     moduleFoo.ModulePath,
						Synopsis:       moduleFoo.Packages[0].Synopsis,
						DisplayVersion: moduleFoo.Version,
						Licenses:       []string{"MIT"},
						CommitTime:     elapsedTime(moduleFoo.CommitTime),
						NumImportedBy:  0,
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.modules {
				if err := testDB.InsertModule(ctx, v); err != nil {
					t.Fatal(err)
				}
			}

			got, err := fetchSearchPage(ctx, testDB, tc.query, paginationParams{limit: 20, page: 1})
			if err != nil {
				t.Fatalf("fetchSearchPage(db, %q): %v", tc.query, err)
			}

			opts := cmp.Options{
				cmp.AllowUnexported(SearchPage{}, pagination{}),
				cmpopts.IgnoreFields(licenses.Metadata{}, "FilePath"),
				cmpopts.IgnoreFields(pagination{}, "Approximate"),
			}
			if diff := cmp.Diff(tc.wantSearchPage, got, opts...); diff != "" {
				t.Errorf("fetchSearchPage(db, %q) mismatch (-want +got):\n%s", tc.query, diff)
			}
		})
	}
}

func TestApproximateNumber(t *testing.T) {
	tests := []struct {
		estimate int
		sigma    float64
		want     int
	}{
		{55872, 0.1, 60000},
		{55872, 1.0, 100000},
		{45872, 1.0, 0},
		{85872, 0.1, 90000},
		{85872, 0.4, 100000},
		{15711, 0.1, 16000},
		{136368, 0.05, 140000},
		{136368, 0.005, 136000},
		{3, 0.1, 3},
	}
	for _, test := range tests {
		if got := approximateNumber(test.estimate, test.sigma); got != test.want {
			t.Errorf("approximateNumber(%d, %f) = %d, want %d", test.estimate, test.sigma, got, test.want)
		}
	}
}

func TestSearchRequestRedirectPath(t *testing.T) {
	ctx := context.Background()

	golangTools := sample.Module()
	golangTools.ModulePath = "golang.org/x/tools"
	lspPkg := sample.Package()
	lspPkg.Path = "golang.org/x/tools/internal/lsp"
	golangTools.Packages = []*internal.Package{lspPkg}

	std := sample.Module()
	std.ModulePath = "std"
	var stdlibPackages []*internal.Package
	for _, path := range []string{"cmd/go", "cmd/go/internal/auth", "fmt"} {
		pkg := sample.Package()
		pkg.Path = path
		stdlibPackages = append(stdlibPackages, pkg)
	}
	std.Packages = stdlibPackages
	modules := []*internal.Module{golangTools, std}

	for _, v := range modules {
		if err := testDB.InsertModule(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{"module", "golang.org/x/tools", "/mod/golang.org/x/tools"},
		{"directory", "golang.org/x/tools/internal", "/golang.org/x/tools/internal"},
		{"package", "golang.org/x/tools/internal/lsp", "/golang.org/x/tools/internal/lsp"},
		{"stdlib package does not redirect", "errors", ""},
		{"stdlib package does redirect", "cmd/go", "/cmd/go"},
		{"stdlib directory does redirect", "cmd/go/internal", "/cmd/go/internal"},
		{"std does not redirect", "std", ""},
		{"non-existent path does not redirect", "github.com/non-existent", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := searchRequestRedirectPath(ctx, testDB, tc.query); got != tc.want {
				t.Errorf("searchRequestRedirectPath(ctx, %q) = %q; want = %q", tc.query, got, tc.want)
			}
		})
	}
}
