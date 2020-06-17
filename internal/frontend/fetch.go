// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
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

var (
	// errModuleDoesNotExist indicates that we have attempted to fetch the
	// module, and the proxy returned a status 404/410. There is a row for
	// this module version in version_map.
	errModuleDoesNotExist = errors.New("module does not exist")
	// errPathDoesNotExistInModule indicates that a module for the path prefix
	// exists, but within that module version, this fullPath could not be found.
	errPathDoesNotExistInModule = errors.New("path does not exist in module")
	fetchTimeout                = 30 * time.Second
	pollEvery                   = 500 * time.Millisecond

	// keyFrontendFetchVersion is a census tag for frontend fetch version types.
	keyFrontendFetchVersion = tag.MustNewKey("frontend-fetch.version")
	// keyFrontendFetchStatus is a census tag for frontend fetch status types.
	keyFrontendFetchStatus = tag.MustNewKey("frontend-fetch.status")
	// keyFrontendFetchLatency holds observed latency in individual
	// frontend fetch queries.
	keyFrontendFetchLatency = stats.Float64(
		"go-discovery/frontend-fetch/latency",
		"Latency of a frontend fetch request.",
		stats.UnitMilliseconds,
	)
	// FrontendFetchLatencyDistribution aggregates frontend fetch request
	// latency by status code.
	FrontendFetchLatencyDistribution = &view.View{
		Name:        "go-discovery/frontend-fetch/latency",
		Measure:     keyFrontendFetchLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "FrontendFetch latency, by result source query type.",
		TagKeys:     []tag.Key{keyFrontendFetchStatus},
	}
	// FrontendFetchResponseCount counts frontend fetch responses by response type.
	FrontendFetchResponseCount = &view.View{
		Name:        "go-discovery/frontend-fetch/count",
		Measure:     keyFrontendFetchLatency,
		Aggregation: view.Count(),
		Description: "Frontend fetch request count",
		TagKeys:     []tag.Key{keyFrontendFetchStatus},
	}
)

// fetchHandler checks if a requested path and version exists in the database.
// If not, it will enqueuing potential module versions that could contain
// the requested path and version to a task queue, to be fetched by the worker.
// Meanwhile, the request will poll the database until a row is found, or a
// timeout occurs. A status and responseText will be returned based on the
// result of the request.
// TODO(golang/go#37002): This should be a POST request, since it is causing a change in state.
// update middleware.AcceptMethods so that this can be a POST instead of a GET.
func (s *Server) fetchHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ds.(*postgres.DB); !ok {
		// There's no reason for the proxydatasource to need this codepath.
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	ctx := r.Context()
	if !isActiveFrontendFetch(ctx) {
		// If the experiment flag is not on, treat this as a request for the
		// "fetch" package, which does not exist.
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	// fetchHander accepts a requests following the same URL format as the
	// detailsHandler.
	fullPath, modulePath, requestedVersion, err := parseDetailsURLPath(strings.TrimPrefix(r.URL.Path, "/fetch"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if !isActivePathAtMaster(ctx) && requestedVersion != internal.MasterVersion {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	status, responseText := s.fetchAndPoll(r.Context(), modulePath, fullPath, requestedVersion)
	if status != http.StatusOK {
		http.Error(w, responseText, status)
		return
	}
}

type fetchResult struct {
	modulePath string
	goModPath  string
	status     int
	err        error
}

var statusToResponseText = map[int]string{
	http.StatusOK:                  "",
	http.StatusRequestTimeout:      "This request is taking a little longer than usual. We'll keep working on it - come back in a few minutes!",
	http.StatusInternalServerError: "Something went wrong. We'll keep working on it - try again in a few minutes!",
}

func (s *Server) fetchAndPoll(parentCtx context.Context, modulePath, fullPath, requestedVersion string) (status int, responseText string) {
	start := time.Now()
	defer func() {
		log.Infof(parentCtx, "fetchAndPoll(ctx, ds, q, %q, %q, %q): status=%d, responseText=%q",
			modulePath, fullPath, requestedVersion, status, responseText)
		recordFrontendFetchMetric(status, requestedVersion, time.Since(start))
	}()

	if !semver.IsValid(requestedVersion) &&
		requestedVersion != internal.MasterVersion &&
		requestedVersion != internal.LatestVersion {
		return http.StatusBadRequest, http.StatusText(http.StatusBadRequest)
	}

	// Generate all possible module paths for the fullPath.
	db := s.ds.(*postgres.DB)
	modulePaths, err := modulePathsToFetch(parentCtx, db, fullPath, modulePath)
	if err != nil {
		return derrors.ToHTTPStatus(err), err.Error()
	}

	// Fetch all possible module paths concurrently.
	ctx, cancel := context.WithTimeout(parentCtx, fetchTimeout)
	defer cancel()
	var wg sync.WaitGroup
	results := make([]*fetchResult, len(modulePaths))
	for i, modulePath := range modulePaths {
		wg.Add(1)
		i := i
		modulePath := modulePath
		go func() {
			defer wg.Done()
			start := time.Now()
			fr := s.fetchModule(ctx, fullPath, modulePath, requestedVersion)
			logf := log.Infof
			if fr.status == http.StatusInternalServerError {
				logf = log.Errorf
			}
			logf(ctx, "fetched %s@%s for %s: status=%d, err=%v; took %.3fs", modulePath, requestedVersion, fullPath, fr.status, fr.err, time.Since(start).Seconds())
			results[i] = fr
		}()
	}
	wg.Wait()
	// If the context timed out before all of the requests finished, return an
	// error letting the user to check back later. The worker will still be
	// processing the modules in the background.
	if ctx.Err() != nil {
		log.Infof(ctx, "fetchAndPoll(ctx, ds, q, %q, %q, %q): %v", fullPath, modulePath, requestedVersion, ctx.Err())
		return http.StatusRequestTimeout, statusToResponseText[http.StatusRequestTimeout]
	}

	var moduleMatchingPathPrefix string
	for _, fr := range results {
		// Results are in order of longest module path first. Once an
		// appropriate result is found, return. Otherwise, look at the next path.
		if fr.status == derrors.ToHTTPStatus(derrors.AlternativeModule) {
			return fr.status, fmt.Sprintf("%q is not a supported package path. Were you looking for %q?", fullPath, fr.goModPath)
		}
		if responseText, ok := statusToResponseText[fr.status]; ok {
			return fr.status, responseText
		}
		if errors.Is(fr.err, errPathDoesNotExistInModule) && moduleMatchingPathPrefix == "" {
			// A module was found for a prefix of the path, but the path did
			// not exist in that module. Note the longest possible modulePath in
			// this case, and let the user know that it exists. For example, if
			// the request was for github.com/hashicorp/vault/@master/api,
			// github.com/hashicorp/vault/api does not exist at master, but it
			// does in older versions of github.com/hashicorp/vault.
			moduleMatchingPathPrefix = fr.modulePath
		}
	}
	if moduleMatchingPathPrefix != "" {
		return http.StatusNotFound,
			// TODO(golang/go#37002): return as template.HTML so that link is clickable.
			fmt.Sprintf("%q could not be found. Other versions of module %q may have it! Check them out at https://pkg.go.dev/mod/%s?tab=versions",
				fullPath, moduleMatchingPathPrefix, moduleMatchingPathPrefix)
	}
	p := fullPath
	if requestedVersion != internal.LatestVersion {
		p = fullPath + "@" + requestedVersion
	}
	return http.StatusNotFound, fmt.Sprintf("%q could not be found.", p)
}

func (s *Server) fetchModule(ctx context.Context, fullPath, modulePath, requestedVersion string) (fr *fetchResult) {
	// Before enqueuing the module version to be fetched, check if we have
	// already attempted to fetch it in the past. If so, just return the result
	// from that fetch process.
	db := s.ds.(*postgres.DB)
	fr = checkForPath(ctx, db, fullPath, modulePath, requestedVersion)
	if fr.status == http.StatusOK {
		return fr
	}
	if fr.status != http.StatusProcessing {
		if fr.status != http.StatusRequestTimeout && fr.status != http.StatusInternalServerError {
			fr.err = fmt.Errorf("already attempted to fetch %q in the past and was unsuccessful: %w", fmt.Sprintf("%s@%s", modulePath, requestedVersion), fr.err)
		}
		return fr
	}
	// A row for this modulePath and requestedVersion combination does not
	// exist in version_map. Enqueue the module version to be fetched.
	if err := s.queue.ScheduleFetch(ctx, modulePath, requestedVersion, "", s.taskIDChangeInterval); err != nil {
		fr.err = err
		fr.status = http.StatusInternalServerError
		return fr
	}
	// After the fetch request is enqueued, poll the database until it has been
	// inserted or the request times out.
	return pollForPath(ctx, db, pollEvery, fullPath, modulePath, requestedVersion)
}

// pollForPath polls the database until a row for fullPath is found.
func pollForPath(ctx context.Context, db *postgres.DB, pollEvery time.Duration,
	fullPath, modulePath, requestedVersion string) *fetchResult {
	fr := &fetchResult{modulePath: modulePath}
	defer derrors.Wrap(&fr.err, "pollForRedirectURL(%q, %q, %q)", modulePath, fullPath, requestedVersion)
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// The request timed out before the fetch process completed.
			fr.status = http.StatusRequestTimeout
			fr.err = ctx.Err()
			return fr
		case <-ticker.C:
			ctx2, cancel := context.WithTimeout(ctx, pollEvery)
			defer cancel()
			fr = checkForPath(ctx2, db, fullPath, modulePath, requestedVersion)
			if fr.status != http.StatusProcessing {
				return fr
			}
		}
	}
}

// checkForPath checks for the existence of fullPath, modulePath, and
// requestedVersion in the database. If the modulePath does not exist in
// version_map, it returns errModuleNotInVersionMap, signaling that the fetch
// process that was initiated is not yet complete.  If the row exists version_map
// but not paths, it means that a module was found at the requestedVersion, but
// not the fullPath, so errPathDoesNotExistInModule is returned.
func checkForPath(ctx context.Context, db *postgres.DB, fullPath, modulePath, requestedVersion string) (fr *fetchResult) {
	defer func() {
		// Based on
		// https://github.com/lib/pq/issues/577#issuecomment-298341053, it seems
		// that ctx.Error() will return nil because this error is coming from
		// postgres. This is also how github.com/lib/pq currently handles the
		// error in their tests:
		// https://github.com/lib/pq/blob/e53edc9b26000fec4c4e357122d56b0f66ace6ea/go18_test.go#L89
		if fr.err != nil && strings.Contains(fr.err.Error(), "pq: canceling statement due to user request") {
			fr.err = fmt.Errorf("%v: %w", fr.err, context.DeadlineExceeded)
			fr.status = http.StatusRequestTimeout
		}
		derrors.Wrap(&fr.err, "checkForPath(%q, %q, %q)", fullPath, modulePath, requestedVersion)
	}()

	// Check the version_map table to see if a row exists for modulePath and
	// requestedVersion.
	// TODO(golang/go#37002): update db.GetVersionMap to return updated_at,
	// so that we can determine if a module version is stale.
	vm, err := db.GetVersionMap(ctx, modulePath, requestedVersion)
	if err != nil {
		// If an error is returned, there are two possibilities:
		// (1) A row for this modulePath and version does not exist.
		// This means that the fetch request is not done yet, so return
		// http.StatusProcessing so the fetchHandler will call checkForPath
		// again in a few seconds.
		// (2) Something went wrong, so return that error.
		fr = &fetchResult{
			modulePath: modulePath,
			status:     derrors.ToHTTPStatus(err),
			err:        err,
		}
		if errors.Is(err, derrors.NotFound) {
			fr.status = http.StatusProcessing
		}
		return fr
	}

	// We successfully retrieved a row in version_map for the modulePath and
	// requestedVersion. Look at the status of that row to determine whether
	// an error should be returned.
	fr = &fetchResult{
		modulePath: modulePath,
		status:     vm.Status,
		goModPath:  vm.GoModPath,
	}
	switch fr.status {
	case http.StatusNotFound:
		// The version_map indicates that the proxy returned a 404/410.
		fr.err = errModuleDoesNotExist
		return fr
	case derrors.ToHTTPStatus(derrors.AlternativeModule):
		// The row indicates that the provided module path did not match the
		// module path returned by a request to
		// /<modulePath>/@v/<requestedPath>.mod.
		fr.err = derrors.AlternativeModule
		return fr
	default:
		// The module was marked for reprocessing by the worker.
		// Return http.StatusProcessing here, so that the tasks gets enqueued
		// to frontend tasks, and we don't return a result to the user until
		// that is complete.
		// TODO(golang/go#37002): mark versions for reprocessing in version_map
		// inside postgres.UpdateModuleVersionStatesForReprocessing.
		if fr.status >= derrors.ToHTTPStatus(derrors.ReprocessStatusOK) {
			fr.status = http.StatusProcessing
		}
		// All remaining non-200 statuses will be in the 40x range.
		// In that case, just return a not found error.
		if fr.status > 400 {
			fr.status = http.StatusNotFound
			fr.err = errModuleDoesNotExist
			return
		}
	}

	// The row in version_map indicated that the module version exists (status
	// was 200 or 290).  Now check the paths table to see if the fullPath exists.
	// vm.status for the module version was either a 200 or 290. Now determine if
	// the fullPath exists in that module.
	if _, _, _, err := db.GetPathInfo(ctx, fullPath, modulePath, vm.ResolvedVersion); err != nil {
		if errors.Is(err, derrors.NotFound) {
			// The module version exists, but the fullPath does not exist in
			// that module version.
			fr.err = errPathDoesNotExistInModule
			fr.status = http.StatusNotFound
			return fr
		}
		// Something went wrong when we made the DB request to ds.GetPathInfo.
		fr.status = http.StatusInternalServerError
		fr.err = err
		return fr
	}
	// Success! The fullPath exists in the requested module version.
	fr.status = http.StatusOK
	return fr
}

// modulePathsToFetch returns the slice of module paths that we should check
// for the path. If modulePath is known, only check that modulePath. If a row
// for the fullPath already exists, check that modulePath. Otherwise, check all
// possible module paths based on the elements for the fullPath.
// Resulting paths are returned in reverse length order.
func modulePathsToFetch(ctx context.Context, ds internal.DataSource, fullPath, modulePath string) (_ []string, err error) {

	defer derrors.Wrap(&err, "modulePathsToFetch(ctx, ds, %q, %q)", fullPath, modulePath)
	if modulePath != internal.UnknownModulePath {
		return []string{modulePath}, nil
	}

	modulePath, _, _, err = ds.GetPathInfo(ctx, fullPath, modulePath, internal.LatestVersion)
	if err != nil && !errors.Is(err, derrors.NotFound) {
		return nil, &serverError{
			status: http.StatusInternalServerError,
			err:    fmt.Errorf("fetchModuleForPath: %v", err),
		}
	}
	if err == nil {
		return []string{modulePath}, nil
	}
	return candidateModulePaths(fullPath)
}

var vcsHostsWithThreeElementRepoName = map[string]bool{
	"bitbucket.org": true,
	"github.com":    true,
	"gitlab.com":    true,
}

// candidateModulePaths returns the potential module paths that could contain
// the fullPath. The paths are returned in reverse length order.
func candidateModulePaths(fullPath string) (_ []string, err error) {
	var (
		path        string
		modulePaths []string
	)
	parts := strings.Split(fullPath, "/")
	if _, ok := vcsHostsWithThreeElementRepoName[parts[0]]; ok {
		if len(parts) < 3 {
			return nil, &serverError{
				status: http.StatusBadRequest,
				err:    fmt.Errorf("invalid path"),
			}
		}
		path = strings.Join(parts[0:2], "/") + "/"
		parts = parts[2:]
	}
	for _, part := range parts {
		path += part
		modulePaths = append([]string{path}, modulePaths...)
		path += "/"
	}
	return modulePaths, nil
}

// FetchAndUpdateState is used by the InMemory queue for testing in
// internal/frontend and running cmd/frontend locally. It is a copy of
// worker.FetchAndUpdateState that does not update module_version_states, so that
// we don't have to import internal/worker here. It is not meant to be used
// when running on AppEngine.
func FetchAndUpdateState(ctx context.Context, modulePath, requestedVersion string, proxyClient *proxy.Client, sourceClient *source.Client, db *postgres.DB, _ string) (_ int, err error) {
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
		log.Infof(ctx, "FetchAndUpdateState(%q, %q): db.InsertModule succeeded", modulePath, requestedVersion)
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

func isActiveFrontendFetch(ctx context.Context) bool {
	return experiment.IsActive(ctx, internal.ExperimentFrontendFetch) &&
		experiment.IsActive(ctx, internal.ExperimentInsertDirectories)
}

func recordFrontendFetchMetric(status int, version string, latency time.Duration) {
	l := float64(latency) / float64(time.Millisecond)

	// Tag versions based on latest, master and semver.
	v := version
	if semver.IsValid(v) {
		v = "semver"
	}
	stats.RecordWithTags(context.Background(), []tag.Mutator{
		tag.Upsert(keyFrontendFetchStatus, strconv.Itoa(status)),
		tag.Upsert(keyFrontendFetchVersion, v),
	}, keyFrontendFetchLatency.M(l))
}
