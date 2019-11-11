// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/discovery/internal/stdlib"
)

const testTimeout = 5 * time.Second

var testDB *postgres.DB

func TestMain(m *testing.M) {
	postgres.RunDBTests("discovery_frontend_test", m, &testDB)
}

func TestHTMLInjection(t *testing.T) {
	s, err := NewServer(testDB, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/<em>UHOH</em>", nil))
	if strings.Contains(w.Body.String(), "<em>") {
		t.Error("User input was rendered unescaped.")
	}
}

func TestServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	mustInsertVersion := func(modulePath, version string, pkgs []*internal.Package) {
		v := sample.Version()
		v.ModulePath = modulePath
		v.Version = version
		v.Packages = pkgs
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	pkg := sample.Package()
	pkg2 := sample.Package()
	pkg2.Path = sample.ModulePath + "/foo/directory/hello"
	pkg2.DocumentationHTML = []byte(`<a href="/pkg/io#Writer">io.Writer</a>`)
	mustInsertVersion(sample.ModulePath, "v0.9.0", []*internal.Package{pkg, pkg2})
	mustInsertVersion(sample.ModulePath, "v1.0.0", []*internal.Package{pkg, pkg2})

	nonRedistModulePath := "github.com/non_redistributable"
	nonRedistPkgPath := nonRedistModulePath + "/bar"
	mustInsertVersion(nonRedistModulePath, "v1.0.0", []*internal.Package{{
		Name:   "bar",
		Path:   nonRedistPkgPath,
		V1Path: nonRedistPkgPath,
	}})

	pkgCmdGo := sample.Package()
	pkgCmdGo.Name = "main"
	pkgCmdGo.Path = "cmd/go"
	mustInsertVersion(stdlib.ModulePath, "v1.13.0", []*internal.Package{pkgCmdGo})

	s, err := NewServer(testDB, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)

	type header struct {
		// suffix is not used for the module header.
		// the fields must be exported for use by template.Execute.
		Version, Title, Suffix, ModuleURL, LatestURL, URLPath string
		notLatest                                             bool
	}

	mustExecuteTemplate := func(h *header, s string) string {
		t.Helper()
		tmpl := template.Must(template.New("").Parse(s))
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, h); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	licenseInfo := func(h *header, latest bool) string {
		if h.URLPath == "" {
			return `None detected`
		}
		var path string
		if latest {
			path = h.LatestURL
		} else {
			path = h.URLPath
		}
		return fmt.Sprintf(`<a href="/%s?tab=licenses#LICENSE">MIT</a>`, path)
	}
	pkgHeader := func(h *header, latest bool) []string {
		return []string{
			mustExecuteTemplate(h, `<span class="DetailsHeader-breadcrumbCurrent">{{.Suffix}}</span>`),
			mustExecuteTemplate(h, `<h1 class="DetailsHeader-title">{{.Title}}</h1>`),
			mustExecuteTemplate(h, `<div class="DetailsHeader-version">{{.Version}}</div>`),
			licenseInfo(h, latest),
			h.ModuleURL,
		}
	}
	modHeader := func(h *header, latest bool) []string {
		return []string{
			mustExecuteTemplate(h, `<h1 class="DetailsHeader-title">{{.Title}}</h1>`),
			mustExecuteTemplate(h, `<div class="DetailsHeader-version">{{.Version}}</div>`),
			licenseInfo(h, latest),
		}
	}

	pkgV100 := &header{
		Version:   "v1.0.0",
		Suffix:    "foo",
		Title:     "Package foo",
		URLPath:   `github.com/valid_module_name@v1.0.0/foo`,
		LatestURL: "github.com/valid_module_name/foo",
	}
	pkgV090 := &header{
		Version:   "v0.9.0",
		Suffix:    "foo",
		Title:     "Package foo",
		URLPath:   `github.com/valid_module_name@v0.9.0/foo`,
		notLatest: true,
	}
	pkgNonRedist := &header{
		Version: "v1.0.0",
		Suffix:  "bar",
		Title:   "Package bar",
	}
	cmdGo := &header{
		Suffix:    "go",
		Version:   "go1.13",
		Title:     "Command go",
		URLPath:   `cmd/go@go1.13`,
		LatestURL: "cmd/go",
	}
	mod := &header{
		Version:   "v1.0.0",
		Title:     "Module github.com/valid_module_name",
		URLPath:   `mod/github.com/valid_module_name@v1.0.0`,
		LatestURL: "mod/github.com/valid_module_name",
	}
	std := &header{
		Version:   "go1.13",
		Title:     "Standard library",
		URLPath:   `std@go1.13`,
		LatestURL: `std`,
	}
	dir := &header{
		Suffix:    "directory",
		Version:   "v1.0.0",
		Title:     "Directory github.com/valid_module_name/foo/directory",
		URLPath:   `github.com/valid_module_name@v1.0.0/foo/directory`,
		LatestURL: `github.com/valid_module_name/foo/directory`,
	}
	dirCmd := &header{
		Suffix:    "cmd",
		Version:   "go1.13",
		Title:     "Directory cmd",
		URLPath:   `cmd@go1.13`,
		LatestURL: `cmd`,
	}

	pkgSuffix := strings.TrimPrefix(sample.PackagePath, sample.ModulePath+"/")
	nonRedistPkgSuffix := strings.TrimPrefix(nonRedistPkgPath, nonRedistModulePath+"/")
	for _, tc := range []struct {
		// name of the test
		name string
		// path to use in an HTTP GET request
		urlPath string
		// whether to mutate the identifier links in documentation.
		doDocumentationHack bool
		// statusCode we expect to see in the headers.
		wantStatusCode int
		// substrings we expect to see in the body
		want []string
	}{
		{
			name:           "static",
			urlPath:        "/static/",
			wantStatusCode: http.StatusOK,
			want:           []string{"css", "html", "img", "js"},
		},
		{
			name:           "license policy",
			urlPath:        "/license-policy",
			wantStatusCode: http.StatusOK,
			want: []string{
				"The Go website displays license information",
				"this is not legal advice",
			},
		},
		{
			// just check that it returns 200
			name:           "favicon",
			urlPath:        "/favicon.ico",
			wantStatusCode: http.StatusOK,
			want:           nil,
		},
		{
			name:           "robots.txt",
			urlPath:        "/robots.txt",
			wantStatusCode: http.StatusOK,
			want:           []string{"User-agent: *", "Disallow: /*?tab=*"},
		},
		{
			name:           "search",
			urlPath:        fmt.Sprintf("/search?q=%s", sample.PackageName),
			wantStatusCode: http.StatusOK,
			want: []string{
				`<a href="/github.com/valid_module_name/foo?tab=overview">github.com/valid_module_name/foo</a>`,
			},
		},
		{
			name:           "package default",
			urlPath:        fmt.Sprintf("/%s?tab=doc", sample.PackagePath),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, true),
				`This is the documentation HTML`,
			),
		},
		{
			name:           "package default redirect",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath),
			wantStatusCode: http.StatusFound,
			want:           []string{},
		},
		{
			name: "package default nonredistributable",
			// For a non-redistributable package, the "latest" route goes to the modules tab.
			urlPath:        fmt.Sprintf("/%s?tab=overview", nonRedistPkgPath),
			wantStatusCode: http.StatusOK,
			want:           pkgHeader(pkgNonRedist, true),
		},
		{
			name:           "package@version default",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=doc", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`This is the documentation HTML`,
			),
		},
		{
			name: "package@version default specific version nonredistributable",
			// For a non-redistributable package, the name@version route goes to the modules tab.
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=overview", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			wantStatusCode: http.StatusOK,
			want:           pkgHeader(pkgNonRedist, false),
		},
		{
			name:           "package@version doc tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=doc", sample.ModulePath, "v0.9.0", pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV090, false),
				`This is the documentation HTML`,
			),
		},
		{
			name:           "package@version doc with links",
			urlPath:        fmt.Sprintf("/%s?tab=doc", pkg2.Path),
			wantStatusCode: http.StatusOK,
			want:           []string{`<a href="/pkg/io#Writer">io.Writer</a>`},
		},
		{
			name:                "package@version doc with hacked up links",
			urlPath:             fmt.Sprintf("/%s?tab=doc", pkg2.Path),
			doDocumentationHack: true,
			wantStatusCode:      http.StatusOK,
			want:                []string{`<a href="/io?tab=doc#Writer">io.Writer</a>`},
		},
		{
			name: "package@version doc tab nonredistributable",
			// For a non-redistributable package, the doc tab will not show the doc.
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=doc", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgNonRedist, false),
				`hidden due to license restrictions`,
			),
		},
		{
			name:           "package@version readme tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=overview", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`<div class="Overview-module">`,
				`Repository: <a href="github.com/valid_module_name" target="_blank">github.com/valid_module_name</a>`,
				`<div class="Overview-readmeContent"><p>readme</p>`,
				`<div class="Overview-readmeSource">Source: github.com/valid_module_name@v1.0.0/README.md</div>`),
		},
		{
			name: "package@version readme tab nonredistributable",
			// For a non-redistributable package, the readme tab will not show the readme.
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=overview", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgNonRedist, false),
				`hidden due to license restrictions`,
			),
		},
		{
			name:           "package@version subdirectories tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=subdirectories", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`foo`,
			),
		},
		{
			name:           "package@version versions tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=versions", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`Versions`,
				`v1`,
				`<a href="/github.com/valid_module_name@v1.0.0/foo" title="v1.0.0">v1.0.0</a>`,
			),
		},
		{
			name:           "package@version imports tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=imports", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`Imports`,
				`Standard Library`,
				`<a href="/fmt">fmt</a>`,
				`<a href="/path/to/bar">path/to/bar</a>`),
		},
		{
			name:           "package@version imported by tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`No known importers for this package`,
			),
		},
		{
			name:           "package@version imported by tab second page",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=importedby&page=2", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`No known importers for this package`,
			),
		},
		{
			name:           "package@version licenses tab",
			urlPath:        fmt.Sprintf("/%s@%s/%s?tab=licenses", sample.ModulePath, sample.VersionString, pkgSuffix),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(pkgV100, false),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
				`<div class="License-source">Source: github.com/valid_module_name@v1.0.0/LICENSE</div>`,
			),
		},
		{
			name:           "directory subdirectories",
			urlPath:        fmt.Sprintf("/%s", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(dir, true),
				`<th>Path</th>`,
				`<th>Synopsis</th>`,
			),
		},
		{
			name:           "directory overview",
			urlPath:        fmt.Sprintf("/%s?tab=overview", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(dir, true),
				`<div class="Overview-module">`,
				`Repository: <a href="github.com/valid_module_name" target="_blank">github.com/valid_module_name</a>`,
				`<div class="Overview-readmeContent"><p>readme</p>`,
				`<div class="Overview-readmeSource">Source: github.com/valid_module_name@v1.0.0/README.md</div>`),
		},
		{
			name:           "directory licenses",
			urlPath:        fmt.Sprintf("/%s?tab=licenses", sample.PackagePath+"/directory"),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(dir, true),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
				`<div class="License-source">Source: github.com/valid_module_name@v1.0.0/LICENSE</div>`),
		},
		{
			name:           "stdlib directory default",
			urlPath:        fmt.Sprintf("/cmd"),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(dirCmd, true),
				`<th>Path</th>`,
				`<th>Synopsis</th>`,
			),
		},
		{
			name:           "stdlib directory subdirectories",
			urlPath:        fmt.Sprintf("/cmd@go1.13?tab=subdirectories"),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(dirCmd, false),
				`<th>Path</th>`,
				`<th>Synopsis</th>`,
			),
		},
		{
			name:           "stdlib directory overview",
			urlPath:        fmt.Sprintf("/cmd@go1.13?tab=overview"),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(dirCmd, false),
				`<div class="Overview-module">`,
				`Repository: <a href="github.com/valid_module_name" target="_blank">github.com/valid_module_name</a>`,
				`<div class="Overview-readmeContent"><p>readme</p>`,
				`<div class="Overview-readmeSource">Source: go.googlesource.com/go/&#43;/refs/tags/go1.13/README.md</div>`),
		},
		{
			name:           "stdlib directory licenses",
			urlPath:        fmt.Sprintf("/cmd@go1.13?tab=licenses"),
			wantStatusCode: http.StatusOK,
			want: append(
				pkgHeader(dirCmd, false),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
				`<div class="License-source">Source: go.googlesource.com/go/&#43;/refs/tags/go1.13/LICENSE</div>`),
		},
		{
			name:           "module default",
			urlPath:        fmt.Sprintf("/mod/%s", sample.ModulePath),
			wantStatusCode: http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			want: append(modHeader(mod, true), `readme`),
		},
		{
			name:           "module overview",
			urlPath:        fmt.Sprintf("/mod/%s?tab=overview", sample.ModulePath),
			wantStatusCode: http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			want: append(modHeader(mod, true), `readme`),
		},
		// TODO(b/139498072): add a second module, so we can verify that we get the latest version.
		{
			name:           "module packages tab latest version",
			urlPath:        fmt.Sprintf("/mod/%s?tab=packages", sample.ModulePath),
			wantStatusCode: http.StatusOK,
			// Fall back to the latest version.
			want: append(modHeader(mod, true), `This is a package synopsis`),
		},
		{
			name:           "module@version readme tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=overview", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want:           append(modHeader(mod, false), `readme`),
		},
		{
			name:           "module@version packages tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=packages", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want:           append(modHeader(mod, false), `This is a package synopsis`),
		},
		{
			name:           "module@version versions tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=versions", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: append(
				modHeader(mod, false),
				`Versions`,
				`v1`,
				`<a href="/mod/github.com/valid_module_name@v1.0.0" title="v1.0.0">v1.0.0</a>`,
			),
		},
		{
			name:           "module@version licenses tab",
			urlPath:        fmt.Sprintf("/mod/%s@%s?tab=licenses", sample.ModulePath, sample.VersionString),
			wantStatusCode: http.StatusOK,
			want: append(
				modHeader(mod, false),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
			),
		},
		{
			name:           "cmd go package page",
			urlPath:        "/cmd/go?tab=doc",
			wantStatusCode: http.StatusOK,
			want:           pkgHeader(cmdGo, true),
		},
		{
			name:           "cmd go package page at version",
			urlPath:        "/cmd/go@go1.13?tab=doc",
			wantStatusCode: http.StatusOK,
			want:           pkgHeader(cmdGo, false),
		},
		{
			name:           "standard library module page",
			urlPath:        "/std",
			wantStatusCode: http.StatusOK,
			want:           modHeader(std, true),
		},
		{
			name:           "standard library module page at version",
			urlPath:        "/std@go1.13",
			wantStatusCode: http.StatusOK,
			want:           modHeader(std, false),
		},
		{
			name:           "latest version for the standard library",
			urlPath:        "/latest-version/std",
			wantStatusCode: http.StatusOK,
			want:           []string{`"go1.13"`},
		},
		{
			name:           "latest version for module",
			urlPath:        "/latest-version/" + sample.ModulePath,
			wantStatusCode: http.StatusOK,
			want:           []string{`"v1.0.0"`},
		},
		{
			name:           "latest version for package",
			urlPath:        fmt.Sprintf("/latest-version/%s?pkg=%s", sample.ModulePath, pkg2.Path),
			wantStatusCode: http.StatusOK,
			want:           []string{`"v1.0.0"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) { // remove initial '/' for name
			defer func(orig bool) { doDocumentationHack = orig }(doDocumentationHack)
			doDocumentationHack = tc.doDocumentationHack
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", tc.urlPath, nil))
			res := w.Result()
			if res.StatusCode != tc.wantStatusCode {
				t.Errorf("GET %q = %d, want %d", tc.urlPath, res.StatusCode, tc.wantStatusCode)
			}
			bytes, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			body := string(bytes)
			for _, want := range tc.want {
				i := strings.Index(body, want)
				if i < 0 {
					b := body
					if len(b) > 100 {
						b = "<content exceeds 100 chars>"
					}
					t.Fatalf("`%s` not found in body\n%s", want, b)
					continue
				}
				// Truncate the body each time through the loop to make sure the wanted strings
				// are found in order.
				body = body[i+len(want):]
			}
		})
	}
}

func TestServerErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	sampleVersion := sample.Version()
	if err := testDB.InsertVersion(ctx, sampleVersion); err != nil {
		t.Fatal(err)
	}

	s, err := NewServer(testDB, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/invalid-page", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status code: got = %d, want %d", w.Code, http.StatusNotFound)
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

func TestPackageTTL(t *testing.T) {
	tests := []struct {
		r    *http.Request
		want time.Duration
	}{
		{mustRequest("/host.com/module@v1.2.3/suffix", t), longTTL},
		{mustRequest("/host.com/module/suffix", t), shortTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=overview", t), longTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=versions", t), defaultTTL},
		{mustRequest("/host.com/module@v1.2.3/suffix?tab=importedby", t), defaultTTL},
	}
	for _, test := range tests {
		if got := packageTTL(test.r); got != test.want {
			t.Errorf("packageTTL(%v) = %v, want %v", test.r, got, test.want)
		}
	}
}

func TestModuleTTL(t *testing.T) {
	tests := []struct {
		r    *http.Request
		want time.Duration
	}{
		{mustRequest("/mod/host.com/module@v1.2.3/suffix", t), longTTL},
		{mustRequest("/mod/host.com/module/suffix", t), shortTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=overview", t), longTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=versions", t), defaultTTL},
		{mustRequest("/mod/host.com/module@v1.2.3/suffix?tab=importedby", t), defaultTTL},
	}
	for _, test := range tests {
		if got := moduleTTL(test.r); got != test.want {
			t.Errorf("packageTTL(%v) = %v, want %v", test.r, got, test.want)
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
	}
	for _, test := range tests {
		if got := TagRoute(test.route, test.req); got != test.want {
			t.Errorf("TagRoute(%q, %v) = %q, want %q", test.route, test.req, got, test.want)
		}
	}
}
