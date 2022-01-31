// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/vuln/osv"
)

func TestSearchQueryAndMode(t *testing.T) {
	for _, test := range []struct {
		name, m, q, wantSearchMode string
	}{
		{
			name:           "symbol: prefix in symbol mode",
			m:              searchModeSymbol,
			q:              "#foo",
			wantSearchMode: searchModeSymbol,
		},
		{
			name:           "symbol: prefix in package mode",
			m:              searchModeSymbol,
			q:              "#foo",
			wantSearchMode: searchModeSymbol,
		},
		{
			name:           "search in package mode",
			m:              searchModePackage,
			q:              "foo",
			wantSearchMode: searchModePackage,
		},
		{
			name:           "search in symbol mode",
			m:              searchModeSymbol,
			q:              "foo",
			wantSearchMode: searchModeSymbol,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			u := fmt.Sprintf("/search?q=%s&m=%s", test.q, test.m)
			r := httptest.NewRequest("GET", u, nil)
			gotSearchMode := searchMode(r)
			if gotSearchMode != test.wantSearchMode {
				t.Errorf("searchQueryAndMode(%q) = %q; want = %q",
					u, gotSearchMode, test.wantSearchMode)
			}
		})
	}
}

func TestFetchSearchPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	ctx = experiment.NewContext(ctx, internal.ExperimentVulns)

	var (
		now       = sample.NowTruncated()
		moduleFoo = &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "github.com/mod/foo",
				Version:           "v1.0.0",
				CommitTime:        now,
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						ModuleInfo: internal.ModuleInfo{
							ModulePath:        "github.com/mod/foo",
							Version:           "v1.0.0",
							CommitTime:        now,
							IsRedistributable: true,
						},
						Name:              "foo",
						Path:              "github.com/mod/foo",
						Licenses:          sample.LicenseMetadata(),
						IsRedistributable: true,
					},
					Documentation: []*internal.Documentation{{
						GOOS:     sample.GOOS,
						GOARCH:   sample.GOARCH,
						Synopsis: "foo is a package.",
						Source:   []byte{},
					}},
					Readme: &internal.Readme{
						Filepath: "readme",
						Contents: "readme",
					},
				},
			},
		}
		moduleBar = &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "github.com/mod/bar",
				Version:           "v1.0.0",
				CommitTime:        now,
				IsRedistributable: true,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						ModuleInfo: internal.ModuleInfo{
							CommitTime:        now,
							ModulePath:        "github.com/mod/bar",
							Version:           "v1.0.0",
							IsRedistributable: true,
						},
						Name:              "bar",
						Path:              "github.com/mod/bar",
						Licenses:          sample.LicenseMetadata(),
						IsRedistributable: true,
					},
					Documentation: []*internal.Documentation{{
						GOOS:     sample.GOOS,
						GOARCH:   sample.GOARCH,
						Synopsis: "bar is used by foo.",
						Source:   []byte{},
					}},
					Readme: &internal.Readme{
						Filepath: "readme",
						Contents: "readme",
					},
				},
			},
		}

		vulnEntries = []*osv.Entry{{
			ID:      "test",
			Details: "vuln",
			Affected: []osv.Affected{{
				Package: osv.Package{Name: "github.com/mod/foo"},
				Ranges: []osv.AffectsRange{{
					Type:   osv.TypeSemver,
					Events: []osv.RangeEvent{{Introduced: "1.0.0"}, {Fixed: "1.9.0"}},
				}},
			}},
		}}

		getVulnEntries = func(modulePath string) ([]*osv.Entry, error) {
			if modulePath == moduleFoo.ModulePath {
				return vulnEntries, nil
			}
			return nil, nil
		}
	)

	for _, m := range []*internal.Module{moduleFoo, moduleBar} {
		postgres.MustInsertModule(ctx, t, testDB, m)
	}

	for _, test := range []struct {
		name, query    string
		modules        []*internal.Module
		wantSearchPage *SearchPage
	}{
		{
			name:  "want expected search page",
			query: "foo bar",
			wantSearchPage: &SearchPage{
				PackageTabQuery: "foo bar",
				Pagination: pagination{
					TotalCount:   1,
					ResultCount:  1,
					PrevPage:     0,
					NextPage:     0,
					Limit:        20,
					DefaultLimit: 25,
					MaxLimit:     100,
					Page:         1,
					Pages:        []int{1},
				},
				Results: []*SearchResult{
					{
						Name:           moduleBar.Packages()[0].Name,
						PackagePath:    moduleBar.Packages()[0].Path,
						ModulePath:     moduleBar.ModulePath,
						Version:        "v1.0.0",
						Synopsis:       moduleBar.Packages()[0].Documentation[0].Synopsis,
						DisplayVersion: moduleBar.Version,
						Licenses:       []string{"MIT"},
						CommitTime:     elapsedTime(moduleBar.CommitTime),
					},
				},
			},
		},
		{
			name:  "want only foo search page",
			query: "package",
			wantSearchPage: &SearchPage{
				PackageTabQuery: "package",
				Pagination: pagination{
					TotalCount:   1,
					ResultCount:  1,
					PrevPage:     0,
					NextPage:     0,
					Limit:        20,
					DefaultLimit: 25,
					MaxLimit:     100,
					Page:         1,
					Pages:        []int{1},
				},
				Results: []*SearchResult{
					{
						Name:           moduleFoo.Packages()[0].Name,
						PackagePath:    moduleFoo.Packages()[0].Path,
						ModulePath:     moduleFoo.ModulePath,
						Version:        "v1.0.0",
						Synopsis:       moduleFoo.Packages()[0].Documentation[0].Synopsis,
						DisplayVersion: moduleFoo.Version,
						Licenses:       []string{"MIT"},
						CommitTime:     elapsedTime(moduleFoo.CommitTime),
						Vulns:          []Vuln{{ID: "test", Details: "vuln", FixedVersion: "v1.9.0"}},
					},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := fetchSearchPage(ctx, testDB, test.query, "", paginationParams{limit: 20, page: 1}, false, getVulnEntries)
			if err != nil {
				t.Fatalf("fetchSearchPage(db, %q): %v", test.query, err)
			}

			opts := cmp.Options{
				cmp.AllowUnexported(SearchPage{}, pagination{}),
				cmpopts.IgnoreFields(SearchResult{}, "NumImportedBy"),
				cmpopts.IgnoreFields(licenses.Metadata{}, "FilePath"),
				cmpopts.IgnoreFields(basePage{}, "MetaDescription"),
			}
			if diff := cmp.Diff(test.wantSearchPage, got, opts...); diff != "" {
				t.Errorf("fetchSearchPage(db, %q) mismatch (-want +got):\n%s", test.query, diff)
			}
		})
	}
}

func TestNewSearchResult(t *testing.T) {
	for _, test := range []struct {
		name string
		tag  language.Tag
		in   postgres.SearchResult
		want SearchResult
	}{
		{
			name: "basic",
			tag:  language.English,
			in: postgres.SearchResult{
				Name:          "pkg",
				PackagePath:   "m.com/pkg",
				ModulePath:    "m.com",
				Version:       "v1.0.0",
				NumImportedBy: 3,
			},
			want: SearchResult{
				Name:           "pkg",
				PackagePath:    "m.com/pkg",
				ModulePath:     "m.com",
				Version:        "v1.0.0",
				DisplayVersion: "v1.0.0",
				NumImportedBy:  "3",
			},
		},
		{
			name: "command",
			tag:  language.English,
			in: postgres.SearchResult{
				Name:          "main",
				PackagePath:   "m.com/cmd",
				ModulePath:    "m.com",
				Version:       "v1.0.0",
				NumImportedBy: 1234,
			},
			want: SearchResult{
				Name:           "cmd",
				PackagePath:    "m.com/cmd",
				ModulePath:     "m.com",
				Version:        "v1.0.0",
				DisplayVersion: "v1.0.0",
				ChipText:       "command",
				NumImportedBy:  "1,234",
			},
		},
		{
			name: "stdlib",
			tag:  language.English,
			in: postgres.SearchResult{
				Name:        "math",
				PackagePath: "math",
				ModulePath:  "std",
				Version:     "v1.14.0",
			},
			want: SearchResult{
				Name:           "math",
				PackagePath:    "math",
				ModulePath:     "std",
				Version:        "v1.14.0",
				DisplayVersion: "go1.14",
				ChipText:       "standard library",
				NumImportedBy:  "0",
			},
		},
		{
			name: "German",
			tag:  language.German,
			in: postgres.SearchResult{
				Name:          "pkg",
				PackagePath:   "m.com/pkg",
				ModulePath:    "m.com",
				Version:       "v1.0.0",
				NumImportedBy: 3456,
			},
			want: SearchResult{
				Name:           "pkg",
				PackagePath:    "m.com/pkg",
				ModulePath:     "m.com",
				Version:        "v1.0.0",
				DisplayVersion: "v1.0.0",
				NumImportedBy:  "3.456",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pr := message.NewPrinter(test.tag)
			got := newSearchResult(&test.in, false, pr)
			test.want.CommitTime = "unknown"
			if diff := cmp.Diff(&test.want, got); diff != "" {
				t.Errorf("mimatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestSearchRequestRedirectPath(t *testing.T) {
	// Experiments need to be set in the context, for DB work, and as
	// a middleware, for request handling.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)

	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	golangTools := sample.Module("golang.org/x/tools", sample.VersionString, "internal/lsp")
	std := sample.Module("std", sample.VersionString,
		"cmd/go", "cmd/go/internal/auth", "fmt")
	modules := []*internal.Module{golangTools, std}

	for _, v := range modules {
		postgres.MustInsertModule(ctx, t, testDB, v)
	}
	for _, test := range []struct {
		name  string
		query string
		want  string
	}{
		{"module", "golang.org/x/tools", "/golang.org/x/tools"},
		{"directory", "golang.org/x/tools/internal", "/golang.org/x/tools/internal"},
		{"package", "golang.org/x/tools/internal/lsp", "/golang.org/x/tools/internal/lsp"},
		{"stdlib package does not redirect", "errors", ""},
		{"stdlib package does redirect", "cmd/go", "/cmd/go"},
		{"stdlib directory does redirect", "cmd/go/internal", "/cmd/go/internal"},
		{"std does not redirect", "std", ""},
		{"non-existent path does not redirect", "github.com/non-existent", ""},
		{"trim URL scheme from query", "https://golang.org/x/tools", "/golang.org/x/tools"},
		{"Go vuln redirects", "GO-1969-0720", "/vuln/GO-1969-0720"},
		{"not a Go vuln", "somepkg/GO-1969-0720", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := searchRequestRedirectPath(ctx, testDB, test.query); got != test.want {
				t.Errorf("searchRequestRedirectPath(ctx, %q) = %q; want = %q", test.query, got, test.want)
			}
		})
	}
}

func TestElapsedTime(t *testing.T) {
	now := sample.NowTruncated()
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
		{
			name:        "zero",
			date:        time.Time{},
			elapsedTime: "unknown",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			elapsedTime := elapsedTime(test.date)

			if elapsedTime != test.elapsedTime {
				t.Errorf("elapsedTime(%q) = %s, want %s", test.date, elapsedTime, test.elapsedTime)
			}
		})
	}
}

func TestSymbolSynopsis(t *testing.T) {
	for _, test := range []struct {
		name string
		r    *postgres.SearchResult
		want string
	}{
		{
			"struct field",
			&postgres.SearchResult{
				SymbolName:     "Foo.Bar",
				SymbolSynopsis: "Bar string",
				SymbolKind:     internal.SymbolKindField,
			},
			`
type Foo struct {
	Bar string
}
`,
		},
		{
			"interface method",
			&postgres.SearchResult{
				SymbolName:     "Foo.Bar",
				SymbolSynopsis: "Bar func() string",
				SymbolKind:     internal.SymbolKindMethod,
			},
			`
type Foo interface {
	Bar func() string
}
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := symbolSynopsis(test.r)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch(-want, +got): %s", diff)
			}
		})
	}
}

func TestShouldDefaultToSymbolSearch(t *testing.T) {
	for _, test := range []struct {
		q    string
		want bool
	}{
		{"barista.run", false},
		{"github.com", false},
		{"julie.io", false},
		{"my.name", false},
		{"sql", false},
		{"sql.DB", true},
		{"sql.DB.Begin", true},
		{"yaml.v2", false},
		{"gopkg.in", false},
		{"Unmarshal", true},
	} {
		t.Run(test.q, func(t *testing.T) {
			got := shouldDefaultToSymbolSearch(test.q)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
