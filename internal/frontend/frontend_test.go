// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
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
	s.InstallFS("/dir", os.DirFS("."))
	// Request this file.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/files/dir/frontend_test.go", nil))
	if w.Code != http.StatusOK {
		t.Errorf("got status code = %d, want %d", w.Code, http.StatusOK)
	}
	if want := "TestInstallFS"; !strings.Contains(w.Body.String(), want) {
		t.Errorf("body does not contain %q", want)
	}
}
