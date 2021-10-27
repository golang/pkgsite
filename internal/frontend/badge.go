// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"net/http"
	"strings"
)

type badgePage struct {
	basePage
	// LinkPath is the URL path of the badge will link to.
	LinkPath string
	// BadgePath is the URL path of the badge SVG.
	BadgePath string
}

// badgeHandler serves a Go SVG badge image for requests to /badge/<path>
// and a badge generation tool page for requests to /badge/[?path=<path>].
func (s *Server) badgeHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/badge/")
	if path != "" {
		serveFileFS(w, r, s.staticFS, "frontend/badge/badge.svg")
		return
	}

	// The user may input a fully qualified URL (https://pkg.go.dev/net/http
	// or https://github.com/my/module) or just a pathname (net/http).
	path = strings.TrimPrefix(r.URL.Query().Get("path"), "https://pkg.go.dev/")
	urlSchemeIdx := strings.Index(path, "://")
	if urlSchemeIdx > -1 {
		path = path[urlSchemeIdx+3:]
	}

	page := badgePage{
		basePage:  s.newBasePage(r, "Badge generation tool"),
		LinkPath:  path,
		BadgePath: "badge/" + path + ".svg",
	}
	s.servePage(r.Context(), w, "badge", page)
}
