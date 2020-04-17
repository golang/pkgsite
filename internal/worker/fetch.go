// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/config"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/experiment"
	"golang.org/x/discovery/internal/fetch"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/mod/semver"
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

// fetchAndInsertModule fetches the given module version from the module proxy
// or (in the case of the standard library) from the Go repo and writes the
// resulting data to the database.
//
// The given parentCtx is used for tracing, but fetches actually execute in a
// detached context with fixed timeout, so that fetches are allowed to complete
// even for short-lived requests.
func fetchAndInsertModule(parentCtx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client, db *postgres.DB) (_ *fetch.FetchResult, err error) {
	defer derrors.Wrap(&err, "fetchAndInsertModule(%q, %q)", modulePath, requestedVersion)

	if ProxyRemoved[modulePath+"@"+requestedVersion] {
		log.Infof(parentCtx, "not fetching %s@%s because it is on the ProxyRemoved list", modulePath, requestedVersion)
		return nil, derrors.Excluded
	}

	exc, err := db.IsExcluded(parentCtx, modulePath)
	if err != nil {
		return nil, err
	}
	if exc {
		return nil, derrors.Excluded
	}

	parentSpan := trace.FromContext(parentCtx)
	// A fixed timeout for FetchAndInsertModule to allow module processing to
	// succeed even for extremely short lived requests.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	ctx = experiment.NewContext(ctx, experiment.FromContext(parentCtx))
	defer cancel()

	ctx, span := trace.StartSpanWithRemoteParent(ctx, "FetchAndInsertModule", parentSpan.SpanContext())
	defer span.End()

	res, err := fetch.FetchModule(ctx, modulePath, requestedVersion, proxyClient, sourceClient)
	if err != nil {
		return res, err
	}
	log.Infof(ctx, "fetch.FetchVersion succeeded for %s@%s", res.Module.ModulePath, res.Module.Version)
	if err = db.InsertModule(ctx, res.Module); err != nil {
		return res, err
	}
	log.Infof(ctx, "db.InsertModule succeeded for %s@%s", res.Module.ModulePath, res.Module.Version)
	return res, nil
}

// FetchAndUpdateState fetches and processes a module version, and then updates
// the module_version_states table according to the result. It returns an HTTP
// status code representing the result of the fetch operation, and a non-nil
// error if this status code is not 200.
func FetchAndUpdateState(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client, db *postgres.DB) (_ int, err error) {
	defer derrors.Wrap(&err, "FetchAndUpdateState(%q, %q)", modulePath, requestedVersion)

	tctx, span := trace.StartSpan(ctx, "FetchAndUpdateState")
	ctx = experiment.NewContext(tctx, experiment.FromContext(ctx))
	span.AddAttributes(
		trace.StringAttribute("modulePath", modulePath),
		trace.StringAttribute("version", requestedVersion))
	defer span.End()
	var (
		code     = http.StatusOK
		fetchErr error
	)
	res, fetchErr := fetchAndInsertModule(ctx, modulePath, requestedVersion, proxyClient, sourceClient, db)
	if fetchErr != nil {
		code = derrors.ToHTTPStatus(fetchErr)
		logf := log.Errorf
		if code < 500 {
			logf = log.Infof
		}
		logf(ctx, "Error executing fetch: %v (code %d)", fetchErr, code)
	}
	var (
		hasIncompletePackages bool
		goModPath             string
		packageVersionStates  []*internal.PackageVersionState
		resolvedVersion       = requestedVersion
	)
	if res != nil {
		if code == http.StatusOK && res.HasIncompletePackages {
			code = hasIncompletePackagesCode
		}
		goModPath = res.GoModPath
		if res.Module != nil {
			resolvedVersion = res.Module.Version
		}
		packageVersionStates = res.PackageVersionStates
	}

	var errMsg string
	if fetchErr != nil {
		errMsg = fetchErr.Error()
	}
	if err := db.UpsertVersionMap(ctx, &internal.VersionMap{
		ModulePath:       modulePath,
		RequestedVersion: requestedVersion,
		ResolvedVersion:  resolvedVersion,
		Status:           code,
		Error:            errMsg,
	}); err != nil {
		log.Error(ctx, err)
		return http.StatusInternalServerError, err
	}

	if !semver.IsValid(resolvedVersion) {
		// If the requestedVersion was not successfully resolved, at
		// this point it will be the same as the resolvedVersion.  Only
		// in this case, where the requestedVersion is a semantic
		// version, is possible that the module was published in the
		// index, and then later disappeared, so we need to update
		// module_version_states below to reflect these changes.
		// Otherwise, module_version_states does not need to be
		// modified.
		return code, fetchErr
	}

	// If there were any errors processing the module then we didn't insert it.
	// Delete it in case we are reprocessing an existing module.
	if code > 400 {
		log.Infof(ctx, "%s@%s: code=%d, deleting", modulePath, resolvedVersion, code)
		if err := db.DeleteModule(ctx, nil, modulePath, resolvedVersion); err != nil {
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
		log.Infof(ctx, "%s@%s: code=491, deleting older version from search", modulePath, resolvedVersion)
		if err := db.DeleteOlderVersionFromSearchDocuments(ctx, modulePath, resolvedVersion); err != nil {
			log.Error(ctx, err)
			return http.StatusInternalServerError, err
		}
	}

	// Update the module_version_states table with the new status of
	// module@version. This must happen last, because if it succeeds with a
	// code < 500 but a later action fails, we will never retry the later action.

	// TODO(b/139178863): Split UpsertModuleVersionState into InsertModuleVersionState and UpdateModuleVersionState.
	if err := db.UpsertModuleVersionState(ctx, modulePath, resolvedVersion, config.AppVersionLabel(),
		time.Time{}, code, goModPath, fetchErr, packageVersionStates); err != nil {
		log.Error(ctx, err)
		if fetchErr != nil {
			err = fmt.Errorf("error updating module version state: %v, original error: %v", err, fetchErr)
		}
		return http.StatusInternalServerError, err
	}
	log.Infof(ctx, "Updated module version state for %s@%s: code=%d, hasIncompletePackages=%t err=%v",
		modulePath, resolvedVersion, code, hasIncompletePackages, fetchErr)
	return code, fetchErr
}
