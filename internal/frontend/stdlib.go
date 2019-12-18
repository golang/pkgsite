// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/mod/module"
)

// handleStdLib handles a request for a stdlib package or module.
func (s *Server) handleStdLib(w http.ResponseWriter, r *http.Request) {
	path, version, err := parseStdLibURLPath(r.URL.Path)
	if err != nil {
		log.Errorf(r.Context(), "handleStdLib: %v", err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}
	if path == stdlib.ModulePath {
		s.serveModulePage(w, r, stdlib.ModulePath, version)
		return
	}

	// Package "C" is a special case: redirect to the Go Blog article on cgo.
	// (This is what godoc.org does.)
	if path == "C" {
		http.Redirect(w, r, "https://golang.org/doc/articles/c_go_cgo.html", http.StatusMovedPermanently)
		return
	}
	s.servePackagePage(w, r, path, stdlib.ModulePath, version)
}

func parseStdLibURLPath(urlPath string) (path, version string, err error) {
	defer derrors.Wrap(&err, "parseStdLibURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<path>@<tag> or /<path>
	parts := strings.SplitN(urlPath, "@", 2)
	path = strings.TrimSuffix(strings.TrimPrefix(parts[0], "/"), "/")
	if err := module.CheckImportPath(path); err != nil {
		return "", "", err
	}

	if len(parts) == 1 {
		return path, internal.LatestVersion, nil
	}
	version = stdlib.VersionForTag(parts[1])
	if version == "" {
		return "", "", fmt.Errorf("invalid Go tag for url: %q", urlPath)
	}
	return path, version, nil
}
