// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
)

// FetchAndUpdateState is used by the InMemory queue for testing in
// internal/frontend and running cmd/frontend locally. It is a copy of
// worker.FetchAndUpdateState that does not update module_version_states, so that
// we don't have to import internal/worker here. It is not meant to be used
// when running on AppEngine.
func FetchAndUpdateState(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client, db *postgres.DB) (_ int, err error) {
	defer func() {
		if err != nil {
			log.Infof(ctx, "FetchAndUpdateState(%q, %q) completed with err: %v. ", modulePath, requestedVersion, err)
		} else {
			log.Infof(ctx, "FetchAndUpdateState(%q, %q) succeeded", modulePath, requestedVersion)
		}
		derrors.Wrap(&err, "FetchAndUpdateState(%q, %q)", modulePath, requestedVersion)
	}()

	fr := fetch.FetchModule(ctx, modulePath, requestedVersion, proxyClient, sourceClient)
	if fr.Error == nil {
		// Only attempt to insert the module into module_version_states if the
		// fetch process was successful.
		if err := db.InsertModule(ctx, fr.Module); err != nil {
			return http.StatusInternalServerError, err
		}
	}
	var errMsg string
	if fr.Error != nil {
		errMsg = fr.Error.Error()
	}
	vm := &internal.VersionMap{
		ModulePath:       fr.ModulePath,
		RequestedVersion: fr.RequestedVersion,
		ResolvedVersion:  fr.ResolvedVersion,
		GoModPath:        fr.GoModPath,
		Status:           fr.Status,
		Error:            errMsg,
	}
	if err := db.UpsertVersionMap(ctx, vm); err != nil {
		return http.StatusInternalServerError, err
	}
	if fr.Error != nil {
		return fr.Status, fr.Error
	}
	return http.StatusOK, nil
}
