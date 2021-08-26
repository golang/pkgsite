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
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
)

func Test(t *testing.T) {
	repoPath := func(fn string) string { return filepath.Join("..", "..", fn) }
	localModule := repoPath("internal/fetch/testdata/has_go_mod")
	flag.Set("static", repoPath("static"))
	testModules := proxytest.LoadTestModules(repoPath("internal/proxy/testdata"))
	prox, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	server, err := newServer(context.Background(), []string{localModule}, false, prox)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Install(mux.Handle, nil, nil)
	w := httptest.NewRecorder()

	for _, url := range []string{"/example.com/testmod", "/example.com/single/pkg"} {
		mux.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%q: got status code = %d, want %d", url, w.Code, http.StatusOK)
		}
	}
}

func TestCollectPaths(t *testing.T) {
	got := collectPaths([]string{"a", "b,c2,d3", "e4", "f,g"})
	want := []string{"a", "b", "c2", "d3", "e4", "f", "g"}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
