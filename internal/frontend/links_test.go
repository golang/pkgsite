// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"golang.org/x/pkgsite/internal"
)

func expectedCodeWikiURL(baseURL, path string) string {
	return fmt.Sprintf("%s/%s?utm_source=first_party_link&utm_medium=go_pkg_web&utm_campaign=%s", baseURL, path, path)
}

func TestCodeWikiURLGenerator(t *testing.T) {
	// The log package is periodically used to log warnings on a
	// separate goroutine, which can pollute test output.
	// For this test, we can discard all of that output.
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/_/exists/github.com/owner/repo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/_/exists/github.com/golang/glog", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	oldCodeWikiURLBase := codeWikiURLBase
	oldCodeWikiExistsURL := codeWikiExistsURL
	codeWikiURLBase = server.URL + "/"
	codeWikiExistsURL = server.URL + "/_/exists/"
	t.Cleanup(func() {
		codeWikiURLBase = oldCodeWikiURLBase
		codeWikiExistsURL = oldCodeWikiExistsURL
	})

	testCases := []struct {
		name, modulePath, path string
		want                   string
	}{
		{
			name:       "github repo",
			modulePath: "github.com/owner/repo",
			want:       expectedCodeWikiURL(server.URL, "github.com/owner/repo"),
		},
		{
			name:       "github repo subpackage",
			modulePath: "github.com/owner/repo",
			want:       expectedCodeWikiURL(server.URL, "github.com/owner/repo"),
		},
		{
			name:       "github repo not found",
			modulePath: "github.com/owner/repo-not-found",
			want:       "",
		},
		{
			name:       "non-github repo",
			modulePath: "example.com/owner/repo",
			want:       "",
		},
		{
			name:       "golang.org/x/ repo",
			modulePath: "golang.org/x/glog",
			want:       expectedCodeWikiURL(server.URL, "github.com/golang/glog"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			um := &internal.UnitMeta{ModuleInfo: internal.ModuleInfo{ModulePath: tc.modulePath}}
			url := codeWikiURLGenerator(context.Background(), server.Client(), um, false)()
			if url != tc.want {
				t.Errorf("codeWikiURLGenerator(ctx, client, %q) = %q, want %q, got %q", tc.path, url, tc.want, url)
			}
		})
	}
}
