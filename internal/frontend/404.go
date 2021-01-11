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

	"github.com/google/safehtml/template"
	"github.com/google/safehtml/template/uncheckedconversions"
	"golang.org/x/pkgsite/internal"
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
	ds internal.DataSource, fullPath, requestedVersion string) (err error) {
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

	fr, err := previousFetchStatusAndResponse(ctx, db, fullPath, requestedVersion)
	if err != nil || fr.status == http.StatusInternalServerError {
		if err != nil && !errors.Is(err, derrors.NotFound) {
			log.Error(ctx, err)
		}
		return pathNotFoundError(fullPath, requestedVersion)
	}
	if fr.goModPath != fr.modulePath && fr.status == derrors.ToStatus(derrors.AlternativeModule) {
		http.Redirect(w, r, constructUnitURL(fr.goModPath, fr.goModPath, internal.LatestVersion), http.StatusFound)
		return
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

// previousFetchStatusAndResponse returns the status and response text from a
// previous fetch of the fullPath and requestedVersion.
func previousFetchStatusAndResponse(ctx context.Context, db *postgres.DB, fullPath, requestedVersion string) (_ *fetchResult, err error) {
	defer derrors.Wrap(&err, "fetchRedirectPath(w, r, %q, %q)", fullPath, requestedVersion)

	vm, err := db.GetVersionMap(ctx, fullPath, requestedVersion)
	if err != nil {
		return nil, err
	}
	if vm.Status != http.StatusNotFound {
		return resultFromFetchRequest([]*fetchResult{
			{
				modulePath: vm.ModulePath,
				goModPath:  vm.GoModPath,
				status:     vm.Status,
				err:        errors.New(vm.Error),
			},
		}, fullPath, requestedVersion)
	}

	// If the status is 404, it likely means that the fullPath is not a
	// modulePath. Check all of the candidate module paths for the past result.
	paths, err := candidateModulePaths(fullPath)
	if err != nil {
		return nil, err
	}
	vms, err := db.GetVersionMapsNon2xxStatus(ctx, paths, requestedVersion)
	if err != nil {
		return nil, err
	}
	if len(vms) == 0 {
		return nil, nil
	}
	var fetchResults []*fetchResult
	for _, vm := range vms {
		fetchResults = append(fetchResults, &fetchResult{
			modulePath: vm.ModulePath,
			goModPath:  vm.GoModPath,
			status:     vm.Status,
			err:        errors.New(vm.Error),
		})
	}
	if len(fetchResults) == 0 {
		return nil, derrors.NotFound
	}
	return resultFromFetchRequest(fetchResults, fullPath, requestedVersion)
}
