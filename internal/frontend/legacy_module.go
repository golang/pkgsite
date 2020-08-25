// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"errors"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// legacyServeModulePage serves details pages for the module specified by modulePath
// and version.
func (s *Server) legacyServeModulePage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, modulePath, requestedVersion, resolvedVersion string) error {
	// This function handles top level behavior related to the existence of the
	// requested modulePath@version:
	// TODO: fix
	//   1. If the module version exists, serve it.
	//   2. else if we got any unexpected error, serve a server error
	//   3. else if the error is NotFound, serve the directory page
	//   3. else, we didn't find the module so there are two cases:
	//     a. We don't know anything about this module: just serve a 404
	//     b. We have valid versions for this module path, but `version` isn't
	//        one of them. Serve a 404 but recommend the other versions.
	ctx := r.Context()
	mi, err := ds.LegacyGetModuleInfo(ctx, modulePath, resolvedVersion)
	if err == nil {
		readme := &internal.Readme{Filepath: mi.LegacyReadmeFilePath, Contents: mi.LegacyReadmeContents}
		return s.serveModulePage(ctx, w, r, ds, &mi.ModuleInfo, readme, requestedVersion)
	}
	if !errors.Is(err, derrors.NotFound) {
		return err
	}
	if requestedVersion != internal.LatestVersion {
		_, err = ds.LegacyGetModuleInfo(ctx, modulePath, internal.LatestVersion)
		if err == nil {
			return pathFoundAtLatestError(ctx, "module", modulePath, displayVersion(requestedVersion, modulePath))
		}
		if !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "error checking for latest module: %v", err)
		}
	}
	return pathNotFoundError(ctx, "module", modulePath, requestedVersion)
}
