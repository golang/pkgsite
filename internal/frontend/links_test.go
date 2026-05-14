// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
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
			url := codeWikiURLGenerator(t.Context(), server.Client(), um, false)()
			if url != tc.want {
				t.Errorf("codeWikiURLGenerator(ctx, client, %q) = %q, want %q", tc.path, url, tc.want)
			}
		})
	}
}

// recordingTransport is an http.RoundTripper that records every outgoing
// request URL and fails the test if invoked.
type recordingTransport struct {
	t        *testing.T
	requests []string
}

func (rt *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.requests = append(rt.requests, r.URL.String())
	rt.t.Errorf("unexpected external request: %s", r.URL)
	return nil, fmt.Errorf("unexpected request: %s", r.URL)
}

func TestExternalLinkGeneratorsSkipsPrivateModules(t *testing.T) {
	for _, tc := range []struct {
		name      string
		goprivate string
		gonoproxy string
	}{
		{name: "GOPRIVATE match", goprivate: "github.com/owner/*"},
		{name: "GONOPROXY match", gonoproxy: "github.com/owner/*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := goPrivatePatterns
			goPrivatePatterns = func() goPrivateConfig {
				return goPrivateConfig{goprivate: tc.goprivate, gonoproxy: tc.gonoproxy}
			}
			t.Cleanup(func() { goPrivatePatterns = old })

			rt := &recordingTransport{t: t}
			client := &http.Client{Transport: rt}

			um := &internal.UnitMeta{ModuleInfo: internal.ModuleInfo{ModulePath: "github.com/owner/private-repo"}}
			depsDev, codeWiki := externalLinkGenerators(t.Context(), client, um, false, false)
			if got := depsDev(); got != "" {
				t.Errorf("depsDev() = %q, want empty for private module", got)
			}
			if got := codeWiki(); got != "" {
				t.Errorf("codeWiki() = %q, want empty for private module", got)
			}
		})
	}
}

func TestExternalLinkGeneratorsCallsForPublicModules(t *testing.T) {
	// Sanity check: when no privacy env var matches, the generators should
	// invoke the HTTP client. We intercept and return 404 to keep the test hermetic.
	mux := http.NewServeMux()
	var depsHits, codeHits int
	mux.HandleFunc("/_/s/go/", func(w http.ResponseWriter, r *http.Request) {
		depsHits++
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/_/exists/", func(w http.ResponseWriter, r *http.Request) {
		codeHits++
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	oldCodeWikiExistsURL := codeWikiExistsURL
	codeWikiExistsURL = server.URL + "/_/exists/"
	t.Cleanup(func() { codeWikiExistsURL = oldCodeWikiExistsURL })

	old := goPrivatePatterns
	goPrivatePatterns = func() goPrivateConfig {
		return goPrivateConfig{goprivate: "internal.example.com"}
	}
	t.Cleanup(func() { goPrivatePatterns = old })

	// HTTP client whose RoundTripper rewrites deps.dev requests to the test server.
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "deps.dev" {
				r.URL.Scheme = "http"
				r.URL.Host = server.Listener.Addr().String()
			}
			return http.DefaultTransport.RoundTrip(r)
		}),
	}

	um := &internal.UnitMeta{ModuleInfo: internal.ModuleInfo{ModulePath: "github.com/public/repo"}}
	depsDev, codeWiki := externalLinkGenerators(t.Context(), client, um, false, false)
	depsDev()
	codeWiki()
	if depsHits == 0 {
		t.Error("expected deps.dev to be contacted for a public module")
	}
	if codeHits == 0 {
		t.Error("expected codewiki.google to be contacted for a public module")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
