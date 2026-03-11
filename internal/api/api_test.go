// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/vuln"
)

func TestServePackage(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	const (
		pkgPath     = "example.com/a/b"
		modulePath1 = "example.com/a"
		modulePath2 = "example.com/a/b"
		version     = "v1.2.3"
	)

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com",
			Version:       version,
			LatestVersion: "v1.2.4",
		},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path: "example.com/pkg",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:    "example.com",
					Version:       version,
					LatestVersion: "v1.2.4",
				},
				Name: "pkg",
			},
			Documentation: []*internal.Documentation{sample.Documentation("linux", "amd64", sample.DocContents)},
		}},
	})

	for _, mp := range []string{modulePath1, modulePath2} {
		u := &internal.Unit{
			UnitMeta: internal.UnitMeta{
				Path: pkgPath,
				ModuleInfo: internal.ModuleInfo{
					ModulePath:    mp,
					Version:       version,
					LatestVersion: version,
				},
				Name: "b",
			},
			Documentation: []*internal.Documentation{
				{
					GOOS:     "linux",
					GOARCH:   "amd64",
					Synopsis: "Synopsis for " + mp,
				},
			},
		}
		ds.MustInsertModule(ctx, &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:    mp,
				Version:       version,
				LatestVersion: version,
			},
			Units: []*internal.Unit{u},
		})
	}

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath:    "example.com",
			Version:       "v1.2.4",
			LatestVersion: "v1.2.4",
		},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path: "example.com/pkg",
				ModuleInfo: internal.ModuleInfo{
					ModulePath:    "example.com",
					Version:       "v1.2.4",
					LatestVersion: "v1.2.4",
				},
				Name: "pkg",
			},
			Documentation: []*internal.Documentation{{
				GOOS:     "linux",
				GOARCH:   "amd64",
				Synopsis: "Basic synopsis",
			}},
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		want       any // Can be *Package or *Error
	}{
		{
			name:       "basic metadata",
			url:        "/v1/package/example.com/pkg?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "ambiguous path",
			url:        "/v1/package/example.com/a/b?version=v1.2.3",
			wantStatus: http.StatusBadRequest,
			want: &Error{
				Code:    http.StatusBadRequest,
				Message: "ambiguous package path",
				Candidates: []Candidate{
					{ModulePath: "example.com/a/b", PackagePath: "example.com/a/b"},
					{ModulePath: "example.com/a", PackagePath: "example.com/a/b"},
				},
			},
		},
		{
			name:       "disambiguated path",
			url:        "/v1/package/example.com/a/b?version=v1.2.3&module=example.com/a",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          pkgPath,
				ModulePath:    modulePath1,
				ModuleVersion: version,
				Synopsis:      "Synopsis for " + modulePath1,
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "default build context",
			url:        "/v1/package/example.com/pkg?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				IsLatest:      false,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "latest version",
			url:        "/v1/package/example.com/pkg?version=v1.2.4",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: "v1.2.4",
				Synopsis:      "Basic synopsis",
				IsLatest:      true,
				GOOS:          "linux",
				GOARCH:        "amd64",
			},
		},
		{
			name:       "doc",
			url:        "/v1/package/example.com/pkg?version=v1.2.3&doc=text",
			wantStatus: http.StatusOK,
			want: &Package{
				Path:          "example.com/pkg",
				ModulePath:    "example.com",
				ModuleVersion: version,
				Synopsis:      "This is a package synopsis for GOOS=linux, GOARCH=amd64",
				GOOS:          "linux",
				GOARCH:        "amd64",
				Docs:          "package p\n\nPackage p is a package.\n\n# Links\n\n- pkg.go.dev, https://pkg.go.dev\n\nVARIABLES\n\nvar V int\n\n",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServePackage(w, r, ds)
			if err != nil {
				t.Fatalf("ServePackage returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d. Body: %s", w.Code, test.wantStatus, w.Body.String())
			}

			if test.want != nil {
				got, err := unmarshalResponse[Package](w.Body.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestServeModule(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	const (
		modulePath = "example.com"
		version    = "v1.2.3"
	)

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{
			ModulePath: modulePath,
			Version:    version,
		},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path: modulePath,
				ModuleInfo: internal.ModuleInfo{
					ModulePath: modulePath,
					Version:    version,
				},
			},
			Readme: &internal.Readme{Filepath: "README.md", Contents: "Hello world"},
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		want       any
	}{
		{
			name:       "basic module metadata",
			url:        "/v1/module/example.com?version=v1.2.3",
			wantStatus: http.StatusOK,
			want: &Module{
				Path:    modulePath,
				Version: version,
			},
		},
		{
			name:       "bad version",
			url:        "/v1/module/example.com?version=nope",
			wantStatus: http.StatusNotFound,
			want:       &Error{Code: 404, Message: "could not find module for import path example.com: not found"},
		},
		{
			name:       "module with readme",
			url:        "/v1/module/example.com?version=v1.2.3&readme=true",
			wantStatus: http.StatusOK,
			want: &Module{
				Path:    modulePath,
				Version: version,
				Readme: &Readme{
					Filepath: "README.md",
					Contents: "Hello world",
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServeModule(w, r, ds)
			if err != nil && w.Code != test.wantStatus {
				t.Fatalf("ServeModule returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.want != nil {
				got, err := unmarshalResponse[Module](w.Body.Bytes())
				if err != nil {
					t.Fatalf("unmarshaling: %v", err)
				}
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestServeModuleVersions(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{ModulePath: "example.com", Version: "v1.0.0"},
		Units:      []*internal.Unit{{UnitMeta: internal.UnitMeta{Path: "example.com"}}},
	})
	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{ModulePath: "example.com", Version: "v1.1.0"},
		Units:      []*internal.Unit{{UnitMeta: internal.UnitMeta{Path: "example.com"}}},
	})
	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{ModulePath: "example.com/v2", Version: "v2.0.0"},
		Units:      []*internal.Unit{{UnitMeta: internal.UnitMeta{Path: "example.com/v2"}}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "all versions (cross-major)",
			url:        "/v1/versions/example.com",
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name:       "with limit",
			url:        "/v1/versions/example.com?limit=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "pagination",
			url:        "/v1/versions/example.com?limit=1&token=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServeModuleVersions(w, r, ds)
			if err != nil && w.Code != test.wantStatus {
				t.Fatalf("ServeModuleVersions returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[internal.ModuleInfo]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
				}
			}
		})
	}
}

func TestServeModulePackages(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	const (
		modulePath = "example.com"
		version    = "v1.0.0"
	)

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{ModulePath: modulePath, Version: version},
		Units: []*internal.Unit{
			{UnitMeta: internal.UnitMeta{Path: modulePath, Name: "pkg1"}},
			{UnitMeta: internal.UnitMeta{Path: modulePath + "/sub", Name: "pkg2"}},
		},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "all packages",
			url:        "/v1/packages/example.com?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServeModulePackages(w, r, ds)
			if err != nil && w.Code != test.wantStatus {
				t.Fatalf("ServeModulePackages returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[Package]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
				}
			}
		})
	}
}

func TestServeSearch(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{ModulePath: "example.com", Version: "v1.0.0"},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path:       "example.com/pkg",
				ModuleInfo: internal.ModuleInfo{ModulePath: "example.com", Version: "v1.0.0"},
				Name:       "pkg",
			},
			Documentation: []*internal.Documentation{{Synopsis: "A great package."}},
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "basic search",
			url:        "/v1/search?q=great",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "no results",
			url:        "/v1/search?q=nonexistent",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "missing query",
			url:        "/v1/search",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "search with filter",
			url:        "/v1/search?q=great&filter=example.com",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "search with non-matching filter",
			url:        "/v1/search?q=great&filter=nomatch",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServeSearch(w, r, ds)
			if err != nil {
				t.Fatalf("ServeSearch returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("%s: status = %d, want %d", test.name, w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[SearchResult]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("%s: json.Unmarshal: %v", test.name, err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("%s: count = %d, want %d", test.name, len(got.Items), test.wantCount)
				}
			}
		})
	}
}

func TestServeSearchPagination(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	for i := 0; i < 10; i++ {
		pkgPath := "example.com/pkg" + strconv.Itoa(i)
		ds.MustInsertModule(ctx, &internal.Module{
			ModuleInfo: internal.ModuleInfo{ModulePath: pkgPath, Version: "v1.0.0"},
			Units: []*internal.Unit{{
				UnitMeta: internal.UnitMeta{
					Path:       pkgPath,
					ModuleInfo: internal.ModuleInfo{ModulePath: pkgPath, Version: "v1.0.0"},
					Name:       "pkg",
				},
				Documentation: []*internal.Documentation{{Synopsis: "Synopsis" + strconv.Itoa(i)}},
			}},
		})
	}

	for _, test := range []struct {
		name          string
		url           string
		wantCount     int
		wantTotal     int
		wantNextToken string
	}{
		{
			name:          "first page",
			url:           "/v1/search?q=Synopsis&limit=3",
			wantCount:     3,
			wantTotal:     10,
			wantNextToken: "3",
		},
		{
			name:          "second page",
			url:           "/v1/search?q=Synopsis&limit=3&token=3",
			wantCount:     3,
			wantTotal:     10,
			wantNextToken: "6",
		},
		{
			name:          "last page",
			url:           "/v1/search?q=Synopsis&limit=3&token=9",
			wantCount:     1,
			wantTotal:     10,
			wantNextToken: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServeSearch(w, r, ds); err != nil {
				t.Fatalf("ServeSearch error: %v", err)
			}

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}

			var got PaginatedResponse[SearchResult]
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}

			if len(got.Items) != test.wantCount {
				t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
			}
			if got.Total != test.wantTotal {
				t.Errorf("total = %d, want %d", got.Total, test.wantTotal)
			}
			if got.NextPageToken != test.wantNextToken {
				t.Errorf("nextToken = %q, want %q", got.NextPageToken, test.wantNextToken)
			}
		})
	}
}

func TestServePackageSymbols(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	const (
		pkgPath    = "example.com/pkg"
		modulePath = "example.com"
		version    = "v1.0.0"
	)

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{ModulePath: modulePath, Version: version},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path:       pkgPath,
				ModuleInfo: internal.ModuleInfo{ModulePath: modulePath, Version: version},
				Name:       "pkg",
			},
			Symbols: map[internal.BuildContext][]*internal.Symbol{
				{GOOS: "linux", GOARCH: "amd64"}: {
					{
						SymbolMeta: internal.SymbolMeta{Name: "LinuxSym", Kind: internal.SymbolKindFunction},
						GOOS:       "linux",
						GOARCH:     "amd64",
					},
					{
						SymbolMeta: internal.SymbolMeta{Name: "T", Kind: internal.SymbolKindType},
						GOOS:       "linux",
						GOARCH:     "amd64",
						Children: []*internal.SymbolMeta{
							{Name: "T.M", Kind: internal.SymbolKindMethod, ParentName: "T"},
						},
					},
				},
				{GOOS: "windows", GOARCH: "amd64"}: {
					{SymbolMeta: internal.SymbolMeta{Name: "WindowsSym", Kind: internal.SymbolKindFunction}, GOOS: "windows", GOARCH: "amd64"},
				},
				{GOOS: "js", GOARCH: "wasm"}: {
					{SymbolMeta: internal.SymbolMeta{Name: "WasmSym", Kind: internal.SymbolKindFunction}, GOOS: "js", GOARCH: "wasm"},
				},
			},
		}},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
		wantName   string // Check name of the first symbol to verify build context
	}{
		{
			name:       "default best match (linux)",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantName:   "LinuxSym",
		},
		{
			name:       "explicit linux",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=linux&goarch=amd64",
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantName:   "LinuxSym",
		},
		{
			name:       "version latest",
			url:        "/v1/symbols/example.com/pkg?version=latest",
			wantStatus: http.StatusOK,
			wantCount:  2,
			wantName:   "LinuxSym",
		},
		{
			name:       "explicit windows",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=windows&goarch=amd64",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantName:   "WindowsSym",
		},
		{
			name:       "explicit wasm",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=js&goarch=wasm",
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantName:   "WasmSym",
		},
		{
			name:       "not found build context",
			url:        "/v1/symbols/example.com/pkg?version=v1.0.0&goos=darwin&goarch=amd64",
			wantStatus: http.StatusNotFound,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServePackageSymbols(w, r, ds)
			if err != nil && w.Code != test.wantStatus {
				t.Fatalf("ServePackageSymbols returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d. Body: %s", w.Code, test.wantStatus, w.Body.String())
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[Symbol]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
				}
				if test.wantName != "" && got.Items[0].Name != test.wantName {
					t.Errorf("first symbol = %q, want %q", got.Items[0].Name, test.wantName)
				}
			}
		})
	}
}

func TestServePackageImportedBy(t *testing.T) {
	ctx := context.Background()
	ds := fakedatasource.New()

	const (
		pkgPath    = "example.com/pkg"
		modulePath = "example.com"
		version    = "v1.0.0"
	)

	ds.MustInsertModule(ctx, &internal.Module{
		ModuleInfo: internal.ModuleInfo{ModulePath: modulePath, Version: version},
		Units: []*internal.Unit{
			{UnitMeta: internal.UnitMeta{Path: pkgPath, ModuleInfo: internal.ModuleInfo{ModulePath: modulePath, Version: version}}},
			{
				UnitMeta: internal.UnitMeta{Path: "example.com/other", ModuleInfo: internal.ModuleInfo{ModulePath: modulePath, Version: version}},
				Imports:  []string{pkgPath},
			},
		},
	})

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "all imported by",
			url:        "/v1/imported-by/example.com/pkg?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServePackageImportedBy(w, r, ds)
			if err != nil && w.Code != test.wantStatus {
				t.Fatalf("ServePackageImportedBy returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PackageImportedBy
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.ImportedBy.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.ImportedBy.Items), test.wantCount)
				}
			}
		})
	}
}

func TestServeVulnerabilities(t *testing.T) {
	ds := fakedatasource.New()
	vc, err := vuln.NewInMemoryClient([]*osv.Entry{
		{
			ID:      "VULN-1",
			Summary: "Vulnerability 1",
			Affected: []osv.Affected{
				{
					Module: osv.Module{Path: "example.com"},
					Ranges: []osv.Range{{Type: osv.RangeTypeSemver, Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.1.0"}}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "all vulns",
			url:        "/v1/vulns/example.com?version=v1.0.0",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "no vulns",
			url:        "/v1/vulns/example.com?version=v1.2.0",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			err := ServeVulnerabilities(vc)(w, r, ds)
			if err != nil && w.Code != test.wantStatus {
				t.Fatalf("ServeVulnerabilities returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[Vulnerability]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(got.Items) != test.wantCount {
					t.Errorf("count = %d, want %d", len(got.Items), test.wantCount)
				}
			}
		})
	}
}

// unmarshalResponse unmarshals an API response into either
// a *T or an *Error.
func unmarshalResponse[T any](data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var t T
	err1 := d.Decode(&t)
	if err1 == nil {
		return &t, nil
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var e Error
	err2 := d.Decode(&e)
	if err2 == nil {
		return &e, nil
	}
	return nil, errors.Join(err1, err2)
}
