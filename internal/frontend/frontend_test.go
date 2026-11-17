// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/api"
	"golang.org/x/pkgsite/internal/testing/fakedatasource"
	"golang.org/x/pkgsite/static"
	thirdparty "golang.org/x/pkgsite/third_party"
)

type testModule struct {
	path            string
	redistributable bool
	versions        []string
	packages        []testPackage
}

type testPackage struct {
	name           string
	suffix         string
	readmeContents string
	readmeFilePath string
	docs           []*internal.Documentation
}

func newTestServer(t *testing.T, cacher Cacher) (*Server, http.Handler) {
	t.Helper()

	s, err := NewServer(ServerConfig{
		DataSourceGetter: func(context.Context) internal.DataSource { return fakedatasource.New() },
		TemplateFS:       template.TrustedFSFromEmbed(static.FS),
		// Use the embedded FSs here to make sure they're tested.
		// Integration tests will use the actual directories.
		StaticFS:     static.FS,
		ThirdPartyFS: thirdparty.FS,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, cacher, nil)

	return s, mux
}

func TestHTMLInjection(t *testing.T) {
	_, handler := newTestServer(t, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/<em>UHOH</em>", nil))
	if strings.Contains(w.Body.String(), "<em>") {
		t.Error("User input was rendered unescaped.")
	}
}

func mustRequest(urlPath string, t *testing.T) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, "http://localhost"+urlPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDetailsTTL(t *testing.T) {
	tests := []struct {
		r    *http.Request
		want time.Duration
	}{
		{mustRequest("/host.com/module@v1.2.3/suffix", t), longTTL},
		{mustRequest("/host.com/module/suffix", t), shortTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=overview", t), longTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=versions", t), defaultTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=importedby", t), defaultTTL},
		{
			func() *http.Request {
				r := mustRequest("/host.com/module@v1.2.3/suffix?tab=overview", t)
				r.Header.Set("user-agent",
					"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)")
				return r
			}(),
			tinyTTL,
		},
	}
	for _, test := range tests {
		if got := detailsTTL(test.r); got != test.want {
			t.Errorf("detailsTTL(%v) = %v, want %v", test.r, got, test.want)
		}
	}
}

func TestSearchTTL(t *testing.T) {
	tests := []struct {
		path string
		want time.Duration
	}{
		{"/search?q=http&m=package", symbolSearchTTL},
		{"/search?q=New&m=symbol", slowSymbolSearchTTL},
		{"/v1/search?q=http", symbolSearchTTL},
		{"/v1/search?q=http&symbol=ListenAndServe", symbolSearchTTL},
		{"/v1/search?q=http&symbol=New", slowSymbolSearchTTL},
		{"/v1/search?symbol=client", slowSymbolSearchTTL},
	}
	for _, test := range tests {
		r := mustRequest(test.path, t)
		if got := searchTTL(r); got != test.want {
			t.Errorf("searchTTL(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

type fakeCacher map[string][]byte

func (fc fakeCacher) Cache(string, func(*http.Request) time.Duration, []string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if b, ok := fc[r.URL.String()]; ok {
				w.Write(b)
				return
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			maps.Copy(w.Header(), rec.Header())
			w.WriteHeader(rec.Code)
			w.Write(rec.Body.Bytes())
			if rec.Code == http.StatusOK {
				fc[r.URL.String()] = rec.Body.Bytes()
			}
		})
	}
}

func TestAPISearchCache(t *testing.T) {
	cacher := make(fakeCacher)
	_, handler := newTestServer(t, cacher)

	t.Run("cache miss then hit preserves headers and body", func(t *testing.T) {
		for i := range 2 {
			if i == 1 {
				if _, ok := cacher["/v1/search?q=foo"]; !ok {
					t.Fatal("expected first request to populate cache")
				}
				cacher["/v1/search?q=foo"] = []byte("cached")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", "/v1/search?q=foo", nil))
			if got, want := w.Code, http.StatusOK; got != want {
				t.Fatalf("req %d: got status %d, want %d", i, got, want)
			}
			if got, want := w.Header().Get("Content-Type"), "application/json"; got != want {
				t.Errorf("req %d: got Content-Type %q, want %q", i, got, want)
			}
			if got, want := w.Header().Get("Cache-Control"), "public, max-age=3600"; got != want {
				t.Errorf("req %d: got Cache-Control %q, want %q", i, got, want)
			}
			if i == 1 && w.Body.String() != "cached" {
				t.Errorf("got body %q, want %q", w.Body.String(), "cached")
			}
		}
	})

	t.Run("error response overwrites Cache-Control and is not cached", func(t *testing.T) {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/v1/search", nil))
		if got, want := w.Code, http.StatusBadRequest; got != want {
			t.Fatalf("got status %d, want %d", got, want)
		}
		if got, want := w.Header().Get("Cache-Control"), "public, max-age=600"; got != want {
			t.Errorf("got Cache-Control %q, want %q", got, want)
		}
		if _, ok := cacher["/v1/search"]; ok {
			t.Error("expected 400 error response not to be cached")
		}
	})
}

func TestTagRoute(t *testing.T) {
	mustRequest := func(url string) *http.Request {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	tests := []struct {
		route string
		req   *http.Request
		want  string
	}{
		{"/pkg", mustRequest("http://localhost/pkg/foo?tab=versions"), "pkg-versions"},
		{"/", mustRequest("http://localhost/foo?tab=imports"), "imports"},
		{"/search", mustRequest("http://localhost/search?q=net&m=vuln"), "search-vuln"},
		{"/search", mustRequest("http://localhost/search?q=net&m=package"), "search-package"},
		{"/search", mustRequest("http://localhost/search?q=net&m=symbol"), "search-symbol"},
		{"/search", mustRequest("http://localhost/search?q=net"), "search-package"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			if got := TagRoute(test.route, test.req); got != test.want {
				t.Errorf("TagRoute(%q, %v) = %q, want %q", test.route, test.req, got, test.want)
			}
		})
	}
}

func TestInstallFS(t *testing.T) {
	s, handler := newTestServer(t, nil)
	s.InstallFS("/a dir", os.DirFS("."))
	// Request this file.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/files/a%20dir/frontend_test.go", nil))
	if w.Code != http.StatusOK {
		t.Errorf("got status code = %d, want %d", w.Code, http.StatusOK)
	}
	if want := "TestInstallFS"; !strings.Contains(w.Body.String(), want) {
		t.Errorf("body does not contain %q", want)
	}
}

func TestAPIDiscoverability(t *testing.T) {
	cacher := fakeCacher{
		"/example.com/pkg":           []byte("<html></html>"),
		"/example.com/pkg?tab=evals": []byte("<html></html>"),
	}
	_, handler := newTestServer(t, cacher)
	searchLink := apiDiscoveryLinkHeader + `, </v1/search?q=foo%3Ebar>; rel="alternate"; type="application/json"`
	tests := []struct {
		method, path, wantLink string
	}{
		{"GET", "/", ""},
		{"GET", "/about", ""},
		{"GET", "/nonexistent.example.com/pkg", ""},
		{"GET", "/search?q=", ""},
		{"GET", "/search?q=foo%3Ebar&m=package", searchLink},
		{"GET", "/search?q=json+Unmarshal&m=symbol", ""},
		{"GET", "/example.com/pkg", apiDiscoveryLinkHeader},
		{"HEAD", "/example.com/pkg", apiDiscoveryLinkHeader},
		{"POST", "/example.com/pkg", ""},
		{"GET", "/example.com/pkg?tab=evals", ""},
	}
	for _, test := range tests {
		for range 2 {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
			if got := w.Header().Get("Link"); got != test.wantLink {
				t.Errorf("%s %s: Link = %q, want %q", test.method, test.path, got, test.wantLink)
			}
			if test.wantLink == "" && strings.Contains(w.Body.String(), `rel="service-desc"`) {
				t.Errorf("%s %s: body unexpectedly links to the API", test.method, test.path)
			}
		}
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/search?q=foo", nil))
	for _, want := range []string{
		`<link rel="service-desc" type="text/yaml" href="/v1/openapi.yaml"`,
		`<link rel="service-doc" href="/v1/api"`,
		`<link rel="alternate" type="application/json" href="/v1/search?q=foo"`,
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("GET /search?q=foo: body does not contain %s", want)
		}
	}
}

func TestUnitAPIURL(t *testing.T) {
	pkg := &internal.UnitMeta{
		Path:       "golang.org/x/sync/errgroup",
		Name:       "errgroup",
		ModuleInfo: internal.ModuleInfo{ModulePath: "golang.org/x/sync"},
	}
	mod := &internal.UnitMeta{
		Path:       "golang.org/x/mod",
		ModuleInfo: internal.ModuleInfo{ModulePath: "golang.org/x/mod"},
	}
	tests := []struct {
		um           *internal.UnitMeta
		version, tab string
		want         string
	}{
		{pkg, "latest", tabMain, "/v1/package/golang.org/x/sync/errgroup?doc=md"},
		{pkg, "latest", tabImports, "/v1/package/golang.org/x/sync/errgroup?imports=true"},
		{pkg, "latest", tabImportedBy, "/v1/imported-by/golang.org/x/sync/errgroup"},
		{pkg, "v0.10.0", tabLicenses, "/v1/package/golang.org/x/sync/errgroup?licenses=true&version=v0.10.0"},
		{pkg, "v2.2.2+incompatible", tabMain, "/v1/package/golang.org/x/sync/errgroup?doc=md&version=v2.2.2%2Bincompatible"},
		{pkg, "latest", tabEvals, ""},
		{mod, "latest", tabMain, "/v1/packages/golang.org/x/mod"},
		{mod, "latest", tabLicenses, "/v1/module/golang.org/x/mod?licenses=true"},
		{mod, "v0.14.0", tabVersions, "/v1/versions/golang.org/x/mod"},
	}
	for _, test := range tests {
		if got := unitAPIURL(test.um, test.version, test.tab); got != test.want {
			t.Errorf("unitAPIURL(%q, %q, %q) = %q, want %q", test.um.Path, test.version, test.tab, got, test.want)
		}
	}
}

func TestSearchAPIURL(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{"q=foo&limit=50", "/v1/search?limit=50&q=foo"},
		{"q=Unmarshal&m=package", "/v1/search?q=Unmarshal"},
		{"q=Unmarshal", "/v1/search?symbol=Unmarshal"},
		{"q=json.Unmarshal", "/v1/search?symbol=json.Unmarshal"},
		{"q=foo&m=symbol&limit=50", "/v1/search?limit=50&symbol=foo"},
		{"q=%23Unmarshal", "/v1/search?symbol=Unmarshal"},
		{"q=%23unmarshal+json", "/v1/search?q=json&symbol=unmarshal"},
		{"q=json+%23unmarshal", "/v1/search?q=json&symbol=unmarshal"},
		{"q=%23yaml+yaml", "/v1/search?q=yaml&symbol=yaml"},
		{"q=json+Unmarshal&m=symbol", ""},
		{"q=%23json+%23Unmarshal", ""},
		{"q=", ""},
	}
	for _, test := range tests {
		r := httptest.NewRequest("GET", "/search?"+test.query, nil)
		if got := searchAPIURL(r); got != test.want {
			t.Errorf("searchAPIURL(%q) = %q, want %q", test.query, got, test.want)
		}
	}
}

func TestAPIRedirect(t *testing.T) {
	_, handler := newTestServer(t, nil)

	for _, tc := range []struct {
		name     string
		path     string
		wantCode int
		wantLoc  string
	}{
		{"redirect /api", "/api", http.StatusFound, "/v1/api"},
		{"redirect /api/", "/api/", http.StatusFound, "/v1/api"},
		{"redirect /v1", "/v1", http.StatusFound, "/v1/api"},
		{"redirect /v1/", "/v1/", http.StatusFound, "/v1/api"},
		{"redirect /v1beta", "/v1beta", http.StatusMovedPermanently, "/v1/api"},
		{"redirect /v1beta/", "/v1beta/", http.StatusMovedPermanently, "/v1/api"},
		{"redirect /v1beta/ path", "/v1beta/moo", http.StatusMovedPermanently, "/v1/moo"},
		{"redirect /v1beta/ path with query", "/v1beta/search?q=uuid&limit=25", http.StatusMovedPermanently, "/v1/search?q=uuid&limit=25"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.wantCode {
				t.Errorf("got status code = %d, want %d", w.Code, tc.wantCode)
			}
			if got := w.Header().Get("Location"); got != tc.wantLoc {
				t.Errorf("got Location = %q, want %q", got, tc.wantLoc)
			}
		})
	}
}

func TestAPIUnknownEndpoint(t *testing.T) {
	_, handler := newTestServer(t, nil)
	for _, tc := range []struct {
		method, path string
	}{
		{"GET", "/v1/moo"},
		{"GET", "/api/search"},
		{"GET", "/api/v1/package/foo"},
		{"POST", "/api/"},
		{"POST", "/v1/"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if got, want := w.Code, http.StatusBadRequest; got != want {
				t.Errorf("%s %q status = %d, want %d", tc.method, tc.path, got, want)
			}
			var bodyErr api.Error
			if err := json.Unmarshal(w.Body.Bytes(), &bodyErr); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.ToLower(bodyErr.Message), "unknown") {
				t.Errorf("%s %q error message = %q, want substring 'unknown'", tc.method, tc.path, bodyErr.Message)
			}
		})
	}
}
