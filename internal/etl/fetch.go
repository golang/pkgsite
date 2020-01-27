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

// fetchTimeout bounds the time allowed for fetching a single module.  It is
// mutable for testing purposes.
var fetchTimeout = 2 * config.StatementTimeout

const (
	// Indicates that although we have a valid module, some packages could not be processed.
	hasIncompletePackagesCode = 290
	hasIncompletePackagesDesc = "incomplete packages"
)

// ProxyRemoved is a set of module@version that have been removed from the proxy,
// even though they are still in the index.
var ProxyRemoved = map[string]bool{}

// fetchAndInsertVersion fetches the given module version from the module proxy
// or (in the case of the standard library) from the Go repo and writes the
// resulting data to the database.
//
// The given parentCtx is used for tracing, but fetches actually execute in a
// detached context with fixed timeout, so that fetches are allowed to complete
// even for short-lived requests.
func fetchAndInsertVersion(parentCtx context.Context, modulePath, version string, proxyClient *proxy.Client, db *postgres.DB) (hasIncompletePackages bool, goModPath string, err error) {
	defer derrors.Wrap(&err, "FetchAndInsertVersion(%q, %q)", modulePath, version)

	if ProxyRemoved[modulePath+"@"+version] {
		log.Infof(parentCtx, "not fetching %s@%s because it is on the ProxyRemoved list", modulePath, version)
		return false, "", derrors.Excluded
	}

	exc, err := db.IsExcluded(parentCtx, modulePath)
	if err != nil {
		return false, "", err
	}
	if exc {
		return false, "", derrors.Excluded
	}

	parentSpan := trace.FromContext(parentCtx)
	// A fixed timeout for FetchAndInsertVersion to allow module processing to
	// succeed even for extremely short lived requests.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	ctx, span := trace.StartSpanWithRemoteParent(ctx, "FetchAndInsertVersion", parentSpan.SpanContext())
	defer span.End()

	res, err := fetch.FetchVersion(ctx, modulePath, version, proxyClient)
	if res != nil {
		goModPath = res.GoModPath
		for _, state := range res.PackageVersionStates {
			if state.Status != http.StatusOK {
				hasIncompletePackages = true
			}
		}
	}
	if err != nil {
		return false, goModPath, err
	}
	log.Infof(ctx, "Fetched %s@%s", res.Version.ModulePath, res.Version.Version)
	if err = db.InsertVersion(ctx, res.Version); err != nil {
		return false, goModPath, err
	}
	log.Infof(ctx, "Inserted %s@%s", res.Version.ModulePath, res.Version.Version)
	return hasIncompletePackages, goModPath, nil
}

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
	hasIncompletePackages, goModPath, fetchErr := fetchAndInsertVersion(ctx, modulePath, version, client, db)
	if fetchErr != nil {
		code = derrors.ToHTTPStatus(fetchErr)
		logf := log.Errorf
		if code < 500 {
			logf = log.Infof
		}
		logf(ctx, "Error executing fetch: %v (code %d)", fetchErr, code)
	}
	// TODO(b/147348928): return this information as an error from fetchAndInsertVersion.
	if hasIncompletePackages {
		code = hasIncompletePackagesCode
	}

	// If there were any errors processing the module then we didn't insert it.
	// Delete it in case we are reprocessing an existing module.
	
	
	if code > 400 {
		log.Infof(ctx, "%s@%s: code=%d, deleting", modulePath, version, code)
		if err := db.DeleteVersion(ctx, nil, modulePath, version); err != nil {
			log.Error(ctx, err)
			return http.StatusInternalServerError, err
		}
	}

	// If this was an alternative path (code == 491) and there is an older
	// version in search_documents, delete it. This is the case where a module's
	// canonical path was changed by the addition of a go.mod file. For example,
	// versions of logrus before it acquired a go.mod file could have the path
	// github.com/Sirupsen/logrus, but once the go.mod file specifies that the
	// path is all lower-case, the old versions should not show up in search. We
	// still leave their pages in the database so users of those old versions
	// can still view documentation.
	if code == 491 {
		log.Infof(ctx, "%s@%s: code=491, deleting older version from search", modulePath, version)
		if err := db.DeleteOlderVersionFromSearchDocuments(ctx, modulePath, version); err != nil {
			log.Error(ctx, err)
			return http.StatusInternalServerError, err
		}
	}

	// Update the module_version_states table with the new status of
	// module@version. This must happen last, because if it succeeds with a
	// code < 500 but a later action fails, we will never retry the later action.

	// TODO(b/139178863): Split UpsertModuleVersionState into InsertModuleVersionState and UpdateModuleVersionState.
	if err := db.UpsertModuleVersionState(ctx, modulePath, version, config.AppVersionLabel(), time.Time{}, code, goModPath, fetchErr); err != nil {
		log.Error(ctx, err)
		if fetchErr != nil {
			err = fmt.Errorf("error updating module version state: %v, original error: %v", err, fetchErr)
		}
		return http.StatusInternalServerError, err
	}
	log.Infof(ctx, "Updated module version state for %s@%s: code=%d, hasIncompletePackages=%t err=%v",
		modulePath, version, code, hasIncompletePackages, fetchErr)
	return code, fetchErr
}
