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
			Documentation: []*internal.Documentation{{
				GOOS:     "linux",
				GOARCH:   "amd64",
				Synopsis: "Basic synopsis",
			}},
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
				Synopsis:      "Basic synopsis",
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
				Synopsis:      "Basic synopsis",
				GOOS:          "linux",
				GOARCH:        "amd64",
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
