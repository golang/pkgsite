// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"net/http"
	"net/url"

	"golang.org/x/discovery/internal/log"
)

// GodocURLPlaceholder should be used as the value for any godoc.org URL in rendered
// content. It is substituted for the actual godoc.org URL value by the GodocURL middleware.
const GodocURLPlaceholder = "$$GODISCOVERY_GODOCURL$$"

// GodocURL adds a corresponding godoc.org URL value to the rendered page
// if the request is due to godoc.org automatically redirecting a user.
// The value is empty otherwise.
func GodocURL() Middleware {
	// In order to reliably know that a request is coming to pkg.go.dev from
	// godoc.org, we look for a utm_source GET parameter set to 'godoc'.
	// If we see this, we set a temporary cookie and redirect to the
	// pkg.go.dev URL with the utm_source param stripped (so that it doesn’t
	// remain in all our URLs coming from godoc.org). If this temporary cookie
	// is seen, a non-empty value for the “Back to godoc.org” link is set.
	// The existence of this value will be used to determine whether to show the
	// button in the UI.
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const tmpCookieName = "tmp-from-godoc"

			// If the user is redirected from godoc.org, the request’s URL will have
			// utm_source=godoc.
			if r.FormValue("utm_source") == "godoc" {
				http.SetCookie(w, &http.Cookie{
					Name:     tmpCookieName,
					Value:    "1",
					SameSite: http.SameSiteLaxMode, // request can originate from another domain via redirect
				})

				// Redirect to the same URL only without the utm_source parameter.
				u := r.URL
				q := u.Query()
				q.Del("utm_source")
				u.RawQuery = q.Encode()
				http.Redirect(w, r, u.String(), http.StatusFound)
				return
			}

			godocURL := godoc(r.URL)
			if _, err := r.Cookie(tmpCookieName); err == http.ErrNoCookie {
				// Cookie isn’t set, indicating user is not coming from godoc.org.
				godocURL = ""
			} else {
				http.SetCookie(w, &http.Cookie{
					Name:   tmpCookieName,
					MaxAge: -1,
				})
			}

			// TODO(b/144509703): avoid copying if possible
			crw := &capturingResponseWriter{ResponseWriter: w}
			h.ServeHTTP(crw, r)
			body := crw.bytes()
			body = bytes.ReplaceAll(body, []byte(GodocURLPlaceholder), []byte(godocURL))
			if _, err := w.Write(body); err != nil {
				log.Errorf(r.Context(), "GodocURL, writing: %v", err)
			}
		})
	}
}

// godoc takes a Discovery URL and returns the corresponding godoc.org equivalent.
func godoc(u *url.URL) string {
	result := &url.URL{Scheme: "https", Host: "godoc.org"}

	switch u.Path {
	case "/std":
		result.Path = "/-/go"
	case "/about":
		result.Path = "/-/about"
	case "/search":
		result.Path = "/"
		result.RawQuery = u.RawQuery
	default:
		{
			result.Path = u.Path
			switch u.Query().Get("tab") {
			case "imports":
				result.RawQuery = "imports"
			case "importedby":
				result.RawQuery = "importers"
			case "subdirectories":
				result.Fragment = "pkg-subdirectories"
			}
		}
	}
	q := result.Query()
	q.Add("utm_source", "backtogodoc")
	result.RawQuery = q.Encode()
	return result.String()
}
