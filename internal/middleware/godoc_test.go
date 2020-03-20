// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGodocURL(t *testing.T) {
	mw := GodocURL()
	mwh := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GodocURLFromContext(r.Context()); u != nil {
			// Set in response header since we can’t access the request context.
			w.Header().Set("x-test-godoc-url", u.String())
		}
		w.WriteHeader(http.StatusOK)
	}))

	testCases := []struct {
		desc string

		// Request values
		path    string
		cookies map[string]string

		// Response values
		code    int
		headers map[string]string
	}{
		{
			desc: "Unaffected request",
			path: "/cloud.google.com/go/storage",
			code: http.StatusOK,
		},
		{
			desc: "Strip utm_source, set temporary cookie, and redirect",
			path: "/cloud.google.com/go/storage?tab=doc&utm_source=godoc",
			code: http.StatusSeeOther,
			headers: map[string]string{
				"Location":   "/cloud.google.com/go/storage?tab=doc",
				"Set-Cookie": "tmp-from-godoc=1; SameSite=Strict",
			},
		},
		{
			desc: "Delete temporary cookie; godoc URL should be set",
			path: "/cloud.google.com/go/storage?tab=doc",
			cookies: map[string]string{
				"tmp-from-godoc": "1",
			},
			code: http.StatusOK,
			headers: map[string]string{
				"Set-Cookie":       "tmp-from-godoc=; Max-Age=0",
				"X-Test-Godoc-Url": "https://godoc.org/cloud.google.com/go/storage?utm_source=backtogodoc",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			for k, v := range tc.cookies {
				req.AddCookie(&http.Cookie{
					Name:  k,
					Value: v,
				})
			}
			w := httptest.NewRecorder()
			mwh.ServeHTTP(w, req)
			resp := w.Result()
			if got, want := resp.StatusCode, tc.code; got != want {
				t.Errorf("Status code = %d; want %d", got, want)
			}
			for k, v := range tc.headers {
				if _, ok := resp.Header[k]; !ok {
					t.Errorf("%q not present in response headers", k)
					continue
				}
				if got, want := resp.Header.Get(k), v; got != want {
					t.Errorf("Response header mismatch for %q: got %q; want %q", k, got, want)
				}
			}
		})
	}
}

func TestGodoc(t *testing.T) {
	testCases := []struct {
		from, to string
	}{
		{
			from: "https://pkg.go.dev/cloud.google.com/go/storage?tab=doc",
			to:   "https://godoc.org/cloud.google.com/go/storage?utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/cloud.google.com/go/storage?tab=overview",
			to:   "https://godoc.org/cloud.google.com/go/storage?utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/cloud.google.com/go/storage?tab=versions",
			to:   "https://godoc.org/cloud.google.com/go/storage?utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/cloud.google.com/go/storage?tab=licenses",
			to:   "https://godoc.org/cloud.google.com/go/storage?utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/cloud.google.com/go/storage?tab=subdirectories",
			to:   "https://godoc.org/cloud.google.com/go/storage?utm_source=backtogodoc#pkg-subdirectories",
		},
		{
			from: "https://pkg.go.dev/cloud.google.com/go/storage?tab=imports",
			to:   "https://godoc.org/cloud.google.com/go/storage?imports=&utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/cloud.google.com/go/storage?tab=importedby",
			to:   "https://godoc.org/cloud.google.com/go/storage?importers=&utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/std?tab=packages",
			to:   "https://godoc.org/-/go?utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/search?q=foo",
			to:   "https://godoc.org/?q=foo&utm_source=backtogodoc",
		},
		{
			from: "https://pkg.go.dev/about",
			to:   "https://godoc.org/-/about?utm_source=backtogodoc",
		},
	}

	for _, tc := range testCases {
		u, err := url.Parse(tc.from)
		if err != nil {
			t.Errorf("url.Parse(%q): %v", tc.from, err)
			continue
		}
		to := godoc(u)
		if got, want := to.String(), tc.to; got != want {
			t.Errorf("godocURL(%q) = %q; want %q", u, got, want)
		}
	}
}
