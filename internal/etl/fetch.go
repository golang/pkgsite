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
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/stdlib"
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
func fetchAndInsertVersion(parentCtx context.Context, modulePath, version string, proxyClient *proxy.Client, db *postgres.DB) (hasIncompletePackages bool, err error) {
	defer derrors.Wrap(&err, "FetchAndInsertVersion(%q, %q)", modulePath, version)

	if ProxyRemoved[modulePath+"@"+version] {
		log.Infof(parentCtx, "not fetching %s@%s because it is on the ProxyRemoved list", modulePath, version)
		return false, derrors.Excluded
	}

	exc, err := db.IsExcluded(parentCtx, modulePath)
	if err != nil {
		return false, err
	}
	if exc {
		return false, derrors.Excluded
	}

	parentSpan := trace.FromContext(parentCtx)
	// A fixed timeout for FetchAndInsertVersion to allow module processing to
	// succeed even for extremely short lived requests.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	ctx, span := trace.StartSpanWithRemoteParent(ctx, "FetchAndInsertVersion", parentSpan.SpanContext())
	defer span.End()

	v, hasIncompletePackages, err := fetch.FetchVersion(ctx, modulePath, version, proxyClient)
	if err != nil {
		return false, err
	}
	log.Infof(ctx, "Fetched %s@%s", v.ModulePath, v.Version)

	if modulePath != stdlib.ModulePath {
		goModFile, err := proxyClient.GetMod(ctx, v.ModulePath, v.Version)
		if err != nil {
			return false, fmt.Errorf("%v: %w", err, derrors.BadModule)
		}
		canonicalPath := goModFile.Module.Mod.Path
		if canonicalPath != v.ModulePath {
			// The module path in the go.mod file doesn't match the path of the
			// zip file. Don't insert the module; instead, record the mapping
			// between the given module path and the canonical path.
			if err := db.InsertAlternativeModulePath(ctx, &internal.AlternativeModulePath{
				Alternative: v.ModulePath,
				Canonical:   canonicalPath,
			}); err != nil {
				return false, err
			}
			// Delete all versions of the module with the alternative path.
			if err := db.DeleteAlternatives(ctx, v.ModulePath); err != nil {
				return false, err
			}
			// Store an AlternativeModule status in module_version_states.
			return false, fmt.Errorf("module path=%s, go.mod path=%s: %w",
				v.ModulePath, canonicalPath, derrors.AlternativeModule)
		}
	}

	if err = db.InsertVersion(ctx, v); err != nil {
		return false, err
	}
	log.Infof(ctx, "Inserted %s@%s", v.ModulePath, v.Version)
	return hasIncompletePackages, nil
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
	hasIncompletePackages, fetchErr := fetchAndInsertVersion(ctx, modulePath, version, client, db)
	if fetchErr != nil {
		code = derrors.ToHTTPStatus(fetchErr)
		logf := log.Errorf
		if code < 500 {
			logf = log.Infof
		}
		logf(ctx, "Error executing fetch: %v (code %d)", fetchErr, code)
	}
	if hasIncompletePackages {
		code = hasIncompletePackagesCode
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
