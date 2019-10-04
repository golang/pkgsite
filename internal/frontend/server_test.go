// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
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
	s.Install(mux.Handle)

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

	mustInsertVersion := func(modulePath string, pkgs []*internal.Package) {
		v := sample.Version()
		v.ModulePath = modulePath
		v.RepositoryURL = modulePath
		if modulePath == stdlib.ModulePath {
			v.RepositoryURL = stdlib.GoSourceRepoURL
		}
		v.Packages = pkgs
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	pkg := sample.Package()
	pkg2 := sample.Package()
	pkg2.Path = sample.ModulePath + "/foo/directory/hello"
	mustInsertVersion(sample.ModulePath, []*internal.Package{pkg, pkg2})

	nonRedistModulePath := "github.com/non_redistributable"
	nonRedistPkgPath := nonRedistModulePath + "/bar"
	mustInsertVersion(nonRedistModulePath, []*internal.Package{{
		Name:   "bar",
		Path:   nonRedistPkgPath,
		V1Path: nonRedistPkgPath,
	}})

	cmdGo := sample.Package()
	cmdGo.Name = "main"
	cmdGo.Path = "cmd/go"
	mustInsertVersion(stdlib.ModulePath, []*internal.Package{cmdGo})

	if err := testDB.RefreshSearchDocuments(ctx); err != nil {
		t.Fatal(err)
	}

	s, err := NewServer(testDB, "../../content/static", false)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	s.Install(mux.Handle)

	pkgHeader := []string{
		// part of breadcrumb path
		`<span class="DetailsHeader-breadcrumbDivider">/</span><span class="DetailsHeader-breadcrumbCurrent">foo</span>`,
		`<h1 class="DetailsHeader-title">Package foo</h1>`,
		`Module:`,
		`<a href="/mod/github.com/valid_module_name@v1.0.0">`,
		`github.com/valid_module_name`,
		`</a>`,
		`Version:`,
		`v1.0.0`,
		`<a href="/github.com/valid_module_name@v1.0.0/foo?tab=licenses#LICENSE">MIT</a>`,
		`<a href="github.com/valid_module_name" target="_blank">Source Code</a>`,
	}
	nonRedistPkgHeader := []string{
		`<h1 class="DetailsHeader-title">Package bar</h1>`,
		`Module:`,
		`<a href="/mod/github.com/non_redistributable@v1.0.0">`,
		`github.com/non_redistributable`,
		`</a>`,
		`Version:`,
		`v1.0.0`,
		`None detected`,
		`not legal advice`,
		`<a href="github.com/non_redistributable" target="_blank">Source Code</a>`,
	}

	modHeader := []string{
		`<h1 class="DetailsHeader-title">Module github.com/valid_module_name</h1>`,
		`Version:`,
		`v1.0.0`,
		`<a href="/mod/github.com/valid_module_name@v1.0.0?tab=licenses#LICENSE">MIT</a>`,
		`<a href="github.com/valid_module_name" target="_blank">Source Code</a>`,
	}
	cmdGoHeader := []string{
		`<span class="DetailsHeader-breadcrumbDivider">/</span><span class="DetailsHeader-breadcrumbCurrent">go</span>`,
		`<h1 class="DetailsHeader-title">Command go</h1>`,
		`Module:`,
		`<a href="/std@v1.0.0">`,
		`Standard library`,
		`</a>`,
		`Version:`,
		`v1.0.0`,
		`<a href="/cmd/go@v1.0.0?tab=licenses#LICENSE">MIT</a>`,
		`<a href="https://github.com/golang/go" target="_blank">Source Code</a>`,
	}
	stdHeader := []string{
		`<h1 class="DetailsHeader-title">Standard library</h1>`,
		`Version:`,
		`v1.0.0`,
		`<a href="/std@v1.0.0?tab=licenses#LICENSE">MIT</a>`,
		`<a href="https://github.com/golang/go" target="_blank">Source Code</a>`,
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
			append(pkgHeader, `This is the documentation HTML`),
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
			append(pkgHeader, `This is the documentation HTML`),
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
			fmt.Sprintf("/%s@%s/%s?tab=doc", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader, `This is the documentation HTML`),
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
			fmt.Sprintf("/%s@%s/%s?tab=readme", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader, `<div class="ReadMe"><p>readme</p>`,
				`<div class="ReadMe-source">Source: github.com/valid_module_name@v1.0.0/README.md</div>`),
		},
		{
			"package@version readme tab nonredistributable",
			// For a non-redistributable package, the readme tab will not show the readme.
			fmt.Sprintf("/%s@%s/%s?tab=readme", nonRedistModulePath, sample.VersionString, nonRedistPkgSuffix),
			http.StatusOK,
			append(nonRedistPkgHeader, `hidden due to license restrictions`),
		},
		{
			"package@version subdirectories tab",
			fmt.Sprintf("/%s@%s/%s?tab=subdirectories", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader, `foo`),
		},
		{
			"package@version versions tab",
			fmt.Sprintf("/%s@%s/%s?tab=versions", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader,
				`Versions`,
				`v1`,
				`<a href="/github.com/valid_module_name@v1.0.0/foo" title="v1.0.0">v1.0.0</a>`),
		},
		{
			"package@version imports tab",
			fmt.Sprintf("/%s@%s/%s?tab=imports", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader,
				`Imports`,
				`Standard Library`,
				`<a href="/fmt">fmt</a>`,
				`<a href="/path/to/bar">path/to/bar</a>`),
		},
		{
			"package@version imported by tab",
			fmt.Sprintf("/%s@%s/%s?tab=importedby", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader,
				`No known importers for this package`),
		},
		{
			"package@version imported by tab second page",
			fmt.Sprintf("/%s@%s/%s?tab=importedby&page=2", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader,
				`No known importers for this package`),
		},
		{
			"package@version licenses tab",
			fmt.Sprintf("/%s@%s/%s?tab=licenses", sample.ModulePath, sample.VersionString, pkgSuffix),
			http.StatusOK,
			append(pkgHeader,
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
			fmt.Sprintf("/mod/%s@%s?tab=readme", sample.ModulePath, sample.VersionString),
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
			"standard library module page",
			"/std",
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
	s.Install(mux.Handle)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/invalid-page", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status code: got = %d, want %d", w.Code, http.StatusNotFound)
	}
}
