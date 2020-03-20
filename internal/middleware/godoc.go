// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net/http"
	"net/url"
)

type godocURLContextKey struct{}

// GodocURLFromContext returns a godoc.org URL associated with a context.
// The value may not be present, in which case nil is returned.
func GodocURLFromContext(ctx context.Context) *url.URL {
	u, _ := ctx.Value(godocURLContextKey{}).(*url.URL)
	return u
}

func newContextWithGodocURL(ctx context.Context, u *url.URL) context.Context {
	return context.WithValue(ctx, godocURLContextKey{}, u)
}

// GodocURL adds a corresponding godoc.org URL value to the request
// context if the request is due to godoc.org automatically redirecting a user.
func GodocURL() Middleware {
	// In order to reliably know that a request is coming to pkg.go.dev from
	// godoc.org, we look for a utm_source GET parameter set to 'godoc'.
	// If we see this, we set a temporary cookie and redirect to the
	// pkg.go.dev URL with the utm_source param stripped (so that it doesn’t
	// remain in all our URLs coming from godoc.org). If this temporary cookie
	// is seen, it is marked to be deleted and the correct value for the
	// “Back to godoc.org” link is set. The existence of this value will be
	// used to determine whether to show the button in the UI.
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const tmpCookieName = "tmp-from-godoc"

			// If the user is redirected from godoc.org, the request’s URL will have
			// utm_source=godoc.
			if r.FormValue("utm_source") == "godoc" {
				http.SetCookie(w, &http.Cookie{
					Name:     tmpCookieName,
					Value:    "1",
					SameSite: http.SameSiteStrictMode,
				})

				// Redirect to the same URL only without the utm_source parameter.
				u := r.URL
				q := u.Query()
				q.Del("utm_source")
				u.RawQuery = q.Encode()
				http.Redirect(w, r, u.String(), http.StatusSeeOther)
				return
			}

			if _, err := r.Cookie(tmpCookieName); err == http.ErrNoCookie {
				h.ServeHTTP(w, r) // cookie isn’t set, so can just proceed as normal
				return
			}

			// If the temporary cookie is set, delete it and add the godoc URL to the
			// request’s context.
			http.SetCookie(w, &http.Cookie{
				Name:   tmpCookieName,
				MaxAge: -1,
			})
			ctx := newContextWithGodocURL(r.Context(), godoc(r.URL))
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// godoc takes a Discovery URL and returns the corresponding
// godoc.org equivalent.
func godoc(u *url.URL) *url.URL {
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
	return result
}
