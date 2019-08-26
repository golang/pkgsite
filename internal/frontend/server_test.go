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
	sampleVersion := sample.Version()
	if err := testDB.InsertVersion(ctx, sampleVersion); err != nil {
		t.Fatal(err)
	}
	nonRedistModulePath := "github.com/non_redistributable"
	nonRedistPkgPath := nonRedistModulePath + "/bar"
	nonRedistVersion := sample.Version()
	nonRedistVersion.ModulePath = nonRedistModulePath
	nonRedistVersion.Packages = []*internal.Package{
		{
			Name:   "bar",
			Path:   nonRedistPkgPath,
			V1Path: nonRedistPkgPath,
		},
	}
	nonRedistVersion.RepositoryURL = nonRedistModulePath

	if err := testDB.InsertVersion(ctx, nonRedistVersion); err != nil {
		t.Fatal(err)
	}

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
		`<h1 class="Header-title">github.com/valid_module_name/foo</h1>`,
		`Version:`,
		`v1.0.0`,
		`<a href="/pkg/github.com/valid_module_name/foo@v1.0.0?tab=licenses#LICENSE">MIT</a>`,
		`<a href="github.com/valid_module_name">Source Code</a>`,
	}
	nonRedistPkgHeader := []string{
		`<h1 class="Header-title">github.com/non_redistributable/bar</h1>`,
		`Version:`,
		`v1.0.0`,
		`No license files detected`,
		`not legal advice`,
		`<a href="github.com/non_redistributable">Source Code</a>`,
	}

	modHeader := []string{
		`<h1 class="Header-title">module github.com/valid_module_name</h1>`,
		`Version:`,
		`v1.0.0`,
		`<a href="/mod/github.com/valid_module_name@v1.0.0?tab=licenses#LICENSE">MIT</a>`,
		`<a href="github.com/valid_module_name">Source Code</a>`,
	}

	for _, tc := range []struct {
		// path to use in an HTTP GET request
		urlPath string
		// substrings we expect to see in the body
		want []string
	}{
		{
			"/static/",
			[]string{"css", "html", "img", "js"},
		},
		{
			"/license-policy",
			[]string{
				"The Go website displays license information",
				"this is not legal advice",
			},
		},
		{"/favicon.ico", nil}, // just check that it returns 200
		{
			fmt.Sprintf("/search?q=%s", sample.PackageName),
			[]string{
				`<a href="/pkg/github.com/valid_module_name/foo?q=foo">github.com/valid_module_name/foo</a>`,
			},
		},
		{
			fmt.Sprintf("/pkg/%s", sample.PackagePath),
			append(pkgHeader, `This is the documentation HTML`),
		},
		{
			fmt.Sprintf("/pkg/%s@%s", sample.PackagePath, sample.VersionString),
			append(pkgHeader, `This is the documentation HTML`),
		},
		{
			// For a non-redistributable package, the "latest" route goes to the modules tab.
			fmt.Sprintf("/pkg/%s", nonRedistPkgPath),
			append(nonRedistPkgHeader, `Packages in github.com/non_redistributable`),
		},
		{
			// For a non-redistributable package, the name@version route goes to the modules tab.
			fmt.Sprintf("/pkg/%s@%s", nonRedistPkgPath, sample.VersionString),
			append(nonRedistPkgHeader, `Packages in github.com/non_redistributable`),
		},
		{
			fmt.Sprintf("/pkg/%s?tab=doc", sample.PackagePath),
			append(pkgHeader, `This is the documentation HTML`),
		},
		{
			// For a non-redistributable package, the doc tab will not show the doc.
			fmt.Sprintf("/pkg/%s?tab=doc", nonRedistPkgPath),
			append(nonRedistPkgHeader, `hidden due to license restrictions`),
		},
		{
			fmt.Sprintf("/pkg/%s?tab=readme", sample.PackagePath),
			append(pkgHeader, `<div class="ReadMe"><p>readme</p>`),
		},
		{
			// For a non-redistributable package, the readme tab will not show the readme.
			fmt.Sprintf("/pkg/%s?tab=readme", nonRedistPkgPath),
			append(nonRedistPkgHeader, `hidden due to license restrictions`),
		},

		{
			fmt.Sprintf("/pkg/%s?tab=module", sample.PackagePath),
			append(pkgHeader,
				`foo`,
				`This is a package synopsis`),
		},
		{
			fmt.Sprintf("/pkg/%s?tab=versions", sample.PackagePath),
			append(pkgHeader,
				`Versions`,
				`v1`,
				`<a href="/pkg/github.com/valid_module_name/foo@v1.0.0" title="v1.0.0">v1.0.0</a>`),
		},
		{
			fmt.Sprintf("/pkg/%s?tab=imports", sample.PackagePath),
			append(pkgHeader,
				`Imports`,
				`Standard Library`,
				`<a href="/pkg/fmt">fmt</a>`,
				`<a href="/pkg/path/to/bar">path/to/bar</a>`),
		},
		{
			fmt.Sprintf("/pkg/%s?tab=importedby", sample.PackagePath),
			append(pkgHeader,
				`No known importers for this package`),
		},
		{
			fmt.Sprintf("/pkg/%s?tab=importedby&page=2", sample.PackagePath),
			append(pkgHeader,
				`No known importers for this package`),
		},
		{
			fmt.Sprintf("/pkg/%s?tab=licenses", sample.PackagePath),
			append(pkgHeader,
				`<a href="#LICENSE">MIT</a>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`),
		},

		{
			fmt.Sprintf("/mod/%s@%s", sample.ModulePath, sample.VersionString),
			// Show the readme tab by default.
			append(modHeader, `readme`),
		},
		{
			fmt.Sprintf("/mod/%s", sample.ModulePath),
			// Fall back to the latest version, show readme tab by default.
			append(modHeader, `readme`),
		},
		// TODO(b/139498072): add a second module, so we can verify that we get the latest version.
		{
			fmt.Sprintf("/mod/%s?tab=packages", sample.ModulePath),
			// Fall back to the latest version.
			append(modHeader,
				`Packages in github.com/valid_module_name`,
				`<a href="/pkg/github.com/valid_module_name/foo@v1.0.0">`,
				`foo`,
				`This is a package synopsis`),
		},
		{
			fmt.Sprintf("/mod/%s@%s?tab=readme", sample.ModulePath, sample.VersionString),
			append(modHeader, `readme`),
		},
		{
			fmt.Sprintf("/mod/%s@%s?tab=packages", sample.ModulePath, sample.VersionString),
			append(modHeader,
				`Packages in github.com/valid_module_name`,
				`<a href="/pkg/github.com/valid_module_name/foo@v1.0.0">`,
				`foo`,
				`This is a package synopsis`),
		},
		{
			fmt.Sprintf("/mod/%s@%s?tab=versions", sample.ModulePath, sample.VersionString),
			append(modHeader,
				`Versions`,
				`v1`,
				`<a href="/mod/github.com/valid_module_name@v1.0.0" title="v1.0.0">v1.0.0</a>`),
		},
		{
			fmt.Sprintf("/mod/%s@%s?tab=licenses", sample.ModulePath, sample.VersionString),
			append(modHeader,
				`<a href="#LICENSE">MIT</a>`,
				`This is not legal advice`,
				`<a href="/license-policy">Read disclaimer.</a>`,
				`Lorem Ipsum`),
		},
	} {
		t.Run(tc.urlPath[1:], func(t *testing.T) { // remove initial '/' for name
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", tc.urlPath, nil))
			res := w.Result()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status code: got = %d, want %d", res.StatusCode, http.StatusOK)
			}
			bytes, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			body := string(bytes)
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					if len(body) > 100 {
						body = body[:100] + "..."
					}
					t.Errorf("`%s` not found in body\n%s", want, body)
				}
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
