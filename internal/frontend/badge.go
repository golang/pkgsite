// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type badgePage struct {
	basePage
	SiteURL string
	Path    string
}

// badgeHandler serves a Go SVG badge image for requests to /badge/<path>
// and a badge generation tool page for requests to /badge/[?path=<path>].
func (s *Server) badgeHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/badge/")
	if path != "" {
		http.ServeFile(w, r, fmt.Sprintf("%s/img/badge.svg", s.staticPath))
		return
	}

	// The user may input a fully qualified URL (https://pkg.go.dev/net/http?tab=doc)
	// or just a pathname (net/http). Using url.Parse we handle both cases.
	inputURL := r.URL.Query().Get("path")
	parsedURL, _ := url.Parse(inputURL)
	if parsedURL != nil {
		path = strings.TrimPrefix(parsedURL.RequestURI(), "/")
	}

	page := badgePage{
		basePage: s.newBasePage(r, "Badge generation tool"),
		SiteURL:  "https://" + r.Host,
		Path:     path,
	}
	s.servePage(r.Context(), w, "badge.tmpl", page)
}
