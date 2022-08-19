// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/mod/module"
)

const (
	goGithubRepoURLPath = "/github.com/golang/go"
	pkgGoDevHost        = "pkg.go.dev"
)

// GodocOrgRedirect redirects requests from godoc.org to pkg.go.dev.
func GodocOrgRedirect() Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.Host, "godoc.org") {
				h.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, pkgGoDevURL(r.URL).String(), http.StatusMovedPermanently)
		})
	}
}

func pkgGoDevURL(godocURL *url.URL) *url.URL {
	u := &url.URL{Scheme: "https", Host: pkgGoDevHost}
	q := url.Values{"utm_source": []string{"godoc"}}

	if strings.Contains(godocURL.Path, "/vendor/") || strings.HasSuffix(godocURL.Path, "/vendor") {
		u.Path = "/"
		u.RawQuery = q.Encode()
		return u
	}

	if strings.HasPrefix(godocURL.Path, goGithubRepoURLPath) ||
		strings.HasPrefix(godocURL.Path, goGithubRepoURLPath+"/src") {
		u.Path = strings.TrimPrefix(strings.TrimPrefix(godocURL.Path, goGithubRepoURLPath), "/src")
		if u.Path == "" {
			u.Path = "/std"
		}
		u.RawQuery = q.Encode()
		return u
	}

	_, isSVG := godocURL.Query()["status.svg"]
	_, isPNG := godocURL.Query()["status.png"]
	if isSVG || isPNG {
		u.Path = "/badge" + godocURL.Path
		u.RawQuery = q.Encode()
		return u
	}

	switch godocURL.Path {
	case "/-/go":
		u.Path = "/std"
	case "/-/about":
		u.Path = "/about"
	case "/C":
		u.Path = "/C"
	case "/":
		if qparam := godocURL.Query().Get("q"); qparam != "" {
			u.Path = "/search"
			q.Set("q", qparam)
		} else {
			u.Path = "/"
		}
	case "":
		u.Path = ""
	case "/-/subrepo":
		u.Path = "/search"
		q.Set("q", "golang.org/x")
	default:
		{
			godocURL.Path = strings.TrimSuffix(godocURL.Path, "/")
			// If the import path is invalid, redirect to
			// https://golang.org/issue/43036, so that the users has more context
			// on why this path does not work on pkg.go.dev.
			if err := module.CheckImportPath(strings.TrimPrefix(godocURL.Path, "/")); err != nil && strings.Contains(err.Error(), "invalid char") {
				u.Host = "golang.org"
				u.Path = "/issue/43036"
				return u
			}

			u.Path = godocURL.Path
			if _, ok := godocURL.Query()["imports"]; ok {
				q.Set("tab", "imports")
			} else if _, ok := godocURL.Query()["importers"]; ok {
				q.Set("tab", "importedby")
			}
		}
	}

	u.RawQuery = q.Encode()
	return u
}
