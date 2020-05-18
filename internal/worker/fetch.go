// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
)

const (
	// Indicates that although we have a valid module, some packages could not be processed.
	hasIncompletePackagesCode = 290
	hasIncompletePackagesDesc = "incomplete packages"
)

// ProxyRemoved is a set of module@version that have been removed from the proxy,
// even though they are still in the index.
var ProxyRemoved = map[string]bool{}

// fetchTask represents the result of a fetch task that was processed.
type fetchTask struct {
	fetch.FetchResult
	timings map[string]time.Duration
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

	ft := fetchAndInsertModule(ctx, modulePath, requestedVersion, proxyClient, sourceClient, db)
	span.AddAttributes(trace.Int64Attribute("numPackages", int64(len(ft.PackageVersionStates))))
	dbErr := updateVersionMapAndDeleteModulesWithErrors(ctx, db, ft)
	if dbErr != nil {
		log.Error(ctx, dbErr)
		ft.Error = dbErr
		ft.Status = http.StatusInternalServerError
	}
	if !semver.IsValid(ft.ResolvedVersion) {
		return ft.Status, ft.Error
	}

	// Update the module_version_states table with the new status of
	// module@version. This must happen last, because if it succeeds with a
	// code < 500 but a later action fails, we will never retry the later
	// action.
	// TODO(b/139178863): Split UpsertModuleVersionState into
	// InsertModuleVersionState and UpdateModuleVersionState.
	start := time.Now()
	err = db.UpsertModuleVersionState(ctx, ft.ModulePath, ft.ResolvedVersion, config.AppVersionLabel(),
		time.Time{}, ft.Status, ft.GoModPath, ft.Error, ft.PackageVersionStates)
	ft.timings["db.UpsertModuleVersionState"] = time.Since(start)
	if err != nil {
		log.Error(ctx, err)
		if ft.Error != nil {
			ft.Status = http.StatusInternalServerError
			ft.Error = fmt.Errorf("db.UpsertModuleVersionState: %v, original error: %v", err, ft.Error)
		}
		logTaskResult(ctx, ft, "Failed to update module version state")
		return http.StatusInternalServerError, ft.Error
	}
	logTaskResult(ctx, ft, "Updated module version state")
	return ft.Status, ft.Error
}

// fetchAndInsertModule fetches the given module version from the module proxy
// or (in the case of the standard library) from the Go repo and writes the
// resulting data to the database.
//
// The given parentCtx is used for tracing, but fetches actually execute in a
// detached context with fixed timeout, so that fetches are allowed to complete
// even for short-lived requests.
func fetchAndInsertModule(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client, db *postgres.DB) *fetchTask {
	ft := &fetchTask{
		FetchResult: fetch.FetchResult{
			ModulePath:       modulePath,
			RequestedVersion: requestedVersion,
		},
		timings: map[string]time.Duration{},
	}
	defer func() {
		derrors.Wrap(&ft.Error, "fetchAndInsertModule(%q, %q)", modulePath, requestedVersion)
		if ft.Error != nil {
			ft.Status = derrors.ToHTTPStatus(ft.Error)
			ft.ResolvedVersion = requestedVersion
		}
	}()

	if ProxyRemoved[modulePath+"@"+requestedVersion] {
		log.Infof(ctx, "not fetching %s@%s because it is on the ProxyRemoved list", modulePath, requestedVersion)
		ft.Error = derrors.Excluded
		return ft
	}

	exc, err := db.IsExcluded(ctx, modulePath)
	if err != nil {
		ft.Error = err
		return ft
	}
	if exc {
		ft.Error = derrors.Excluded
		return ft
	}

	start := time.Now()
	fr := fetch.FetchModule(ctx, modulePath, requestedVersion, proxyClient, sourceClient)
	if fr == nil {
		panic("fetch.FetchModule should never return a nil FetchResult")
	}
	ft.FetchResult = *fr
	ft.timings["fetch.FetchModule"] = time.Since(start)
	if ft.Error != nil {
		logf := log.Errorf
		if ft.Status < 500 {
			logf = log.Infof
		}
		logf(ctx, "Error executing fetch: %v (code %d)", ft.Error, ft.Status)
		return ft
	}
	log.Infof(ctx, "fetch.FetchVersion succeeded for %s@%s", ft.ModulePath, ft.RequestedVersion)

	start = time.Now()
	err = db.InsertModule(ctx, ft.Module)
	ft.timings["db.InsertModule"] = time.Since(start)
	if err != nil {
		log.Error(ctx, err)

		ft.Status = derrors.ToHTTPStatus(err)
		ft.Error = err
		return ft
	}
	log.Infof(ctx, "db.InsertModule succeeded for %s@%s", ft.ModulePath, ft.RequestedVersion)
	return ft
}

func updateVersionMapAndDeleteModulesWithErrors(ctx context.Context, db *postgres.DB, ft *fetchTask) (err error) {
	defer derrors.Wrap(&err, "updateVersionMapAndDeleteModulesWithErrors(%q, %q, %q, %d, %v)",
		ft.ModulePath, ft.RequestedVersion, ft.ResolvedVersion, ft.Status, ft.Error)

	ctx, span := trace.StartSpan(ctx, "worker.updateFetchResult")
	defer span.End()

	var errMsg string
	if ft.Error != nil {
		errMsg = ft.Error.Error()
	}
	vm := &internal.VersionMap{
		ModulePath:       ft.ModulePath,
		RequestedVersion: ft.RequestedVersion,
		ResolvedVersion:  ft.ResolvedVersion,
		Status:           ft.Status,
		Error:            errMsg,
	}
	start := time.Now()
	err = db.UpsertVersionMap(ctx, vm)
	ft.timings["db.UpsertVersionMap"] = time.Since(start)
	if err != nil {
		return err
	}
	if !semver.IsValid(vm.ResolvedVersion) {
		// If the requestedVersion was not successfully resolved, at
		// this point it will be the same as the resolvedVersion.
		// No additional tables need to be updated.
		return nil
	}

	// If there were any errors processing the module then we didn't insert it.
	// Delete it in case we are reprocessing an existing module.
	if vm.Status > 400 {
		log.Infof(ctx, "%s@%s: code=%d, deleting", vm.ModulePath, vm.ResolvedVersion, vm.Status)
		start = time.Now()
		err = db.DeleteModule(ctx, vm.ModulePath, vm.ResolvedVersion)
		ft.timings["db.DeleteModule"] = time.Since(start)
		if err != nil {
			return err
		}
	}

	// If this was an alternative path (ft.Status == 491) and there is an older
	// version in search_documents, delete it. This is the case where a module's
	// canonical path was changed by the addition of a go.mod file. For example,
	// versions of logrus before it acquired a go.mod file could have the path
	// github.com/Sirupsen/logrus, but once the go.mod file specifies that the
	// path is all lower-case, the old versions should not show up in search. We
	// still leave their pages in the database so users of those old versions
	// can still view documentation.
	if vm.Status == derrors.ToHTTPStatus(derrors.AlternativeModule) {
		log.Infof(ctx, "%s@%s: code=491, deleting older version from search", vm.ModulePath, vm.ResolvedVersion)
		start = time.Now()
		err = db.DeleteOlderVersionFromSearchDocuments(ctx, vm.ModulePath, vm.ResolvedVersion)
		ft.timings["db.DeleteOlderVersionFromSearchDocuments"] = time.Since(start)
		if err != nil {
			return err
		}
	}
	return nil
}

func logTaskResult(ctx context.Context, ft *fetchTask, prefix string) {
	var times []string
	for k, v := range ft.timings {
		times = append(times, fmt.Sprintf("%s=%.3fs", k, v.Seconds()))
	}
	sort.Strings(times)
	msg := strings.Join(times, ", ")
	logf := log.Infof
	if ft.Status == http.StatusInternalServerError {
		logf = log.Errorf
	}
	logf(ctx, "%s for %s@%s: code=%d, num_packages=%d, err=%v; timings: %s",
		prefix, ft.ModulePath, ft.ResolvedVersion, ft.Status, len(ft.PackageVersionStates), ft.Error, msg)
}
