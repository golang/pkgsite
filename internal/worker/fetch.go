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
	"unicode/utf8"

	"go.opencensus.io/trace"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
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
func FetchAndUpdateState(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client, db *postgres.DB, appVersionLabel string) (_ int, err error) {
	defer derrors.Wrap(&err, "FetchAndUpdateState(%q, %q)", modulePath, requestedVersion)

	tctx, span := trace.StartSpan(ctx, "FetchAndUpdateState")
	ctx = experiment.NewContext(tctx, experiment.FromContext(ctx).Active()...)
	ctx = log.NewContextWithLabel(ctx, "fetch", modulePath+"@"+requestedVersion)
	if !utf8.ValidString(modulePath) {
		log.Errorf(ctx, "module path %q is not valid UTF-8", modulePath)
	}
	if !utf8.ValidString(requestedVersion) {
		log.Errorf(ctx, "requested version %q is not valid UTF-8", requestedVersion)
	}
	span.AddAttributes(
		trace.StringAttribute("modulePath", modulePath),
		trace.StringAttribute("version", requestedVersion))
	defer span.End()

	ft := fetchAndInsertModule(ctx, modulePath, requestedVersion, proxyClient, sourceClient, db)
	span.AddAttributes(trace.Int64Attribute("numPackages", int64(len(ft.PackageVersionStates))))

	// If there were any errors processing the module then we didn't insert it.
	// Delete it in case we are reprocessing an existing module.
	// However, don't delete if the error was internal, or we are shedding load.
	if ft.Status >= 400 && ft.Status < 500 {
		if err := deleteModule(ctx, db, ft); err != nil {
			log.Error(ctx, err)
			ft.Error = err
			ft.Status = http.StatusInternalServerError
		}
		// Do not return an error here, because we want to insert into
		// module_version_states below.
	}
	// Regardless of what the status code is, insert the result into
	// version_map, so that a response can be returned for frontend_fetch.
	if err := updateVersionMap(ctx, db, ft); err != nil {
		log.Error(ctx, err)
		if ft.Status != http.StatusInternalServerError {
			ft.Error = err
			ft.Status = http.StatusInternalServerError
		}
		// Do not return an error here, because we want to insert into
		// module_version_states below.
	}
	if !semver.IsValid(ft.ResolvedVersion) {
		// If the requestedVersion was not successfully resolved to a semantic
		// version, then at this point it will be the same as the
		// resolvedVersion. This fetch request does not need to be recorded in
		// module_version_states, since that table is only used to track
		// modules that have been published to index.golang.org.
		return ft.Status, ft.Error
	}

	// Update the module_version_states table with the new status of
	// module@version. This must happen last, because if it succeeds with a
	// code < 500 but a later action fails, we will never retry the later
	// action.
	// TODO(golang/go#39628): Split UpsertModuleVersionState into
	// InsertModuleVersionState and UpdateModuleVersionState.
	start := time.Now()
	err = db.UpsertModuleVersionState(ctx, ft.ModulePath, ft.ResolvedVersion, appVersionLabel,
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
			ft.Status = derrors.ToStatus(ft.Error)
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
	defer fr.Defer()
	ft.FetchResult = *fr
	ft.timings["fetch.FetchModule"] = time.Since(start)
	if ft.Error != nil {
		logf := log.Infof
		if ft.Status == http.StatusServiceUnavailable {
			logf = log.Warningf
		} else if ft.Status >= 500 && ft.Status != derrors.ToStatus(derrors.ProxyTimedOut) {
			logf = log.Errorf
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

		ft.Status = derrors.ToStatus(err)
		ft.Error = err
		return ft
	}
	log.Infof(ctx, "db.InsertModule succeeded for %s@%s", ft.ModulePath, ft.RequestedVersion)
	return ft
}

func updateVersionMap(ctx context.Context, db *postgres.DB, ft *fetchTask) (err error) {
	start := time.Now()
	defer func() {
		ft.timings["worker.updatedVersionMap"] = time.Since(start)
		derrors.Wrap(&err, "updateVersionMap(%q, %q, %q, %d, %v)",
			ft.ModulePath, ft.RequestedVersion, ft.ResolvedVersion, ft.Status, ft.Error)
	}()
	ctx, span := trace.StartSpan(ctx, "worker.updateVersionMap")
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
		GoModPath:        ft.GoModPath,
		Error:            errMsg,
	}
	if err := db.UpsertVersionMap(ctx, vm); err != nil {
		return err
	}
	return nil
}

func deleteModule(ctx context.Context, db *postgres.DB, ft *fetchTask) (err error) {
	start := time.Now()
	defer func() {
		ft.timings["worker.deleteModule"] = time.Since(start)
		derrors.Wrap(&err, "deleteModule(%q, %q, %q, %d, %v)",
			ft.ModulePath, ft.RequestedVersion, ft.ResolvedVersion, ft.Status, ft.Error)
	}()
	ctx, span := trace.StartSpan(ctx, "worker.deleteModule")
	defer span.End()

	log.Infof(ctx, "%s@%s: code=%d, deleting", ft.ModulePath, ft.ResolvedVersion, ft.Status)
	if err := db.DeleteModule(ctx, ft.ModulePath, ft.ResolvedVersion); err != nil {
		return err
	}
	// If this was an alternative path (ft.Status == 491) and there is an older
	// version in search_documents, delete it. This is the case where a module's
	// canonical path was changed by the addition of a go.mod file. For example,
	// versions of logrus before it acquired a go.mod file could have the path
	// github.com/Sirupsen/logrus, but once the go.mod file specifies that the
	// path is all lower-case, the old versions should not show up in search. We
	// still leave their pages in the database so users of those old versions
	// can still view documentation.
	if ft.Status == derrors.ToStatus(derrors.AlternativeModule) {
		log.Infof(ctx, "%s@%s: code=491, deleting older version from search", ft.ModulePath, ft.ResolvedVersion)
		if err := db.DeleteOlderVersionFromSearchDocuments(ctx, ft.ModulePath, ft.ResolvedVersion); err != nil {
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
