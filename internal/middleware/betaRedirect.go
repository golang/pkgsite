// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/url"
)

const (
	betaPkgGoDevRedirectCookie = "betapkggodev-redirect"
	betaPkgGoDevRedirectParam  = "betaredirect"
	betaPkgGoDevRedirectOn     = "on"
	betaPkgGoDevRedirectOff    = "off"
	betaPkgGoDevHost           = "beta.pkg.go.dev"
)

// BetaPkgGoDevRedirect redirects requests from pkg.go.dev to beta.pkg.go.dev,
// based on whether a cookie is set for betapkggodev-redirect. The cookie
// can be turned on/off using a query param.
func BetaPkgGoDevRedirect() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userReturningFromBetaPkgGoDev(r) {
				h.ServeHTTP(w, r)
				return
			}

			redirectParam := r.FormValue(betaPkgGoDevRedirectParam)

			if redirectParam == betaPkgGoDevRedirectOn {
				cookie := &http.Cookie{Name: betaPkgGoDevRedirectCookie, Value: redirectParam, Path: "/"}
				http.SetCookie(w, cookie)
			}
			if redirectParam == betaPkgGoDevRedirectOff {
				cookie := &http.Cookie{Name: betaPkgGoDevRedirectCookie, Value: "", MaxAge: -1, Path: "/"}
				http.SetCookie(w, cookie)
			}

			if !shouldRedirectToBetaPkgGoDev(r) {
				h.ServeHTTP(w, r)
				return
			}

			http.Redirect(w, r, betaPkgGoDevURL(r.URL).String(), http.StatusFound)
		})
	}
}

func userReturningFromBetaPkgGoDev(req *http.Request) bool {
	return req.FormValue("utm_source") == "backtopkggodev"
}

func shouldRedirectToBetaPkgGoDev(req *http.Request) bool {
	redirectParam := req.FormValue(betaPkgGoDevRedirectParam)
	if redirectParam == betaPkgGoDevRedirectOn || redirectParam == betaPkgGoDevRedirectOff {
		return redirectParam == betaPkgGoDevRedirectOn
	}
	cookie, err := req.Cookie(betaPkgGoDevRedirectCookie)
	return (err == nil && cookie.Value == betaPkgGoDevRedirectOn)
}

func betaPkgGoDevURL(sourceURL *url.URL) *url.URL {
	values := sourceURL.Query()
	values.Del(betaPkgGoDevRedirectParam)
	values.Set("utm_source", "pkggodev")
	return &url.URL{
		Scheme:   "https",
		Host:     betaPkgGoDevHost,
		Path:     sourceURL.Path,
		RawQuery: values.Encode(),
	}
}
