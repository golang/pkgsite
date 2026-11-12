// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
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
		{
			ID:      "VULN-2",
			Summary: "Vulnerability 2",
			Affected: []osv.Affected{
				{
					Module: osv.Module{Path: "example.com"},
					Ranges: []osv.Range{{Type: osv.RangeTypeSemver, Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.1.0"}}}},
					EcosystemSpecific: osv.EcosystemSpecific{
						Packages: []osv.Package{
							{Path: "example.com/pkg"},
						},
					},
				},
			},
		},
		{
			ID:      "VULN-3",
			Summary: "Vulnerability 3",
			Affected: []osv.Affected{
				{
					Module: osv.Module{Path: "example.com"},
					Ranges: []osv.Range{{Type: osv.RangeTypeSemver, Events: []osv.RangeEvent{{Introduced: "0"}, {Fixed: "1.1.0"}}}},
					EcosystemSpecific: osv.EcosystemSpecific{
						Packages: []osv.Package{
							{Path: "example.com/other"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ds := fakedatasource.New()
	insertModule := func(v string) {
		ds.MustInsertModule(t, &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath: "example.com",
				Version:    v,
			},
			Units: []*internal.Unit{
				{
					UnitMeta: internal.UnitMeta{
						Path: "example.com",
						ModuleInfo: internal.ModuleInfo{
							ModulePath: "example.com",
							Version:    v,
						},
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Path: "example.com/pkg",
						ModuleInfo: internal.ModuleInfo{
							ModulePath: "example.com",
							Version:    v,
						},
					},
				},
				{
					UnitMeta: internal.UnitMeta{
						Path: "example.com/other",
						ModuleInfo: internal.ModuleInfo{
							ModulePath: "example.com",
							Version:    v,
						},
					},
				},
			},
		})
	}
	insertModule("v1.0.0")
	insertModule("v1.2.0")

	vuln1 := Vulnerability{ID: "VULN-1", Details: "Vulnerability 1"}
	vuln2 := Vulnerability{ID: "VULN-2", Details: "Vulnerability 2"}
	vuln3 := Vulnerability{ID: "VULN-3", Details: "Vulnerability 3"}

	for _, test := range []struct {
		name       string
		url        string
		wantStatus int
		want       []Vulnerability
	}{
		{
			name:       "all vulns",
			url:        "/v1/vulns/example.com?version=v1.0.0",
			wantStatus: http.StatusOK,
			want:       []Vulnerability{vuln1, vuln2, vuln3},
		},
		{
			name:       "no vulns",
			url:        "/v1/vulns/example.com?version=v1.2.0",
			wantStatus: http.StatusOK,
		},
		{
			name:       "package path in vulns endpoint",
			url:        "/v1/vulns/example.com/pkg?version=v1.0.0",
			wantStatus: http.StatusOK,
			want:       []Vulnerability{vuln1, vuln2},
		},
		{
			name:       "another package path",
			url:        "/v1/vulns/example.com/other?version=v1.0.0",
			wantStatus: http.StatusOK,
			want:       []Vulnerability{vuln1, vuln3},
		},
		{
			name:       "filter by id",
			url:        `/v1/vulns/example.com?version=v1.0.0&filter=id=="VULN-1"`,
			wantStatus: http.StatusOK,
			want:       []Vulnerability{vuln1},
		},
		{
			name:       "filter by details",
			url:        `/v1/vulns/example.com?version=v1.0.0&filter=contains(details,"2")`,
			wantStatus: http.StatusOK,
			want:       []Vulnerability{vuln2},
		},
		// Summary and FixedVersion are currently unpopulated, so filters see "".
		{
			name:       "filter by summary empty",
			url:        `/v1/vulns/example.com?version=v1.0.0&filter=summary==""`,
			wantStatus: http.StatusOK,
			want:       []Vulnerability{vuln1, vuln2, vuln3},
		},
		{
			name:       "filter by summary non-empty",
			url:        `/v1/vulns/example.com?version=v1.0.0&filter=summary!=""`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "filter by fixedVersion empty",
			url:        `/v1/vulns/example.com?version=v1.0.0&filter=fixedVersion==""`,
			wantStatus: http.StatusOK,
			want:       []Vulnerability{vuln1, vuln2, vuln3},
		},
		{
			name:       "filter by fixedVersion non-empty",
			url:        `/v1/vulns/example.com?version=v1.0.0&filter=fixedVersion!=""`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "filter by Go field name fails",
			url:        `/v1/vulns/example.com?version=v1.0.0&filter=ID=="VULN-1"`,
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()

			if err := ServeVulnerabilities(vc)(w, r, ds); err != nil {
				ServeError(w, r, err)
			}

			if w.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, test.wantStatus, w.Body.String())
			}

			if test.wantStatus == http.StatusOK {
				var got PaginatedResponse[Vulnerability]
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if diff := cmp.Diff(test.want, got.Items, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("items mismatch (-want +got):\n%s", diff)
				}
				if got.Total != len(test.want) {
					t.Errorf("total = %d, want %d", got.Total, len(test.want))
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

func TestServeErrorCacheControl(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCC     string
	}{
		{
			name:       "404 not found is cached for 10 min",
			err:        fmt.Errorf("module not found: %w", derrors.NotFound),
			wantStatus: http.StatusNotFound,
			wantCC:     "public, max-age=600",
		},
		{
			name:       "400 bad request is cached for 10 min",
			err:        BadRequest("invalid query", "details"),
			wantStatus: http.StatusBadRequest,
			wantCC:     "public, max-age=600",
		},
		{
			name:       "500 internal error is not cached",
			err:        errors.New("db failure"),
			wantStatus: http.StatusInternalServerError,
			wantCC:     "no-store",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/module/nonexistent", nil)
			w := httptest.NewRecorder()
			if err := ServeError(w, r, test.err); err != nil {
				t.Fatal(err)
			}
			if w.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, test.wantStatus)
			}
			if got := w.Header().Get("Cache-Control"); got != test.wantCC {
				t.Errorf("Cache-Control = %q, want %q", got, test.wantCC)
			}
		})
	}
}

func TestVersionInPath(t *testing.T) {
	ds := fakedatasource.New()
	for _, tc := range []struct {
		url   string
		serve func(http.ResponseWriter, *http.Request, internal.DataSource) error
	}{
		{"/v1/package/example.com/pkg@v1.0.0", ServePackage},
		{"/v1/module/example.com@v1.0.0", ServeModule},
		{"/v1/packages/example.com@v1.0.0", ServeModulePackages},
		{"/v1/symbols/example.com/pkg@v1.0.0", ServePackageSymbols},
		{"/v1/imported-by/example.com/pkg@v1.0.0", ServePackageImportedBy},
		{"/v1/versions/example.com@v1.0.0", ServeModuleVersions},
		{"/v1/vulns/example.com@v1.0.0", ServeVulnerabilities(nil)},
	} {
		t.Run(tc.url, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.url, nil)
			w := httptest.NewRecorder()
			err := tc.serve(w, r, ds)
			if err == nil {
				t.Fatal("got nil error, want BadRequest")
			}
			if err := ServeError(w, r, err); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "?version=v1.0.0") {
				t.Errorf("body = %q, want substring '?version=v1.0.0'", w.Body.String())
			}
		})
	}
}
