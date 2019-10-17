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

	cmdGo := sample.Package()
	cmdGo.Name = "main"
	cmdGo.Path = "cmd/go"
	mustInsertVersion(stdlib.ModulePath, "v1.13.0", []*internal.Package{cmdGo})

	s, err := NewServer(testDB, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle, nil)

	type header struct {
		// suffix is not used for the module header.
		// the fields must be exported for use by template.Execute.
		Version, Title, Suffix, ModuleURL, LicenseInfo, LatestURL string
		notLatest                                                 bool
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
	constructPackageHeader := func(h *header) []string {
		latestInfo := `<div class="DetailsHeader-badge DetailsHeader-latest">Latest</div>`
		if h.notLatest {
			latestInfo = mustExecuteTemplate(h, `<a href="{{.LatestURL}}">Go to latest</a>`)
		}
		return []string{
			mustExecuteTemplate(h, `<span class="DetailsHeader-breadcrumbDivider">/</span>`),
			mustExecuteTemplate(h, `<span class="DetailsHeader-breadcrumbCurrent">{{.Suffix}}</span>`),
			mustExecuteTemplate(h, `<h1 class="DetailsHeader-title">{{.Title}}</h1>`),
			mustExecuteTemplate(h, `<div class="DetailsHeader-version">{{.Version}}</div>`),
			latestInfo,
			h.LicenseInfo,
			h.ModuleURL,
		}
	}
	constructModuleHeader := func(h *header) []string {
		latestInfo := `<div class="DetailsHeader-badge DetailsHeader-latest">Latest</div>`
		if h.notLatest {
			latestInfo = mustExecuteTemplate(h, `<a href="{{.LatestURL}}">Go to latest</a>`)
		}
		return []string{
			mustExecuteTemplate(h, `<h1 class="DetailsHeader-title">{{.Title}}</h1>`),
			mustExecuteTemplate(h, `<div class="DetailsHeader-version">{{.Version}}</div>`),
			latestInfo,
			h.LicenseInfo,
		}
	}

	pkgHeaderLatest := constructPackageHeader(&header{
		Version:     "v1.0.0",
		Suffix:      "foo",
		Title:       "Package foo",
		LicenseInfo: `<a href="/github.com/valid_module_name@v1.0.0/foo?tab=licenses#LICENSE">MIT</a>`,
	})
	pkgHeaderNotLatest := constructPackageHeader(&header{
		Version:     "v0.9.0",
		Suffix:      "foo",
		Title:       "Package foo",
		LicenseInfo: `<a href="/github.com/valid_module_name@v0.9.0/foo?tab=licenses#LICENSE">MIT</a>`,
		LatestURL:   "/github.com/valid_module_name/foo",
		notLatest:   true,
	})
	nonRedistPkgHeader := constructPackageHeader(&header{
		Version:     "v1.0.0",
		Suffix:      "bar",
		Title:       "Package bar",
		LicenseInfo: `None detected`,
	})
	cmdGoHeader := constructPackageHeader(&header{
		Suffix:      "go",
		Version:     "go1.13",
		Title:       "Command go",
		LicenseInfo: `<a href="/cmd/go@go1.13?tab=licenses#LICENSE">MIT</a>`,
	})
	modHeader := constructModuleHeader(&header{
		Version:     "v1.0.0",
		Title:       "Module github.com/valid_module_name",
		LicenseInfo: `<a href="/mod/github.com/valid_module_name@v1.0.0?tab=licenses#LICENSE">MIT</a>`,
	})
	stdHeader := constructModuleHeader(&header{
		Version:     "go1.13",
		Title:       "Standard library",
		LicenseInfo: `<a href="/std@go1.13?tab=licenses#LICENSE">MIT</a>`,
	})

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
			append(pkgHeaderLatest, `This is the documentation HTML`),
		},
		{
			"package default nonredistributable",
			// For a non-redistributable package, the "latest" route goes to the modules tab.
			fmt.Sprintf("/%s", nonRedistPkgPath),
			http.StatusOK,
			nonRedistPkgHeader,
		},
		{
			"package@version default",
			fmt.Sprintf("/%s@%s/%s", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest, `This is the documentation HTML`),
		},
		{
			"package@version default specific version nonredistributable",
			// For a non-redistributable package, the name@version route goes to the modules tab.
			fmt.Sprintf("/%s@%s/%s", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			http.StatusOK,
			nonRedistPkgHeader,
		},
		{
			"package@version doc tab",
			fmt.Sprintf("/%s@%s/%s?tab=doc", sample.ModulePath, "v0.9.0", pkgSuffix),
			http.StatusOK,
			append(pkgHeaderNotLatest, `This is the documentation HTML`),
		},
		{
			"package@version doc tab nonredistributable",
			// For a non-redistributable package, the doc tab will not show the doc.
			fmt.Sprintf("/%s@%s/%s?tab=doc", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			http.StatusOK,
			append(nonRedistPkgHeader, `hidden due to license restrictions`),
		},
		{
			"package@version readme tab",
			fmt.Sprintf("/%s@%s/%s?tab=overview", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest,
				`<div class="Overview-module">`,
				`<b>Packages in this module: </b>`,
				`Repository: <a href="github.com/valid_module_name" target="_blank">github.com/valid_module_name</a>`,
				`<div class="Overview-readmeContent"><p>readme</p>`,
				`<div class="Overview-readmeSource">Source: github.com/valid_module_name@v1.0.0/README.md</div>`),
		},
		{
			"package@version readme tab nonredistributable",
			// For a non-redistributable package, the readme tab will not show the readme.
			fmt.Sprintf("/%s@%s/%s?tab=overview", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			http.StatusOK,
			append(nonRedistPkgHeader, `hidden due to license restrictions`),
		},
		{
			"package@version subdirectories tab",
			fmt.Sprintf("/%s@%s/%s?tab=subdirectories", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest, `foo`),
		},
		{
			"package@version versions tab",
			fmt.Sprintf("/%s@%s/%s?tab=versions", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest,
				`Versions`,
				`v1`,
				`<a href="/github.com/valid_module_name@v1.0.0/foo" title="v1.0.0">v1.0.0</a>`),
		},
		{
			"package@version imports tab",
			fmt.Sprintf("/%s@%s/%s?tab=imports", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest,
				`Imports`,
				`Standard Library`,
				`<a href="/fmt">fmt</a>`,
				`<a href="/path/to/bar">path/to/bar</a>`),
		},
		{
			"package@version imported by tab",
			fmt.Sprintf("/%s@%s/%s?tab=importedby", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest,
				`No known importers for this package`),
		},
		{
			"package@version imported by tab second page",
			fmt.Sprintf("/%s@%s/%s?tab=importedby&page=2", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest,
				`No known importers for this package`),
		},
		{
			"package@version licenses tab",
			fmt.Sprintf("/%s@%s/%s?tab=licenses", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeaderLatest,
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`,
				`<div class="License-source">Source: github.com/valid_module_name@v1.0.0/LICENSE</div>`),
		},
		{
			"directory",
			fmt.Sprintf("/%s", sample.PackagePath+"/directory"),
			http.StatusOK,
			[]string{`<h1 class="DetailsHeader-title">Directories</h1>`},
		},
		{
			"module default",
			fmt.Sprintf("/mod/%s", sample.ModulePath),
			http.StatusOK,
			// Show the readme tab by default.
			// Fall back to the latest version, show readme tab by default.
			append(modHeader, `readme`),
		},
		// TODO(b/139498072): add a second module, so we can verify that we get the latest version.
		{
			"module packages tab latest version",
			fmt.Sprintf("/mod/%s?tab=packages", sample.ModulePath),
			http.StatusOK,
			// Fall back to the latest version.
			append(modHeader, `This is a package synopsis`),
		},
		{
			"module@version readme tab",
			fmt.Sprintf("/mod/%s@%s?tab=overview", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(modHeader, `readme`),
		},
		{
			"module@version packages tab",
			fmt.Sprintf("/mod/%s@%s?tab=packages", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(modHeader, `This is a package synopsis`),
		},
		{
			"module@version versions tab",
			fmt.Sprintf("/mod/%s@%s?tab=versions", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(modHeader,
				`Versions`,
				`v1`,
				`<a href="/mod/github.com/valid_module_name@v1.0.0" title="v1.0.0">v1.0.0</a>`),
		},
		{
			"module@version licenses tab",
			fmt.Sprintf("/mod/%s@%s?tab=licenses", sample.ModulePath, sample.VersionString),
			http.StatusOK,
			append(modHeader,
				`<div id="#LICENSE">MIT</div>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`),
		},
		{
			"cmd go package page",
			"/cmd/go",
			http.StatusOK,
			cmdGoHeader,
		},
		{
			"cmd go package page at version",
			"/cmd/go@go1.13",
			http.StatusOK,
			cmdGoHeader,
		},
		{
			"standard library module page",
			"/std",
			http.StatusOK,
			stdHeader,
		},
		{
			"standard library module page at version",
			"/std@go1.13",
			http.StatusOK,
			stdHeader,
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
					t.Errorf("`%s` not found in body\n%s", want, b)
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
