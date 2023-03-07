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
	"golang.org/x/pkgsite/internal/proxy"
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

func TestBuildGetters(t *testing.T) {
	repoPath := func(fn string) string { return filepath.Join("..", "..", fn) }

	abs := func(dir string) string {
		a, err := filepath.Abs(dir)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	ctx := context.Background()
	localModule := repoPath("internal/fetch/testdata/has_go_mod")
	cacheDir := repoPath("internal/fetch/testdata/modcache")
	testModules := proxytest.LoadTestModules(repoPath("internal/proxy/testdata"))
	prox, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	localGetter := "Dir(" + abs(localModule) + ")"
	cacheGetter := "FSProxy(" + abs(cacheDir) + ")"
	for _, test := range []struct {
		name     string
		dirs     []string
		cacheDir string
		proxy    *proxy.Client
		want     []string
	}{
		{
			name: "local only",
			dirs: []string{localModule},
			want: []string{localGetter},
		},
		{
			name:     "cache",
			cacheDir: cacheDir,
			want:     []string{cacheGetter},
		},
		{
			name:  "proxy",
			proxy: prox,
			want:  []string{"Proxy"},
		},
		{
			name:     "all three",
			dirs:     []string{localModule},
			cacheDir: cacheDir,
			proxy:    prox,
			want:     []string{localGetter, cacheGetter, "Proxy"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			getters, err := buildGetters(ctx, getterConfig{
				dirs:        test.dirs,
				pattern:     "./...",
				modCacheDir: test.cacheDir,
				proxy:       test.proxy,
			})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, g := range getters {
				got = append(got, g.String())
			}

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

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

	defaultConfig := serverConfig{
		paths:         []string{localModule},
		gopathMode:    false,
		useListedMods: true,
		useCache:      true,
		cacheDir:      cacheDir,
		proxy:         prox,
	}

	modcacheChecker := in("",
		in(".Documentation", hasText("var V = 1")),
		sourceLinks(path.Join(abs(cacheDir), "modcache.com@v1.0.0"), "a.go"))

	ctx := context.Background()
	for _, test := range []struct {
		name string
		cfg  serverConfig
		url  string
		want htmlcheck.Checker
	}{
		{
			"local",
			defaultConfig,
			"example.com/testmod",
			in("",
				in(".Documentation", hasText("There is no documentation for this package.")),
				sourceLinks(path.Join(abs(localModule), "example.com/testmod"), "a.go")),
		},
		{
			"modcache",
			defaultConfig,
			"modcache.com@v1.0.0",
			modcacheChecker,
		},
		{
			"modcache latest",
			defaultConfig,
			"modcache.com",
			modcacheChecker,
		},
		{
			"proxy",
			defaultConfig,
			"example.com/single/pkg",
			hasText("G is new in v1.1.0"),
		},
		{
			"search",
			defaultConfig,
			"search?q=a",
			in(".SearchResults",
				hasText("example.com/testmod"),
			),
		},
		{
			"search",
			defaultConfig,
			"search?q=zzz",
			in(".SearchResults",
				hasText("no matches"),
			),
		},
		// TODO(rfindley): add more tests, including a test for the standard
		// library once it doesn't go through the stdlib package.
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
			if w.Code != http.StatusOK {
				t.Fatalf("got status code = %d, want %d", w.Code, http.StatusOK)
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
