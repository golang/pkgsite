// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"net/url"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/fetchdatasource"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/vuln"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func TestDetermineSearchAction(t *testing.T) {
	ctx := context.Background()
	defer postgres.ResetTestDB(testDB, t)
	golangTools := sample.Module("golang.org/x/tools", sample.VersionString, "internal/lsp")
	std := sample.Module("std", sample.VersionString,
		"cmd/go", "cmd/go/internal/auth", "fmt")
	modules := []*internal.Module{golangTools, std}

	for _, v := range modules {
		postgres.MustInsertModule(ctx, t, testDB, v)
	}
	vc, err := vuln.NewInMemoryClient(testEntries)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		method       string
		ds           internal.DataSource
		query        string // query param part of URL
		wantRedirect string
		wantTemplate string
		wantStatus   int // 0 means no error
	}{
		{
			name:       "wrong method",
			method:     "POST",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "bad data source",
			ds:         &fetchdatasource.FetchDataSource{},
			wantStatus: http.StatusFailedDependency,
		},
		{
			name:       "invalid query string",
			query:      "q=\xF4\x90\x80\x80",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "too many filters",
			query:      "q=" + url.QueryEscape("a #b #c"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "query too long",
			query:      "q=" + strings.Repeat("x", maxSearchQueryLength+1),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:         "empty query",
			wantRedirect: "/",
		},
		{
			name:       "offset too large",
			query:      "q=foo&page=100",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "limit too large",
			query:      "q=foo&limit=" + fmt.Sprint(maxSearchPageSize+1),
			wantStatus: http.StatusBadRequest,
		},
		// Some redirections; see more at TestSearchRequestRedirectPath.
		{
			name:         "Go vuln report",
			query:        "q=GO-2020-1234",
			wantRedirect: "/vuln/GO-2020-1234?q", // ??? DO WE WANT THE "?q" ???
		},
		{
			name:         "known unit",
			query:        "q=golang.org/x/tools",
			wantRedirect: "/golang.org/x/tools",
		},
		// Vuln aliases.
		// See testEntries in vulns_test.go to understand results.
		// See TestSearchVulnAlias in this file for more tests.
		{
			name:         "vuln alias",
			query:        "q=GHSA-cccc-ffff-gggg&m=vuln",
			wantRedirect: "/vuln/GO-1990-0001",
		},
		{
			name:         "vuln module path",
			query:        "q=golang.org/x/net&m=vuln",
			wantTemplate: "vuln/list",
		},
		{
			// We turn on vuln mode if the query matches a vuln alias.
			name:         "vuln alias not vuln mode",
			query:        "q=GHSA-cccc-ffff-gggg",
			wantRedirect: "/vuln/GO-1990-0001",
		},
		{
			name:       "vuln alias with no match",
			query:      "q=GHSA-cccc-ffff-xxxx",
			wantStatus: http.StatusNotFound,
		},
		{
			// An explicit mode overrides that.
			name:         "vuln alias symbol mode",
			query:        "q=GHSA-cccc-ffff-gggg?m=symbol",
			wantTemplate: "search",
		},
		{
			name:         "normal search",
			query:        "q=foo",
			wantTemplate: "search",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := buildSearchRequest(t, test.method, test.query)
			var ds internal.DataSource = testDB
			if test.ds != nil {
				ds = test.ds
			}
			gotAction, err := determineSearchAction(req, ds, vc)
			if err != nil {
				var serr *serverError
				if !errors.As(err, &serr) {
					t.Fatalf("got err %#v, want type *serverError", err)
				}
				if g, w := serr.status, test.wantStatus; g != w {
					t.Errorf("got status %d, want %d", g, w)
				}
				return
			}
			if g, w := gotAction.redirectURL, test.wantRedirect; g != w {
				t.Errorf("redirect:\ngot  %q\nwant %q", g, w)
			}
			if g, w := gotAction.template, test.wantTemplate; g != w {
				t.Errorf("template:\ngot  %q\nwant %q", g, w)
			}
		})
	}
}

func buildSearchRequest(t *testing.T, method, query string) *http.Request {
	if method == "" {
		method = "GET"
	}
	u := "/search"
	if query != "" {
		u += "?" + query
	}
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

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
		{
			name:           "search in vuln mode",
			m:              searchModeVuln,
			q:              "foo",
			wantSearchMode: searchModeVuln,
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
				Module: osv.Module{Path: "github.com/mod/foo"},
				Ranges: []osv.Range{{
					Type:   osv.RangeTypeSemver,
					Events: []osv.RangeEvent{{Introduced: "1.0.0"}, {Fixed: "1.9.0"}},
				}},
			}},
		}}
	)

	vc, err := vuln.NewInMemoryClient(vulnEntries)
	if err != nil {
		t.Fatal(err)
	}

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
						Vulns:          []vuln.Vuln{{ID: "test", Details: "vuln"}},
					},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := fetchSearchPage(ctx, testDB, test.query, "", paginationParams{limit: 20, page: 1}, false, vc)
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
				t.Errorf("mismatch (-want, +got):\n%s", diff)
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
		mode  string
	}{
		{"module", "golang.org/x/tools", "/golang.org/x/tools", ""},
		{"directory", "golang.org/x/tools/internal", "/golang.org/x/tools/internal", ""},
		{"package", "golang.org/x/tools/internal/lsp", "/golang.org/x/tools/internal/lsp", ""},
		{"stdlib package does not redirect", "errors", "", ""},
		{"stdlib package does redirect", "cmd/go", "/cmd/go", ""},
		{"stdlib directory does redirect", "cmd/go/internal", "/cmd/go/internal", ""},
		{"std does not redirect", "std", "", ""},
		{"non-existent path does not redirect", "github.com/non-existent", "", ""},
		{"trim URL scheme from query", "https://golang.org/x/tools", "/golang.org/x/tools", ""},
		{"Go vuln redirects", "GO-1969-0720", "/vuln/GO-1969-0720?q", ""},
		{"Lower-case Go vuln redirects", "go-1969-0720", "/vuln/GO-1969-0720?q", ""},
		{"not a Go vuln", "somepkg/GO-1969-0720", "", ""},
		// Just setting the search mode to vuln does not cause a redirect.
		{"search mode is vuln", "searchmodevuln", "", searchModeVuln},
		{"CVE alias", "CVE-2022-32190", "", searchModePackage},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := searchRequestRedirectPath(ctx, testDB, test.query, test.mode, true); got != test.want {
				t.Errorf("searchRequestRedirectPath(ctx, %q) = %q; want = %q", test.query, got, test.want)
			}
		})
	}
}

func TestSearchVulnAlias(t *testing.T) {
	vc, err := vuln.NewInMemoryClient(testEntries)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		mode     string
		query    string
		wantPage *VulnListPage
		wantURL  string
		wantErr  bool
	}{
		{
			name:     "not vuln mode",
			mode:     searchModePackage,
			query:    "doesn't matter",
			wantPage: nil,
			wantURL:  "",
			wantErr:  false,
		},
		{
			name:     "not alias",
			mode:     searchModeVuln,
			query:    "CVE-not-really",
			wantPage: nil,
			wantURL:  "",
			wantErr:  false,
		},
		{
			name:     "no match",
			mode:     searchModeVuln,
			query:    "CVE-1999-1",
			wantPage: nil,
			wantURL:  "",
			wantErr:  true,
		},
		{
			name:    "one match",
			mode:    searchModeVuln,
			query:   "GHSA-cccc-ffff-gggg",
			wantURL: "/vuln/GO-1990-0001",
		},
		{
			name:    "one match - case insensitive",
			mode:    searchModeVuln,
			query:   "gHSa-ccCc-fFFF-gGgG",
			wantURL: "/vuln/GO-1990-0001",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gotAction, err := searchVulnAlias(context.Background(), test.mode, test.query, vc)
			if (err != nil) != test.wantErr {
				t.Fatalf("got %v, want error %t", err, test.wantErr)
			}
			var wantAction *searchAction
			if test.wantURL != "" {
				wantAction = &searchAction{redirectURL: test.wantURL}
			} else if test.wantPage != nil {
				wantAction = &searchAction{
					title:    test.query + " - Vulnerability Reports",
					template: "vuln/list",
					page:     test.wantPage,
				}
			}
			if !cmp.Equal(gotAction, wantAction, cmp.AllowUnexported(searchAction{}), cmpopts.IgnoreUnexported(VulnListPage{})) {
				t.Errorf("\ngot  %+v\nwant %+v", gotAction, wantAction)
			}
		})
	}
}

func TestSearchVulnModulePath(t *testing.T) {
	vc, err := vuln.NewInMemoryClient(testEntries)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		mode     string
		query    string
		wantPage *VulnListPage
		wantURL  string
		wantErr  bool
	}{
		{
			name:     "not vuln mode",
			mode:     searchModePackage,
			query:    "doesn't matter",
			wantPage: nil,
			wantURL:  "",
			wantErr:  false,
		},
		{
			name:     "no match",
			mode:     searchModeVuln,
			query:    "example",
			wantPage: &VulnListPage{Entries: nil},
		},
		{
			name:  "stdlib module match",
			mode:  searchModeVuln,
			query: "stdlib",
			wantPage: &VulnListPage{Entries: []*osv.Entry{
				testEntries[5],
			}},
		},
		{
			name:  "stdlib package match",
			mode:  searchModeVuln,
			query: "net/http",
			wantPage: &VulnListPage{Entries: []*osv.Entry{
				testEntries[5],
			}},
		},
		{
			name:  "module prefix match",
			mode:  searchModeVuln,
			query: "example.com/org",
			wantPage: &VulnListPage{Entries: []*osv.Entry{
				// reverse sorted order
				testEntries[7], testEntries[6],
			}},
		},
		{
			name:  "module path match",
			mode:  searchModeVuln,
			query: "example.com/org/module",
			wantPage: &VulnListPage{Entries: []*osv.Entry{
				testEntries[7],
			}},
		},
		{
			name:  "package path match",
			mode:  searchModeVuln,
			query: "example.com/org/module/a/package",
			wantPage: &VulnListPage{Entries: []*osv.Entry{
				testEntries[7],
			}},
		},
		{
			name:  "package prefix match",
			mode:  searchModeVuln,
			query: "example.com/org/module/a",
			wantPage: &VulnListPage{Entries: []*osv.Entry{
				testEntries[7],
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gotAction, err := searchVulnModule(context.Background(), test.mode, test.query, vc)
			if (err != nil) != test.wantErr {
				t.Fatalf("got %v, want error %t", err, test.wantErr)
			}
			var wantAction *searchAction
			if test.wantURL != "" {
				wantAction = &searchAction{redirectURL: test.wantURL}
			} else if test.wantPage != nil {
				wantAction = &searchAction{
					title:    test.query + " - Vulnerability Reports",
					template: "vuln/list",
					page:     test.wantPage,
				}
			}
			if !cmp.Equal(gotAction, wantAction, cmp.AllowUnexported(searchAction{}), cmpopts.IgnoreUnexported(VulnListPage{})) {
				t.Errorf("\ngot  %+v\nwant %+v", gotAction, wantAction)
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
