// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/pkgsite/internal"
)

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

func TestExternalLinkGeneratorSkipsPrivateModules(t *testing.T) {
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
			depsDev := externalLinkGenerator(t.Context(), client, um, false)
			if got := depsDev(); got != "" {
				t.Errorf("depsDev() = %q, want empty for private module", got)
			}
		})
	}
}

func TestExternalLinkGeneratorCallsForPublicModules(t *testing.T) {
	// Sanity check: when no privacy env var matches, the generator should
	// invoke the HTTP client. We intercept and return 404 to keep the test hermetic.
	mux := http.NewServeMux()
	var depsHits int
	mux.HandleFunc("/_/s/go/", func(w http.ResponseWriter, r *http.Request) {
		depsHits++
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

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
	depsDev := externalLinkGenerator(t.Context(), client, um, false)
	depsDev()
	if depsHits == 0 {
		t.Error("expected deps.dev to be contacted for a public module")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
