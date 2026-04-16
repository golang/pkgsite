// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/internal/vuln"
)

// Tests here do not depend on postgres.
// See internal/tests/api for the ones that do.

func TestServeVulnerabilities(t *testing.T) {
	// This test doesn't need to run against a Postgres DB, because
	// vulnerabilities are not read from the database.
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
		want       any
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

			if err := ServeVulnerabilities(vc)(w, r, nil); err != nil {
				ServeError(w, r, err)
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

func TestCacheControl(t *testing.T) {
	// This test doesn't need to run against a Postgres DB, because
	// it's concerned only with headers.
	ds := fakedatasource.New()
	const modulePath = "example.com"
	for _, v := range []string{"v1.0.0", "master"} {
		ds.MustInsertModule(t, &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: modulePath,
				Version:    v,
			},
			Units: []*internal.Unit{{
				UnitMeta: internal.UnitMeta{
					Path: modulePath,
					ModuleInfo: internal.ModuleInfo{
						ModulePath: modulePath,
						Version:    v,
					},
				},
			}},
		})
	}

	for _, test := range []struct {
		version string
		want    string
	}{
		{"v1.0.0", "public, max-age=10800"},
		{"latest", "public, max-age=3600"},
		{"master", "public, max-age=3600"},
		{"", "public, max-age=3600"},
	} {
		t.Run(test.version, func(t *testing.T) {
			url := "/v1/module/" + modulePath
			if test.version != "" {
				url += "?version=" + test.version
			}
			r := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			if err := ServeModule(w, r, ds); err != nil {
				t.Fatal(err)
			}

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}

			got := w.Header().Get("Cache-Control")
			if got != test.want {
				t.Errorf("Cache-Control = %q, want %q", got, test.want)
			}
		})
	}
}
