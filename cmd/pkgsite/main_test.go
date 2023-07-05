// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/net/html"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var (
	in      = htmlcheck.In
	hasText = htmlcheck.HasText
	attr    = htmlcheck.HasAttr

	// href checks for an exact match in an href attribute.
	href = func(val string) htmlcheck.Checker {
		return attr("href", "^"+regexp.QuoteMeta(val)+"$")
	}
)

func TestServer(t *testing.T) {
	repoPath := func(fn string) string { return filepath.Join("..", "..", fn) }

	abs := func(dir string) string {
		a, err := filepath.Abs(dir)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	localModule, _ := testhelper.WriteTxtarToTempDir(t, `
-- go.mod --
module example.com/testmod
-- a.go --
package a
`)
	cacheDir := repoPath("internal/fetch/testdata/modcache")
	testModules := proxytest.LoadTestModules(repoPath("internal/proxy/testdata"))
	prox, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	cfg := func(modifyDefault func(*serverConfig)) serverConfig {
		c := serverConfig{
			paths:          []string{localModule},
			gopathMode:     false,
			useListedMods:  true,
			useLocalStdlib: true,
			useCache:       true,
			cacheDir:       cacheDir,
			proxy:          prox,
		}
		if modifyDefault != nil {
			modifyDefault(&c)
		}
		return c
	}

	modcacheChecker := in("",
		in(".Documentation", hasText("var V = 1")),
		sourceLinks(path.Join(filepath.ToSlash(abs(cacheDir)), "modcache.com@v1.0.0"), "a.go"))

	ctx := context.Background()
	for _, test := range []struct {
		name     string
		cfg      serverConfig
		url      string
		wantCode int
		want     htmlcheck.Checker
	}{
		{
			"local",
			cfg(nil),
			"example.com/testmod",
			http.StatusOK,
			in("",
				in(".Documentation", hasText("There is no documentation for this package.")),
				sourceLinks(path.Join(filepath.ToSlash(abs(localModule)), "example.com/testmod"), "a.go")),
		},
		{
			"modcache",
			cfg(nil),
			"modcache.com@v1.0.0",
			http.StatusOK,
			modcacheChecker,
		},
		{
			"modcache latest",
			cfg(nil),
			"modcache.com",
			http.StatusOK,
			modcacheChecker,
		},
		{
			"modcache unsupported",
			cfg(func(c *serverConfig) {
				c.useCache = false
			}),
			"modcache.com",
			http.StatusFailedDependency, // TODO(rfindley): should this be 404?
			hasText("page is not supported"),
		},
		{
			"proxy",
			cfg(nil),
			"example.com/single/pkg",
			http.StatusOK,
			hasText("G is new in v1.1.0"),
		},
		{
			"proxy unsupported",
			cfg(func(c *serverConfig) {
				c.proxy = nil
			}),
			"example.com/single/pkg",
			http.StatusFailedDependency, // TODO(rfindley): should this be 404?
			hasText("page is not supported"),
		},
		{
			"search",
			cfg(func(c *serverConfig) {
				c.useLocalStdlib = false
			}),
			"search?q=a",
			http.StatusOK,
			in(".SearchResults",
				hasText("example.com/testmod"),
			),
		},
		{
			"no symbol search",
			cfg(func(c *serverConfig) {
				c.useLocalStdlib = false
			}),
			"search?q=A", // using a capital letter should not cause symbol search
			http.StatusOK,
			in(".SearchResults",
				hasText("example.com/testmod"),
			),
		},
		{
			"search not found",
			cfg(func(c *serverConfig) {
				c.useLocalStdlib = false
			}),
			"search?q=zzz",
			http.StatusOK,
			in(".SearchResults",
				hasText("no matches"),
			),
		},
		{
			"search vulns not found",
			cfg(nil),
			"search?q=GO-1234-1234",
			http.StatusOK,
			in(".SearchResults",
				hasText("no matches"),
			),
		},
		{
			"search unsupported",
			cfg(func(c *serverConfig) {
				c.paths = nil
				c.useLocalStdlib = false
			}),
			"search?q=zzz",
			http.StatusFailedDependency,
			hasText("page is not supported"),
		},
		{
			"vulns unsupported",
			cfg(nil),
			"vuln/",
			http.StatusFailedDependency,
			hasText("page is not supported"),
		},
		// TODO(rfindley): add a test for the standard library once it doesn't go
		// through the stdlib package.
		// See also golang/go#58923.
	} {
		t.Run(test.name, func(t *testing.T) {
			server, err := buildServer(ctx, test.cfg)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			server.Install(mux.Handle, nil, nil)

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/"+test.url, nil))
			if w.Code != test.wantCode {
				t.Fatalf("got status code = %d, want %d", w.Code, test.wantCode)
			}
			doc, err := html.Parse(w.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.want(doc); err != nil {
				if testing.Verbose() {
					html.Render(os.Stdout, doc)
				}
				t.Error(err)
			}
		})
	}
}

func sourceLinks(dir, filename string) htmlcheck.Checker {
	filesPath := path.Join("/files", dir) + "/"
	return in("",
		in(".UnitMeta-repo a", href(filesPath)),
		in(".UnitFiles-titleLink a", href(filesPath)),
		in(".UnitFiles-fileList a", href(filesPath+filename)))
}

func TestCollectPaths(t *testing.T) {
	got := collectPaths([]string{"a", "b,c2,d3", "e4", "f,g"})
	want := []string{"a", "b", "c2", "d3", "e4", "f", "g"}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
