// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.opencensus.io/trace"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/cache"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

// ProxyRemoved is a set of module@version that have been removed from the proxy,
// even though they are still in the index.
var ProxyRemoved = map[string]bool{}

// fetchTask represents the result of a fetch task that was processed.
type fetchTask struct {
	fetch.FetchResult
	timings map[string]time.Duration
}

// A Fetcher holds state for fetching modules.
type Fetcher struct {
	ProxyClient  *proxy.Client
	SourceClient *source.Client
	DB           *postgres.DB
	Cache        *cache.Cache
}

// FetchAndUpdateState fetches and processes a module version, and then updates
// the module_version_states table according to the result. It returns an HTTP
// status code representing the result of the fetch operation, and a non-nil
// error if this status code is not 200.
func (f *Fetcher) FetchAndUpdateState(ctx context.Context, modulePath, requestedVersion, appVersionLabel string) (_ int, resolvedVersion string, err error) {
	defer derrors.Wrap(&err, "FetchAndUpdateState(%q, %q, %q)", modulePath, requestedVersion, appVersionLabel)
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

	// Whenever we fetch a module, make sure its raw latest information is up to
	// date in the DB.
	if err := f.fetchAndUpdateRawLatest(ctx, modulePath); err != nil {
		// Do not fail the fetch just because we couldn't update the raw latest info.
		log.Errorf(ctx, "updating raw latest: %v", err)
	}

	ft := f.fetchAndInsertModule(ctx, modulePath, requestedVersion)
	span.AddAttributes(trace.Int64Attribute("numPackages", int64(len(ft.PackageVersionStates))))

	// If there were any errors processing the module then we didn't insert it.
	// Delete it in case we are reprocessing an existing module.
	// However, don't delete if the error was internal, or we are shedding load.
	if ft.Status >= 400 && ft.Status < 500 {
		if err := deleteModule(ctx, f.DB, ft); err != nil {
			log.Error(ctx, err)
			ft.Error = err
			ft.Status = http.StatusInternalServerError
		}
		// Do not return an error here, because we want to insert into
		// module_version_states below.
	}
	// Regardless of what the status code is, insert the result into
	// version_map, so that a response can be returned for frontend_fetch.
	if err := updateVersionMap(ctx, f.DB, ft); err != nil {
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
		return ft.Status, ft.ResolvedVersion, ft.Error
	}

	// Update the module_version_states table with the new status of
	// module@version. This must happen last, because if it succeeds with a
	// code < 500 but a later action fails, we will never retry the later
	// action.
	// TODO(golang/go#39628): Split UpsertModuleVersionState into
	// InsertModuleVersionState and UpdateModuleVersionState.
	start := time.Now()
	mvs := &postgres.ModuleVersionStateForUpsert{
		ModulePath:           ft.ModulePath,
		Version:              ft.ResolvedVersion,
		AppVersion:           appVersionLabel,
		Status:               ft.Status,
		HasGoMod:             ft.HasGoMod,
		GoModPath:            ft.GoModPath,
		FetchErr:             ft.Error,
		PackageVersionStates: ft.PackageVersionStates,
	}
	err = f.DB.UpsertModuleVersionState(ctx, mvs)
	ft.timings["db.UpsertModuleVersionState"] = time.Since(start)
	if err != nil {
		log.Error(ctx, err)
		if ft.Error != nil {
			ft.Status = http.StatusInternalServerError
			ft.Error = fmt.Errorf("db.UpsertModuleVersionState: %v, original error: %v", err, ft.Error)
		}
		logTaskResult(ctx, ft, "Failed to update module version state")
		return http.StatusInternalServerError, ft.ResolvedVersion, ft.Error
	}
	logTaskResult(ctx, ft, "Updated module version state")
	return ft.Status, ft.ResolvedVersion, ft.Error
}

// fetchAndInsertModule fetches the given module version from the module proxy
// or (in the case of the standard library) from the Go repo and writes the
// resulting data to the database.
func (f *Fetcher) fetchAndInsertModule(ctx context.Context, modulePath, requestedVersion string) *fetchTask {
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

	exc, err := f.DB.IsExcluded(ctx, modulePath)
	if err != nil {
		ft.Error = err
		return ft
	}
	if exc {
		ft.Error = derrors.Excluded
		return ft
	}

	// Fetch the module, and the current @main and @master version of this module.
	// The @main and @master version will be used to update the version_map
	// target if applicable.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		fr := fetch.FetchModule(ctx, modulePath, requestedVersion, f.ProxyClient, f.SourceClient)
		if fr == nil {
			panic("fetch.FetchModule should never return a nil FetchResult")
		}
		defer fr.Defer()
		ft.FetchResult = *fr
		ft.timings["fetch.FetchModule"] = time.Since(start)
	}()
	// Do not resolve the @main and @master version if proxy fetch is disabled.
	var main string
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !f.ProxyClient.FetchDisabled() {
			main = resolvedVersion(ctx, modulePath, internal.MainVersion, f.ProxyClient)
		}
	}()
	var master string
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !f.ProxyClient.FetchDisabled() {
			master = resolvedVersion(ctx, modulePath, internal.MasterVersion, f.ProxyClient)
		}
	}()
	wg.Wait()
	ft.MainVersion = main
	ft.MasterVersion = master

	// There was an error fetching this module.
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

	// The module was successfully fetched.
	log.Infof(ctx, "fetch.FetchModule succeeded for %s@%s", ft.ModulePath, ft.RequestedVersion)
	start := time.Now()
	isLatest, err := f.DB.InsertModule(ctx, ft.Module)
	ft.timings["db.InsertModule"] = time.Since(start)
	if err != nil {
		log.Error(ctx, err)

		ft.Status = derrors.ToStatus(err)
		ft.Error = err
		return ft
	}
	log.Infof(ctx, "db.InsertModule succeeded for %s@%s", ft.ModulePath, ft.RequestedVersion)
	// Invalidate the cache if we just processed the latest version of a module.
	if isLatest {
		if err := f.invalidateCache(ctx, ft.ModulePath); err != nil {
			// Failure to invalidate the cache is not that serious; at worst it means some pages will be stale.
			// (Cache TTLs for details pages configured in internal/frontend/server.go must not be too long,
			// to account for this possibility.)
			log.Errorf(ctx, "failed to invalidate cache for %s: %v", ft.ModulePath, err)
		} else {
			log.Infof(ctx, "invalidated cache for %s", ft.ModulePath)
		}
	}
	return ft
}

// invalidateCache deletes the series path for modulePath, as well as any
// possible URL path of which it is a componentwise prefix. That is, it deletes
// example.com/mod, example.com/mod@v1.2.3 and example.com/mod/pkg, but not the
// unrelated example.com/module.
//
// We delete the series path, not the module path, because adding a v2 module
// can affect v1 pages. For example, the first v2 module will add a "higher
// major version" banner to all v1 pages. While adding a v1 version won't
// currently affect v2 pages, that could change some day (for instance, if we
// decide to provide history). So it's better to be safe and delete all paths in
// the series.
func (f *Fetcher) invalidateCache(ctx context.Context, modulePath string) error {
	if f.Cache == nil {
		return nil
	}
	var errs []error
	seriesPath := internal.SeriesPathForModule(modulePath)
	// All cache keys are request URLs, so they begin with "/".
	if err := f.Cache.Delete(ctx, "/"+seriesPath); err != nil {
		errs = append(errs, err)
	}
	// Delete all suffixes of the series path followed by a character that marks its end.
	for _, end := range "/@?#" {
		if err := f.Cache.DeletePrefix(ctx, fmt.Sprintf("/%s%c", seriesPath, end)); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d errors, first is %w", len(errs), errs[0])
	}
	return nil
}

func resolvedVersion(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client) string {
	if modulePath == stdlib.ModulePath && requestedVersion == internal.MainVersion {
		return ""
	}
	info, err := fetch.GetInfo(ctx, modulePath, requestedVersion, proxyClient)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			// If an error occurs, log it and insert the module as normal.
			log.Errorf(ctx, "fetch.GetInfo(ctx, %q, %q, f.ProxyClient, false): %v", modulePath, requestedVersion, err)
		}
		return ""
	}
	return info.Version
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

	// If the resolved version for the this module version is also the resolved
	// version for @main or @master, update version_map to match.
	requestedVersions := []string{ft.RequestedVersion}
	if ft.MainVersion == ft.ResolvedVersion {
		requestedVersions = append(requestedVersions, internal.MainVersion)
	}
	if ft.MasterVersion == ft.ResolvedVersion {
		requestedVersions = append(requestedVersions, internal.MasterVersion)
	}
	for _, v := range requestedVersions {
		v := v
		vm := &internal.VersionMap{
			ModulePath:       ft.ModulePath,
			RequestedVersion: v,
			ResolvedVersion:  ft.ResolvedVersion,
			Status:           ft.Status,
			GoModPath:        ft.GoModPath,
			Error:            errMsg,
		}
		if err := db.UpsertVersionMap(ctx, vm); err != nil {
			return err
		}
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

// fetchAndUpdateRawLatest fetches information about the raw latest version from the proxy,
// and updates the database if the version has changed.
func (f *Fetcher) fetchAndUpdateRawLatest(ctx context.Context, modulePath string) (err error) {
	defer derrors.Wrap(&err, "fetchAndUpdateRawLatest(%q)", modulePath)
	if modulePath == stdlib.ModulePath {
		return nil
	}
	info, err := fetch.RawLatestInfo(ctx, modulePath, f.ProxyClient, func(v string) (bool, error) {
		modinfo, err := f.DB.GetModuleInfo(ctx, modulePath, v)
		if err != nil {
			return false, err
		}
		return modinfo.HasGoMod, nil
	})
	if err != nil {
		return err
	}
	if info == nil {
		// No info (e.g. for stdlib); that's fine.
		return nil
	}
	return f.DB.UpdateRawLatestInfo(ctx, info)
}
