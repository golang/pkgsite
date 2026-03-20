// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/testing/sample"
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
		ModuleInfo: internal.ModuleInfo{ModulePath: "example.com", Version: version},
		Units: []*internal.Unit{{
			UnitMeta: internal.UnitMeta{
				Path:       "example.com/pkg",
				ModuleInfo: internal.ModuleInfo{ModulePath: "example.com", Version: version},
				Name:       "pkg",
			},
			Documentation: []*internal.Documentation{sample.Documentation("linux", "amd64", sample.DocContents)},
		}},
	})

	for _, mp := range []string{modulePath1, modulePath2} {
		u := &internal.Unit{
			UnitMeta: internal.UnitMeta{
				Path: pkgPath,
				ModuleInfo: internal.ModuleInfo{
					ModulePath: mp,
					Version:    version,
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
				ModulePath: mp,
				Version:    version,
			},
			Units: []*internal.Unit{u},
		})
	}

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
				Docs:          "TODO\n",
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
				switch want := test.want.(type) {
				case *Package:
					var got Package
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatalf("json.Unmarshal Package: %v", err)
					}
					got.IsLatest = false
					if diff := cmp.Diff(want, &got); diff != "" {
						t.Errorf("mismatch (-want +got):\n%s", diff)
					}
				case *Error:
					var got Error
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatalf("json.Unmarshal Error: %v. Body: %s", err, w.Body.String())
					}
					if diff := cmp.Diff(want, &got); diff != "" {
						t.Errorf("mismatch (-want +got):\n%s", diff)
					}
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
		want       *Module
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
			if err != nil {
				t.Fatalf("ServeModule returned error: %v", err)
			}

			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}

			if test.want != nil {
				var got Module
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if diff := cmp.Diff(test.want, &got); diff != "" {
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
			if err != nil {
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
				if test.name == "with limit" && got.NextPageToken != "1" {
					t.Errorf("nextPageToken = %q, want %q", got.NextPageToken, "1")
				}
				if test.name == "pagination" && got.NextPageToken != "2" {
					t.Errorf("nextPageToken = %q, want %q", got.NextPageToken, "2")
				}
			}
		})
	}
}
