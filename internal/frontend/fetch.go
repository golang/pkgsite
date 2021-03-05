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

	"github.com/google/safehtml/template"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
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
	pollEvery                   = 1 * time.Second

	// keyFetchStatus is a census tag for frontend fetch status types.
	keyFetchStatus = tag.MustNewKey("frontend-fetch.status")
	// frontendFetchLatency holds observed latency in individual
	// frontend fetch queries.
	frontendFetchLatency = stats.Float64(
		"go-discovery/frontend-fetch/latency",
		"Latency of a frontend fetch request.",
		stats.UnitMilliseconds,
	)

	// FetchLatencyDistribution aggregates frontend fetch request
	// latency by status code.
	FetchLatencyDistribution = &view.View{
		Name:    "go-discovery/frontend-fetch/latency",
		Measure: frontendFetchLatency,
		// Modified from ochttp.DefaultLatencyDistribution to remove high
		// values. Because our unit is seconds rather than milliseconds, the
		// high values are too large (100000 = 27 hours). The main consequence
		// is that the Fetch Latency heatmap on the dashboard is less
		// informative.
		Aggregation: view.Distribution(
			1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100,
			130, 160, 200, 250, 300, 400, 500, 650, 800, 1000,
			30*60, // half hour: the max time an HTTP task can run
			60*60),
		Description: "FrontendFetch latency, by result source query type.",
		TagKeys:     []tag.Key{keyFetchStatus},
	}
	// FetchResponseCount counts frontend fetch responses by response type.
	FetchResponseCount = &view.View{
		Name:        "go-discovery/frontend-fetch/count",
		Measure:     frontendFetchLatency,
		Aggregation: view.Count(),
		Description: "Frontend fetch request count",
		TagKeys:     []tag.Key{keyFetchStatus},
	}
	// statusNotFoundInVersionMap indicates that a row does not exist in
	// version_map for the module_path and requested_version.
	statusNotFoundInVersionMap = 470
)

// serveFetch checks if a requested path and version exists in the database.
// If not, it will enqueuing potential module versions that could contain
// the requested path and version to a task queue, to be fetched by the worker.
// Meanwhile, the request will poll the database until a row is found, or a
// timeout occurs. A status and responseText will be returned based on the
// result of the request.
func (s *Server) serveFetch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) (err error) {
	defer derrors.Wrap(&err, "serveFetch(%q)", r.URL.Path)
	if _, ok := ds.(*postgres.DB); !ok {
		// There's no reason for the proxydatasource to need this codepath.
		return proxydatasourceNotSupportedErr()
	}
	if r.Method != http.MethodPost {
		// If the experiment flag is not on, or the user makes a GET request,
		// treat this as a request for the "fetch" package, which does not
		// exist.
		return &serverError{status: http.StatusNotFound}
	}
	// fetchHander accepts a requests following the same URL format as the
	// detailsHandler.
	urlInfo, err := extractURLPathInfo(strings.TrimPrefix(r.URL.Path, "/fetch"))
	if err != nil {
		return &serverError{status: http.StatusBadRequest}
	}
	if !isSupportedVersion(urlInfo.fullPath, urlInfo.requestedVersion) ||
		// TODO(https://golang.org/issue/39973): add support for fetching the
		// latest and master versions of the standard library.
		(stdlib.Contains(urlInfo.fullPath) && urlInfo.requestedVersion == internal.LatestVersion) {
		return &serverError{status: http.StatusBadRequest}
	}
	status, responseText := s.fetchAndPoll(r.Context(), ds, urlInfo.modulePath, urlInfo.fullPath, urlInfo.requestedVersion)
	if status != http.StatusOK {
		return &serverError{status: status, responseText: responseText}
	}
	return nil
}

type fetchResult struct {
	modulePath   string
	goModPath    string
	status       int
	err          error
	responseText string
}

func (s *Server) fetchAndPoll(ctx context.Context, ds internal.DataSource, modulePath, fullPath, requestedVersion string) (status int, responseText string) {
	start := time.Now()
	defer func() {
		log.Infof(ctx, "fetchAndPoll(ctx, ds, q, %q, %q, %q): status=%d, responseText=%q",
			modulePath, fullPath, requestedVersion, status, responseText)
		recordFrontendFetchMetric(ctx, status, time.Since(start))
	}()

	if !isSupportedVersion(fullPath, requestedVersion) ||
		// TODO(https://golang.org/issue/39973): add support for fetching the
		// latest and master versions of the standard library
		(stdlib.Contains(fullPath) && requestedVersion == internal.LatestVersion) {
		return http.StatusBadRequest, http.StatusText(http.StatusBadRequest)
	}

	// Generate all possible module paths for the fullPath.
	db := ds.(*postgres.DB)
	modulePaths, err := modulePathsToFetch(ctx, db, fullPath, modulePath)
	if err != nil {
		var serr *serverError
		if errors.As(err, &serr) {
			return serr.status, http.StatusText(serr.status)
		}
		log.Errorf(ctx, "fetchAndPoll(ctx, ds, q, %q, %q, %q): %v", modulePath, fullPath, requestedVersion, err)
		return http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
	}
	results := s.checkPossibleModulePaths(ctx, db, fullPath, requestedVersion, modulePaths, true)
	fr, err := resultFromFetchRequest(results, fullPath, requestedVersion)
	if err != nil {
		log.Errorf(ctx, "fetchAndPoll(ctx, ds, q, %q, %q, %q): %v", modulePath, fullPath, requestedVersion, err)
		return http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)
	}
	if fr.status == derrors.ToStatus(derrors.AlternativeModule) {
		fr.status = http.StatusNotFound
	}
	return fr.status, fr.responseText
}

// checkPossibleModulePaths checks all modulePaths at the requestedVersion, to see
// if the fullPath exists. For each module path, it first checks version_map to
// see if we already attempted to fetch the module. If not, and shouldQueue is
// true, it will enqueue the module to the frontend task queue to be fetched.
// checkPossibleModulePaths will then poll the database for each module path,
// until a result is returned or the request times out. If shouldQueue is false,
// it will return the fetchResult, regardless of what the status is.
func (s *Server) checkPossibleModulePaths(ctx context.Context, db *postgres.DB,
	fullPath, requestedVersion string, modulePaths []string, shouldQueue bool) []*fetchResult {
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	results := make([]*fetchResult, len(modulePaths))
	for i, modulePath := range modulePaths {
		wg.Add(1)
		i := i
		modulePath := modulePath
		go func() {
			defer wg.Done()
			start := time.Now()

			// Before enqueuing the module version to be fetched, check if we
			// have already attempted to fetch it in the past. If so, just
			// return the result from that fetch process.
			fr := checkForPath(ctx, db, fullPath, modulePath, requestedVersion, s.taskIDChangeInterval)
			log.Debugf(ctx, "initial checkForPath(ctx, db, %q, %q, %q, %d): status=%d, err=%v", fullPath, modulePath, requestedVersion, s.taskIDChangeInterval, fr.status, fr.err)
			if !shouldQueue || fr.status != statusNotFoundInVersionMap {
				results[i] = fr
				return
			}

			// A row for this modulePath and requestedVersion combination does not
			// exist in version_map. Enqueue the module version to be fetched.
			if _, err := s.queue.ScheduleFetch(ctx, modulePath, requestedVersion, "", false); err != nil {
				fr.err = err
				fr.status = http.StatusInternalServerError
			}
			log.Debugf(ctx, "queued %s@%s to frontend-fetch task queue", modulePath, requestedVersion)

			// After the fetch request is enqueued, poll the database until it has been
			// inserted or the request times out.
			fr = pollForPath(ctx, db, pollEvery, fullPath, modulePath, requestedVersion, s.taskIDChangeInterval)
			logf := log.Infof
			if fr.status == http.StatusInternalServerError {
				logf = log.Errorf
			}
			logf(ctx, "fetched %s@%s for %s: status=%d, err=%v; took %.3fs", modulePath, requestedVersion, fullPath, fr.status, fr.err, time.Since(start).Seconds())
			results[i] = fr
		}()
	}
	wg.Wait()
	return results
}

// resultFromFetchRequest returns the HTTP status code and response
// text from the results of fetching possible module paths for fullPath at the
// requestedVersion. It is assumed the results are sorted in order of
// decreasing modulePath length, so the first result that is not a
// StatusNotFound is returned. If all of the results are 404, but a module
// path was found that shares the path prefix of fullPath, the responseText will
// contain that information. The status and responseText will be displayed to the
// user.
func resultFromFetchRequest(results []*fetchResult, fullPath, requestedVersion string) (_ *fetchResult, err error) {
	defer derrors.Wrap(&err, "resultFromFetchRequest(results, %q, %q)", fullPath, requestedVersion)
	if len(results) == 0 {
		return nil, fmt.Errorf("no results")
	}

	var moduleMatchingPathPrefix string
	for _, fr := range results {
		switch fr.status {
		// Results are in order of longest module path first. Once an
		// appropriate result is found, return. Otherwise, look at the next
		// path.
		case http.StatusOK:
			if fr.err == nil {
				return fr, nil
			}
		case http.StatusRequestTimeout:
			// If the context timed out or was canceled before all of the requests
			// finished, return an error letting the user to check back later. The
			// worker will still be processing the modules in the background.
			fr.responseText = fmt.Sprintf("We're still working on “%s”. Check back in a few minutes!", displayPath(fullPath, requestedVersion))
			return fr, nil
		case http.StatusInternalServerError:
			fr.responseText = "Oops! Something went wrong."
			return fr, nil
		case derrors.ToStatus(derrors.AlternativeModule):
			if err := module.CheckPath(fr.goModPath); err != nil {
				fr.status = http.StatusNotFound
				fr.responseText = fmt.Sprintf(`%q does not have a valid module path (%q).`, fullPath, fr.goModPath)
				return fr, nil
			}
			t := template.Must(template.New("").Parse(`{{.}}`))
			h, err := t.ExecuteToHTML(fmt.Sprintf("%s is not a valid path. Were you looking for “<a href='https://pkg.go.dev/%s'>%s</a>”?",
				displayPath(fullPath, requestedVersion), fr.goModPath, fr.goModPath))
			if err != nil {
				fr.status = http.StatusInternalServerError
				return fr, err
			}
			fr.responseText = h.String()
			return fr, nil
		case derrors.ToStatus(derrors.BadModule):
			// There are 3 categories of 490 errors that we see:
			// - module contains 0 packages
			// - empty commit time
			// - zip.NewReader: zip: not a valid zip file: bad module
			//   (only seen for foo.maxj.us/oops.fossil)
			//
			// Provide a specific message for the first error.
			fr.status = http.StatusNotFound
			p := fullPath
			if requestedVersion != internal.LatestVersion {
				p = fullPath + "@" + requestedVersion
			}
			fr.responseText = fmt.Sprintf("%s could not be processed.", p)
			if fr.err != nil && strings.Contains(fr.err.Error(), fetch.ErrModuleContainsNoPackages.Error()) {
				fr.responseText = fmt.Sprintf("There are no packages in module %s.", p)
			}
			return fr, nil
		}

		// A module was found for a prefix of the path, but the path did not exist
		// in that module. Note the longest possible modulePath in this case, and
		// let the user know that it exists. For example, if the request was for
		// github.com/hashicorp/vault/@master/api, github.com/hashicorp/vault/api
		// does not exist at master, but it does in older versions of
		// github.com/hashicorp/vault.
		if errors.Is(fr.err, errPathDoesNotExistInModule) && moduleMatchingPathPrefix == "" {
			moduleMatchingPathPrefix = fr.modulePath
		}
	}

	fr := results[0]
	if moduleMatchingPathPrefix != "" {
		t := template.Must(template.New("").Parse(`{{.}}`))
		h, err := t.ExecuteToHTML(fmt.Sprintf(`
		    <div class="Error-message">%s could not be found.</div>
		    <div class="Error-message">However, you can view <a href="https://pkg.go.dev/%s">module %s</a>.</div>`,
			displayPath(fullPath, requestedVersion),
			displayPath(moduleMatchingPathPrefix, requestedVersion),
			displayPath(moduleMatchingPathPrefix, requestedVersion),
		))
		if err != nil {
			fr.status = http.StatusInternalServerError
			return fr, err
		}
		fr.status = http.StatusNotFound
		fr.responseText = h.String()
		return fr, nil
	}
	p := fullPath
	if requestedVersion != internal.LatestVersion {
		p = fullPath + "@" + requestedVersion
	}
	fr.status = http.StatusNotFound
	fr.responseText = fmt.Sprintf("%q could not be found.", p)
	return fr, nil
}

func displayPath(path, version string) string {
	if version == internal.LatestVersion {
		return path
	}
	return fmt.Sprintf("%s@%s", path, version)
}

// pollForPath polls the database until a row for fullPath is found.
func pollForPath(ctx context.Context, db *postgres.DB, pollEvery time.Duration,
	fullPath, modulePath, requestedVersion string, taskIDChangeInterval time.Duration) *fetchResult {
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
			fr = checkForPath(ctx2, db, fullPath, modulePath, requestedVersion, taskIDChangeInterval)
			if fr.status != statusNotFoundInVersionMap {
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
func checkForPath(ctx context.Context, db *postgres.DB,
	fullPath, modulePath, requestedVersion string, taskIDChangeInterval time.Duration) (fr *fetchResult) {
	defer func() {
		// Based on
		// https://github.com/lib/pq/issues/577#issuecomment-298341053, it seems
		// that ctx.Err() will return nil because this error is coming from
		// postgres. This is also how github.com/lib/pq currently handles the
		// error in their tests:
		// https://github.com/lib/pq/blob/e53edc9b26000fec4c4e357122d56b0f66ace6ea/go18_test.go#L89
		if ctx.Err() != nil || (fr.err != nil && strings.Contains(fr.err.Error(), "pq: canceling statement due to user request")) {
			fr.err = fmt.Errorf("%v: %w", fr.err, context.DeadlineExceeded)
			fr.status = http.StatusRequestTimeout
		}
		derrors.Wrap(&fr.err, "checkForPath(%q, %q, %q)", fullPath, modulePath, requestedVersion)
	}()

	// Check the version_map table to see if a row exists for modulePath and
	// requestedVersion.
	vm, err := db.GetVersionMap(ctx, modulePath, requestedVersion)
	if err != nil {
		// If an error is returned, there are two possibilities:
		// (1) A row for this modulePath and version does not exist.
		// This means that the fetch request is not done yet, so return
		// statusNotFoundInVersionMap so the fetchHandler will call checkForPath
		// again in a few seconds.
		// (2) Something went wrong, so return that error.
		fr = &fetchResult{
			modulePath: modulePath,
			status:     derrors.ToStatus(err),
			err:        err,
		}
		if errors.Is(err, derrors.NotFound) {
			fr.status = statusNotFoundInVersionMap
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
	case http.StatusNotFound,
		derrors.ToStatus(derrors.DBModuleInsertInvalid),
		http.StatusInternalServerError:
		if time.Since(vm.UpdatedAt) > taskIDChangeInterval {
			// If the duration of taskIDChangeInterval has passed since
			// a module_path was last inserted into version_map with a failed status,
			// treat that data as expired.
			//
			// It is possible that the module has appeared in the Go Module
			// Mirror during that time, the failure was transient, or the
			// error has been fixed but the module version has not yet been
			// reprocessed.
			//
			// Return statusNotFoundInVersionMap here, so that the fetch
			// request will try to fetch this module version again.
			// Since the taskIDChangeInterval has passed, it is now possible to
			// enqueue that module version to the frontend task queue again.
			fr.status = statusNotFoundInVersionMap
			return fr
		}
		if fr.status == http.StatusInternalServerError {
			fr.err = fmt.Errorf("%q: %v", http.StatusText(fr.status), vm.Error)
		} else {
			// The version_map indicates that the proxy returned a 404/410.
			fr.err = errModuleDoesNotExist
		}
		return fr
	case derrors.ToStatus(derrors.AlternativeModule):
		// The row indicates that the provided module path did not match the
		// module path returned by a request to
		// /<modulePath>/@v/<requestedPath>.mod.
		fr.err = derrors.AlternativeModule
		return fr
	default:
		// The module was marked for reprocessing by the worker.
		// Return statusNotFoundInVersionMap here, so that the tasks gets enqueued
		// to frontend tasks, and we don't return a result to the user until
		// that is complete.
		if fr.status >= derrors.ToStatus(derrors.ReprocessStatusOK) {
			fr.status = statusNotFoundInVersionMap
		}
		// All remaining non-200 statuses will be in the 40x range.
		// In that case, just return a not found error.
		if fr.status >= 400 {
			fr.status = http.StatusNotFound
			fr.err = errModuleDoesNotExist
			return
		}
	}

	// The row in version_map indicated that the module version exists (status
	// was 200 or 290).  Now check the paths table to see if the fullPath exists.
	// vm.status for the module version was either a 200 or 290. Now determine if
	// the fullPath exists in that module.
	if _, err := db.GetUnitMeta(ctx, fullPath, modulePath, vm.ResolvedVersion); err != nil {
		if errors.Is(err, derrors.NotFound) {
			// The module version exists, but the fullPath does not exist in
			// that module version.
			fr.err = errPathDoesNotExistInModule
			fr.status = http.StatusNotFound
			return fr
		}
		// Something went wrong when we made the DB request to ds.GetUnitMeta.
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
	um, err := ds.GetUnitMeta(ctx, fullPath, modulePath, internal.LatestVersion)
	if err != nil && !errors.Is(err, derrors.NotFound) {
		return nil, &serverError{
			status: http.StatusInternalServerError,
			err:    err,
		}
	}
	if err == nil {
		return []string{um.ModulePath}, nil
	}
	return candidateModulePaths(fullPath)
}

var vcsHostsWithThreeElementRepoName = map[string]bool{
	"bitbucket.org": true,
	"gitea.com":     true,
	"gitee.com":     true,
	"github.com":    true,
	"gitlab.com":    true,
}

// maxPathsToFetch is the number of modulePaths that are fetched from a single
// fetch request. The longest module path we've seen in our database had 7 path
// elements. maxPathsToFetch is set to 10 as a buffer.
var maxPathsToFetch = 10

// candidateModulePaths returns the potential module paths that could contain
// the fullPath. The paths are returned in reverse length order.
func candidateModulePaths(fullPath string) (_ []string, err error) {
	if !isValidPath(fullPath) {
		return nil, &serverError{
			status: http.StatusBadRequest,
			err:    fmt.Errorf("isValidPath(%q): false", fullPath),
		}
	}
	paths := internal.CandidateModulePaths(fullPath)
	if paths == nil {
		return nil, &serverError{
			status: http.StatusBadRequest,
			err:    fmt.Errorf("invalid path: %q", fullPath),
		}
	}
	if len(paths) > maxPathsToFetch {
		return paths[len(paths)-maxPathsToFetch:], nil
	}
	return paths, nil
}

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
	defer fr.Defer()
	if fr.Error == nil {
		// Only attempt to insert the module into module_version_states if the
		// fetch process was successful.
		if _, err := db.InsertModule(ctx, fr.Module); err != nil {
			fr.Status = http.StatusInternalServerError
			log.Errorf(ctx, "FetchAndUpdateState(%q, %q): db.InsertModule failed: %v", modulePath, requestedVersion, err)
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

func recordFrontendFetchMetric(ctx context.Context, status int, latency time.Duration) {
	l := float64(latency) / float64(time.Millisecond)

	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(keyFetchStatus, strconv.Itoa(status)),
	}, frontendFetchLatency.M(l))
}
