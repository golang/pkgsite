// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
)

// fetchAndUpdateState fetches and processes a module version, and then updates
// the module_version_states table according to the result. It returns an HTTP
// status code representing the result of the fetch operation, and a non-nil
// error if this status code is not 200.
func fetchAndUpdateState(ctx context.Context, modulePath, version string, client *proxy.Client, db *postgres.DB) (_ int, err error) {
	defer derrors.Wrap(&err, "fetchAndUpdateState(%q, %q)", modulePath, version)

	ctx, span := trace.StartSpan(ctx, "fetchAndUpdateState")
	span.AddAttributes(
		trace.StringAttribute("modulePath", modulePath),
		trace.StringAttribute("version", version))
	defer span.End()
	var (
		code     = http.StatusOK
		fetchErr error
	)
	hasIncompletePackages, fetchErr := fetch.FetchAndInsertVersion(ctx, modulePath, version, client, db)
	if fetchErr != nil {
		code = derrors.ToHTTPStatus(fetchErr)
		logf := log.Errorf
		if code < 500 {
			logf = log.Infof
		}
		logf(ctx, "Error executing fetch: %v (code %d)", fetchErr, code)
	}
	if hasIncompletePackages {
		code = fetch.HasIncompletePackagesCode
	}

	
	
	
	if code == http.StatusNotFound || code == http.StatusGone {
		log.Infof(ctx, "%s@%s: proxy said 404/410, deleting", modulePath, version)
		if err := db.DeleteVersion(ctx, nil, modulePath, version); err != nil {
			log.Error(ctx, err)
			return http.StatusInternalServerError, err
		}
	}

	// Update the module_version_states table with the new status of
	// module@version. This must happen last, because if it succeeds with a
	// code < 500 but a later action fails, we will never retry the later action.

	// TODO(b/139178863): Split UpsertVersionState into InsertVersionState and UpdateVersionState.
	if err := db.UpsertVersionState(ctx, modulePath, version, config.AppVersionLabel(), time.Time{}, code, fetchErr); err != nil {
		log.Error(ctx, err)
		if fetchErr != nil {
			err = fmt.Errorf("error updating version state: %v, original error: %v", err, fetchErr)
		}
		return http.StatusInternalServerError, err
	}
	log.Infof(ctx, "Updated version state for %s@%s: code=%d, hasIncompletePackages=%t err=%v",
		modulePath, version, code, hasIncompletePackages, fetchErr)
	return code, fetchErr
}
