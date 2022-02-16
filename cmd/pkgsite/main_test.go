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
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
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

func Test(t *testing.T) {
	repoPath := func(fn string) string { return filepath.Join("..", "..", fn) }

	abs := func(dir string) string {
		a, err := filepath.Abs(dir)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	localModule := repoPath("internal/fetch/testdata/has_go_mod")
	cacheDir := repoPath("internal/fetch/testdata/modcache")
	testModules := proxytest.LoadTestModules(repoPath("internal/proxy/testdata"))
	prox, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	server, err := newServer(context.Background(), []string{localModule}, false, cacheDir, nil, prox)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Install(mux.Handle, nil, nil)

	modcacheChecker := in("",
		in(".Documentation", hasText("var V = 1")),
		sourceLinks(path.Join(abs(cacheDir), "modcache.com@v1.0.0"), "a.go"))

	for _, test := range []struct {
		name string
		url  string
		want htmlcheck.Checker
	}{
		{
			"local",
			"example.com/testmod",
			in("",
				in(".Documentation", hasText("There is no documentation for this package.")),
				sourceLinks(path.Join(abs(localModule), "example.com/testmod"), "a.go")),
		},
		{
			"modcache",
			"modcache.com@v1.0.0",
			modcacheChecker,
		},
		{
			"modcache latest",
			"modcache.com",
			modcacheChecker,
		},
		{
			"proxy",
			"example.com/single/pkg",
			hasText("G is new in v1.1.0"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
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

func TestListModsForPaths(t *testing.T) {
	listModules = func(string) ([]listedMod, error) {
		return []listedMod{
			{
				internal.Modver{Path: "m1", Version: "v1.2.3"},
				"/dir/cache/download/m1/@v/v1.2.3.mod",
			},
			{
				internal.Modver{Path: "m2", Version: "v1.0.0"},
				"/repos/m2/go.mod",
			},
		}, nil
	}
	defer func() { listModules = _listModules }()

	gotPaths, gotCacheMods, err := listModsForPaths([]string{"m1"}, "/dir")
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"/repos/m2"}
	wantCacheMods := []internal.Modver{{Path: "m1", Version: "v1.2.3"}}
	if !cmp.Equal(gotPaths, wantPaths) || !cmp.Equal(gotCacheMods, wantCacheMods) {
		t.Errorf("got\n%v, %v\nwant\n%v, %v", gotPaths, gotCacheMods, wantPaths, wantCacheMods)
	}
}
