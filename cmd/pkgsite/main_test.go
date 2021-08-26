// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
)

func Test(t *testing.T) {
	repoPath := func(fn string) string { return filepath.Join("..", "..", fn) }

	localModule := repoPath("internal/fetch/testdata/has_go_mod")
	cacheDir := repoPath("internal/fetch/testdata/modcache")
	flag.Set("static", repoPath("static"))
	testModules := proxytest.LoadTestModules(repoPath("internal/proxy/testdata"))
	prox, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	server, err := newServer(context.Background(), []string{localModule}, false, cacheDir, prox)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Install(mux.Handle, nil, nil)

	for _, test := range []struct {
		name       string
		url        string
		wantInBody string
	}{
		{"local", "example.com/testmod", "There is no documentation for this package."},
		{"modcache", "modcache.com@v1.0.0", "var V = 1"},
		{"proxy", "example.com/single/pkg", "G is new in v1.1.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/"+test.url, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("got status code = %d, want %d", w.Code, http.StatusOK)
			}
			body := w.Body.String()
			if !strings.Contains(body, test.wantInBody) {
				t.Fatalf("body is missing %q\n%s", test.wantInBody, body)
			}
		})
	}
}

func TestCollectPaths(t *testing.T) {
	got := collectPaths([]string{"a", "b,c2,d3", "e4", "f,g"})
	want := []string{"a", "b", "c2", "d3", "e4", "f", "g"}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
