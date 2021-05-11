// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/safehtml/template"
	"github.com/google/safehtml/template/uncheckedconversions"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/cookie"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
)

// errUnitNotFoundWithoutFetch returns a 404 with instructions to the user on
// how to manually fetch the package. No fetch button is provided. This is used
// for very large modules or modules that previously 500ed.
var errUnitNotFoundWithoutFetch = &serverError{
	status: http.StatusNotFound,
	epage: &errorPage{
		messageTemplate: template.MakeTrustedTemplate(`
					    <h3 class="Error-message">{{.StatusText}}</h3>
					    <p class="Error-message">Check that you entered the URL correctly or try fetching it following the
                        <a href="/about#adding-a-package">instructions here</a>.</p>`),
		MessageData: struct{ StatusText string }{http.StatusText(http.StatusNotFound)},
	},
}

// servePathNotFoundPage serves a 404 page for the requested path, or redirects
// the user to an appropriate location.
func (s *Server) servePathNotFoundPage(w http.ResponseWriter, r *http.Request,
	ds internal.DataSource, fullPath, modulePath, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "servePathNotFoundPage(w, r, %q, %q)", fullPath, requestedVersion)

	db, ok := ds.(*postgres.DB)
	if !ok {
		return proxydatasourceNotSupportedErr()
	}
	ctx := r.Context()

	if stdlib.Contains(fullPath) {
		var path string
		path, err = stdlibPathForShortcut(ctx, db, fullPath)
		if err != nil {
			// Log the error, but prefer a "path not found" error for a
			// better user experience.
			log.Error(ctx, err)
		}
		if path != "" {
			http.Redirect(w, r, fmt.Sprintf("/%s", path), http.StatusFound)
			return
		}
		return &serverError{status: http.StatusNotFound}
	}

	fr, err := previousFetchStatusAndResponse(ctx, db, fullPath, modulePath, requestedVersion)
	if err != nil {
		// If an error occurred, it means that we have never tried to fetch
		// this path before or an error occurred when we tried to
		// gather data about this 404.
		//
		// If the latter, log the error.
		// In either case, give the user the option to fetch that path.
		if !errors.Is(err, derrors.NotFound) {
			log.Error(ctx, err)
		}
		return pathNotFoundError(fullPath, requestedVersion)
	}

	// If we've reached this point, we know that we've seen this path before.
	// Show a relevant page or redirect the use based on the previous fetch
	// response.
	switch fr.status {
	case http.StatusFound, derrors.ToStatus(derrors.AlternativeModule):
		if fr.goModPath == fullPath {
			// The redirectPath and the fullpath are the same. Do not redirect
			// to avoid ending up in a loop.
			return errUnitNotFoundWithoutFetch
		}
		vm, err := db.GetVersionMap(ctx, fr.goModPath, internal.LatestVersion)
		if (err != nil && !errors.Is(err, derrors.NotFound)) ||
			(vm != nil && vm.Status != http.StatusOK) {
			// We attempted to fetch the canonical module path before and were
			// not successful. Do not redirect this request.
			return errUnitNotFoundWithoutFetch
		}
		u := constructUnitURL(fr.goModPath, fr.goModPath, internal.LatestVersion)
		cookie.Set(w, cookie.AlternativeModuleFlash, fullPath, u)
		http.Redirect(w, r, u, http.StatusFound)
		return nil
	case http.StatusInternalServerError:
		return pathNotFoundError(fullPath, requestedVersion)
	default:
		if u := githubPathRedirect(fullPath); u != "" {
			http.Redirect(w, r, u, http.StatusFound)
			return
		}

		// If a module has a status of 404, but s.taskIDChangeInterval has
		// passed, allow the module to be refetched.
		if fr.status == http.StatusNotFound && time.Since(fr.updatedAt) > s.taskIDChangeInterval {
			return pathNotFoundError(fullPath, requestedVersion)
		}

		// Redirect to the search result page for an empty directory that is above nested modules.
		// See https://golang.org/issue/43725 for context.
		nm, err := ds.GetNestedModules(ctx, fullPath)
		if err == nil && len(nm) > 0 {
			http.Redirect(w, r, "/search?q="+url.QueryEscape(fullPath), http.StatusFound)
			return nil
		}
		return &serverError{
			status: fr.status,
			epage: &errorPage{
				messageTemplate: uncheckedconversions.TrustedTemplateFromStringKnownToSatisfyTypeContract(`
					    <h3 class="Error-message">{{.StatusText}}</h3>
					    <p class="Error-message">` + html.UnescapeString(fr.responseText) + `</p>`),
				MessageData: struct{ StatusText string }{http.StatusText(fr.status)},
			},
		}
	}
}

// githubRegexp is regex to match a GitHub URL scheme containing a "/blob" or
// "/tree" element.
var githubRegexp = regexp.MustCompile(`(blob|tree)(/[^/]+)?`)

func githubPathRedirect(fullPath string) string {
	parts := strings.Split(fullPath, "/")
	if len(parts) <= 3 || parts[0] != "github.com" {
		return ""
	}
	m := strings.Split(fullPath, "/"+githubRegexp.FindString(fullPath))
	if len(m) != 2 {
		return ""
	}
	p := m[0]
	if m[1] != "" {
		p = m[0] + m[1]
	}
	return constructUnitURL(p, p, internal.LatestVersion)
}

// pathNotFoundError returns a page with an option on how to
// add a package or module to the site.
func pathNotFoundError(fullPath, requestedVersion string) error {
	if !isSupportedVersion(fullPath, requestedVersion) {
		return invalidVersionError(fullPath, requestedVersion)
	}
	if stdlib.Contains(fullPath) {
		return &serverError{status: http.StatusNotFound}
	}
	path := fullPath
	if requestedVersion != internal.LatestVersion {
		path = fmt.Sprintf("%s@%s", fullPath, requestedVersion)
	}
	return &serverError{
		status: http.StatusNotFound,
		epage: &errorPage{
			templateName: "fetch.tmpl",
			MessageData:  path,
		},
	}
}

// previousFetchStatusAndResponse returns the fetch result from a
// previous fetch of the fullPath and requestedVersion.
func previousFetchStatusAndResponse(ctx context.Context, db *postgres.DB,
	fullPath, modulePath, requestedVersion string) (_ *fetchResult, err error) {
	defer derrors.Wrap(&err, "previousFetchStatusAndResponse(w, r, %q, %q)", fullPath, requestedVersion)

	// Get all candidate module paths for this path.
	paths, err := modulePathsToFetch(ctx, db, fullPath, modulePath)
	if err != nil {
		return nil, err
	}
	// Check if a row exists in the version_map table for the longest candidate
	// path and version.
	//
	// If we have not fetched the path before, a derrors.NotFound will be
	// returned.
	vm, err := db.GetVersionMap(ctx, paths[0], requestedVersion)
	if err != nil {
		return nil, err
	}
	// If the row has been fetched before, and the result was either a 490,
	// 491, or 5xx, return that result, since it is a final state.
	if vm != nil &&
		(vm.Status >= 500 ||
			vm.Status == derrors.ToStatus(derrors.AlternativeModule) ||
			vm.Status == derrors.ToStatus(derrors.BadModule)) {
		return resultFromFetchRequest([]*fetchResult{
			{
				modulePath: vm.ModulePath,
				goModPath:  vm.GoModPath,
				status:     vm.Status,
				err:        errors.New(vm.Error),
			},
		}, fullPath, requestedVersion)
	}

	// Check if the unit path exists at a higher major version.
	// For example, my.module might not exist, but my.module/v3 might.
	// Similarly, my.module/foo might not exist, but my.module/v3/foo might.
	// In either case, the user will be redirected to the highest major version
	// of the path.
	//
	// Do not bother to look for a specific version if this case. If
	// my.module/foo@v2.1.0 was requested, and my.module/foo/v2 exists, just
	// return the latest version of my.module/foo/v2.
	//
	// Only redirect if the majPath returned is different from the fullPath, and
	// the majPath is not at v1. We don't want to redirect my.module/foo/v3 to
	// my.module/foo, or my.module/foo@v1.5.2 to my.module/foo@v1.0.0.
	majPath, maj, err := db.GetLatestMajorPathForV1Path(ctx, fullPath)
	if err != nil && err != derrors.NotFound {
		return nil, err
	}
	if majPath != fullPath && maj != 1 && majPath != "" {
		return &fetchResult{
			modulePath: majPath,
			goModPath:  majPath,
			status:     http.StatusFound,
		}, nil
	}
	vms, err := db.GetVersionMaps(ctx, paths, requestedVersion)
	if err != nil {
		return nil, err
	}
	var fetchResults []*fetchResult
	for _, vm := range vms {
		fr := fetchResultFromVersionMap(vm)
		fetchResults = append(fetchResults, fr)
		if vm.Status == http.StatusOK || vm.Status == 290 {
			fr.err = errPathDoesNotExistInModule
		}
	}
	if len(fetchResults) == 0 {
		return nil, derrors.NotFound
	}
	return resultFromFetchRequest(fetchResults, fullPath, requestedVersion)
}

func fetchResultFromVersionMap(vm *internal.VersionMap) *fetchResult {
	var err error
	if vm.Error != "" {
		err = errors.New(vm.Error)
	}
	return &fetchResult{
		modulePath: vm.ModulePath,
		goModPath:  vm.GoModPath,
		status:     vm.Status,
		updatedAt:  vm.UpdatedAt,
		err:        err,
	}
}
