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
		// statusCode we expect to see in the headers.
		wantStatusCode int
		// substrings we expect to see in the body
		want []string
	}{
		{
			"static",
			"/static/",
			http.StatusOK,
			[]string{"css", "html", "img", "js"},
		},
		{
			"license policy",
			"/license-policy",
			http.StatusOK,
			[]string{
				"The Go website displays license information",
				"this is not legal advice",
			},
		},
		{
			// just check that it returns 200
			"favicon",
			"/favicon.ico",
			http.StatusOK,
			nil,
		},
		{
			"robots.txt",
			"/robots.txt",
			http.StatusOK,
			[]string{"User-agent: *", "Disallow: /*?tab=*"},
		},
		{
			"search",
			fmt.Sprintf("/search?q=%s", sample.PackageName),
			http.StatusOK,
			[]string{
				`<a href="/github.com/valid_module_name/foo">github.com/valid_module_name/foo</a>`,
			},
		},
		{
			"package default",
			fmt.Sprintf("/%s", sample.PackagePath),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, true),
				`This is the documentation HTML`,
			),
		},
		{
			"package default nonredistributable",
			// For a non-redistributable package, the "latest" route goes to the modules tab.
			fmt.Sprintf("/%s", nonRedistPkgPath),
			http.StatusOK,
			pkgHeader(pkgNonRedist, true),
		},
		{
			"package@version default",
			fmt.Sprintf("/%s@%s/%s", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`This is the documentation HTML`,
			),
		},
		{
			"package@version default specific version nonredistributable",
			// For a non-redistributable package, the name@version route goes to the modules tab.
			fmt.Sprintf("/%s@%s/%s", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			http.StatusOK,
			pkgHeader(pkgNonRedist, false),
		},
		{
			"package@version doc tab",
			fmt.Sprintf("/%s@%s/%s?tab=doc", sample.ModulePath, "v0.9.0", pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV090, false),
				`This is the documentation HTML`,
			),
		},
		{
			"package@version doc tab nonredistributable",
			// For a non-redistributable package, the doc tab will not show the doc.
			fmt.Sprintf("/%s@%s/%s?tab=doc", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgNonRedist, false),
				`hidden due to license restrictions`,
			),
		},
		{
			"package@version readme tab",
			fmt.Sprintf("/%s@%s/%s?tab=overview", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`<div class="Overview-module">`,
				`Repository: <a href="github.com/valid_module_name" target="_blank">github.com/valid_module_name</a>`,
				`<div class="Overview-readmeContent"><p>readme</p>`,
				`<div class="Overview-readmeSource">Source: github.com/valid_module_name@v1.0.0/README.md</div>`),
		},
		{
			"package@version readme tab nonredistributable",
			// For a non-redistributable package, the readme tab will not show the readme.
			fmt.Sprintf("/%s@%s/%s?tab=overview", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgNonRedist, false),
				`hidden due to license restrictions`,
			),
		},
		{
			"package@version subdirectories tab",
			fmt.Sprintf("/%s@%s/%s?tab=subdirectories", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`foo`,
			),
		},
		{
			"package@version versions tab",
			fmt.Sprintf("/%s@%s/%s?tab=versions", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`Versions`,
				`v1`,
				`<a href="/github.com/valid_module_name@v1.0.0/foo" title="v1.0.0">v1.0.0</a>`,
			),
		},
		{
			"package@version imports tab",
			fmt.Sprintf("/%s@%s/%s?tab=imports", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`Imports`,
				`Standard Library`,
				`<a href="/fmt">fmt</a>`,
				`<a href="/path/to/bar">path/to/bar</a>`),
		},
		{
			"package@version imported by tab",
			fmt.Sprintf("/%s@%s/%s?tab=importedby", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`No known importers for this package`,
			),
		},
		{
			"package@version imported by tab second page",
			fmt.Sprintf("/%s@%s/%s?tab=importedby&page=2", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`No known importers for this package`,
			),
		},
		{
			"package@version licenses tab",
			fmt.Sprintf("/%s@%s/%s?tab=licenses", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(
				pkgHeader(pkgV100, false),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
				`<div class="License-source">Source: github.com/valid_module_name@v1.0.0/LICENSE</div>`,
			),
		},
		{
			"directory subdirectories",
			fmt.Sprintf("/%s", sample.PackagePath+"/directory"),
			http.StatusOK,
			append(
				pkgHeader(dir, true),
				`<th>Path</th>`,
				`<th>Synopsis</th>`,
			),
		},
		{
			"directory overview",
			fmt.Sprintf("/%s?tab=overview", sample.PackagePath+"/directory"),
			http.StatusOK,
			append(
				pkgHeader(dir, true),
				`<div class="Overview-module">`,
				`Repository: <a href="github.com/valid_module_name" target="_blank">github.com/valid_module_name</a>`,
				`<div class="Overview-readmeContent"><p>readme</p>`,
				`<div class="Overview-readmeSource">Source: github.com/valid_module_name@v1.0.0/README.md</div>`),
		},
		{
			"directory licenses",
			fmt.Sprintf("/%s?tab=licenses", sample.PackagePath+"/directory"),
			http.StatusOK,
			append(
				pkgHeader(dir, true),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
				`<div class="License-source">Source: github.com/valid_module_name@v1.0.0/LICENSE</div>`),
		},
		{
			"stdlib directory default",
			fmt.Sprintf("/cmd"),
			http.StatusOK,
			append(
				pkgHeader(dirCmd, true),
				`<th>Path</th>`,
				`<th>Synopsis</th>`,
			),
		},
		{
			"stdlib directory subdirectories",
			fmt.Sprintf("/cmd@go1.13?tab=subdirectories"),
			http.StatusOK,
			append(
				pkgHeader(dirCmd, false),
				`<th>Path</th>`,
				`<th>Synopsis</th>`,
			),
		},
		{
			"stdlib directory overview",
			fmt.Sprintf("/cmd@go1.13?tab=overview"),
			http.StatusOK,
			append(
				pkgHeader(dirCmd, false),
				`<div class="Overview-module">`,
				`Repository: <a href="github.com/valid_module_name" target="_blank">github.com/valid_module_name</a>`,
				`<div class="Overview-readmeContent"><p>readme</p>`,
				`<div class="Overview-readmeSource">Source: go.googlesource.com/go/&#43;/refs/tags/go1.13/README.md</div>`),
		},
		{
			"stdlib directory licenses",
			fmt.Sprintf("/cmd@go1.13?tab=licenses"),
			http.StatusOK,
			append(
				pkgHeader(dirCmd, false),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
				`<div class="License-source">Source: go.googlesource.com/go/&#43;/refs/tags/go1.13/LICENSE</div>`),
		},
		{
			"module default",
			fmt.Sprintf("/mod/%s", sample.ModulePath),
			http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			append(modHeader(mod, true), `readme`),
		},
		{
			"module overview",
			fmt.Sprintf("/mod/%s?tab=overview", sample.ModulePath),
			http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			append(modHeader(mod, true), `readme`),
		},
		// TODO(b/139498072): add a second module, so we can verify that we get the latest version.
		{
			"module packages tab latest version",
			fmt.Sprintf("/mod/%s?tab=packages", sample.ModulePath),
			http.StatusOK,
			// Fall back to the latest version.
			append(modHeader(mod, true), `This is a package synopsis`),
		},
		{
			"module@version readme tab",
			fmt.Sprintf("/mod/%s@%s?tab=overview", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(modHeader(mod, false), `readme`),
		},
		{
			"module@version packages tab",
			fmt.Sprintf("/mod/%s@%s?tab=packages", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(modHeader(mod, false), `This is a package synopsis`),
		},
		{
			"module@version versions tab",
			fmt.Sprintf("/mod/%s@%s?tab=versions", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(
				modHeader(mod, false),
				`Versions`,
				`v1`,
				`<a href="/mod/github.com/valid_module_name@v1.0.0" title="v1.0.0">v1.0.0</a>`,
			),
		},
		{
			"module@version licenses tab",
			fmt.Sprintf("/mod/%s@%s?tab=licenses", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(
				modHeader(mod, false),
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
			),
		},
		{
			"cmd go package page",
			"/cmd/go",
			http.StatusOK,
			pkgHeader(cmdGo, true),
		},
		{
			"cmd go package page at version",
			"/cmd/go@go1.13",
			http.StatusOK,
			pkgHeader(cmdGo, false),
		},
		{
			"standard library module page",
			"/std",
			http.StatusOK,
			modHeader(std, true),
		},
		{
			"standard library module page at version",
			"/std@go1.13",
			http.StatusOK,
			modHeader(std, false),
		},
		{
			"latest version for the standard library",
			"/latest-version/std",
			http.StatusOK,
			[]string{`"go1.13"`},
		},
		{
			"latest version for module",
			"/latest-version/" + sample.ModulePath,
			http.StatusOK,
			[]string{`"v1.0.0"`},
		},
		{
			"latest version for package",
			fmt.Sprintf("/latest-version/%s?pkg=%s", sample.ModulePath, pkg2.Path),
			http.StatusOK,
			[]string{`"v1.0.0"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) { // remove initial '/' for name
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
