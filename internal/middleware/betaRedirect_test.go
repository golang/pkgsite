// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleBetaPkgGoDevRedirect(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	h := BetaPkgGoDevRedirect()(handler)

	for _, test := range []struct {
		name, url, wantLocationHeader, wantSetCookieHeader string
		wantStatusCode                                     int
		cookie                                             *http.Cookie
	}{
		{
			name:                "test betapkggodev-redirect param is on",
			url:                 "https://pkg.go.dev/net/http?tab=doc&betaredirect=on",
			wantLocationHeader:  "https://beta.pkg.go.dev/net/http?tab=doc&utm_source=pkggodev",
			wantSetCookieHeader: "betapkggodev-redirect=on; Path=/",
			wantStatusCode:      http.StatusFound,
		},
		{
			name:                "test betapkggodev-redirect param is off",
			url:                 "https://pkg.go.dev/net/http?betaredirect=off",
			wantLocationHeader:  "",
			wantSetCookieHeader: "betapkggodev-redirect=; Path=/; Max-Age=0",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "test betapkggodev-redirect param is unset",
			url:                 "https://pkg.go.dev/net/http",
			wantLocationHeader:  "",
			wantSetCookieHeader: "",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "toggle enabled betapkggodev-redirect cookie",
			url:                 "https://pkg.go.dev/net/http?betaredirect=off",
			cookie:              &http.Cookie{Name: "betapkggodev-redirect", Value: "true"},
			wantLocationHeader:  "",
			wantSetCookieHeader: "betapkggodev-redirect=; Path=/; Max-Age=0",
			wantStatusCode:      http.StatusOK,
		},
		{
			name:                "betapkggodev-redirect enabled cookie should redirect",
			url:                 "https://pkg.go.dev/net/http",
			cookie:              &http.Cookie{Name: "betapkggodev-redirect", Value: "on"},
			wantLocationHeader:  "https://beta.pkg.go.dev/net/http?utm_source=pkggodev",
			wantSetCookieHeader: "",
			wantStatusCode:      http.StatusFound,
		},
		{
			name:           "do not redirect if user is returning from beta.pkg.go.dev",
			url:            "https://pkg.go.dev/net/http?utm_source=backtopkggodev",
			cookie:         &http.Cookie{Name: "betapkggodev-redirect", Value: "on"},
			wantStatusCode: http.StatusOK,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.url, nil)
			if test.cookie != nil {
				req.AddCookie(test.cookie)
			}

			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			resp := w.Result()

			if got, want := resp.Header.Get("Location"), test.wantLocationHeader; got != want {
				t.Errorf("Location header mismatch: got %q; want %q", got, want)
			}

			if got, want := resp.Header.Get("Set-Cookie"), test.wantSetCookieHeader; got != want {
				t.Errorf("Set-Cookie header mismatch: got %q; want %q", got, want)
			}

			if got, want := resp.StatusCode, test.wantStatusCode; got != want {
				t.Errorf("Status code mismatch: got %d; want %d", got, want)
			}
		})
	}
}
