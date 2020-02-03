// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
)

// handlePackageDetails handles requests for package details pages. It expects
// paths of the form "/<path>[@<version>?tab=<tab>]".
func (s *Server) handlePackageDetails(w http.ResponseWriter, r *http.Request) {
	pkgPath, modulePath, version, err := parseDetailsURLPath(r.URL.Path)
	if err != nil {
		log.Errorf(r.Context(), "handlePackageDetails: %v", err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}
	s.servePackagePage(w, r, pkgPath, modulePath, version)
}

// handlePackageDetailsRedirect redirects all redirects to "/pkg" to "/".
func (s *Server) handlePackageDetailsRedirect(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/pkg")
	http.Redirect(w, r, urlPath, http.StatusMovedPermanently)
}

// servePackagePage serves details pages for the package with import path
// pkgPath, in the module specified by modulePath and version.
func (s *Server) servePackagePage(w http.ResponseWriter, r *http.Request, pkgPath, modulePath, version string) {
	ctx := r.Context()

	if code, epage := checkPathAndVersion(ctx, s.ds, pkgPath, version); code != http.StatusOK {
		s.serveErrorPage(w, r, code, epage)
		return
	}
	// This function handles top level behavior related to the existence of the
	// requested pkgPath@version.
	//   1. If a package exists at this version, serve it.
	//   2. If there is a directory at this version, serve it.
	//   3. If there is another version that contains this package path: serve a
	//      404 and suggest these versions.
	//   4. Just serve a 404
	pkg, err := s.ds.GetPackage(ctx, pkgPath, modulePath, version)
	if err == nil {
		s.servePackagePageWithPackage(ctx, w, r, pkg, version)
		return
	}
	if !errors.Is(err, derrors.NotFound) {
		log.Error(ctx, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}
	if version == internal.LatestVersion {
		// If we've already checked the latest version, then we know that this path
		// is not a package at any version, so just skip ahead and serve the
		// directory page.
		s.serveDirectoryPage(w, r, pkgPath, modulePath, version)
		return
	}
	dir, err := s.ds.GetDirectory(ctx, pkgPath, modulePath, version, internal.AllFields)
	if err == nil {
		s.serveDirectoryPageWithDirectory(ctx, w, r, dir, version)
		return
	}
	if !errors.Is(err, derrors.NotFound) {
		// The only error we expect is NotFound, so serve an 500 here, otherwise
		// whatever response we resolve below might be inconsistent or misleading.
		log.Errorf(ctx, "error checking for directory: %v", err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}
	_, err = s.ds.GetPackage(ctx, pkgPath, modulePath, internal.LatestVersion)
	if err == nil {
		epage := &errorPage{
			Message: fmt.Sprintf("Package %s@%s is not available.", pkgPath, displayVersion(version, modulePath)),
			SecondaryMessage: template.HTML(
				fmt.Sprintf(`There are other versions of this package that are! To view them, `+
					`<a href="/%s?tab=versions">click here</a>.`,
					pkgPath)),
		}
		s.serveErrorPage(w, r, http.StatusNotFound, epage)
		return
	}
	if !errors.Is(err, derrors.NotFound) {
		// Unlike the error handling for GetDirectory above, we don't serve an
		// InternalServerError here. The reasoning for this is that regardless of
		// the result of GetPackage(..., "latest"), we're going to serve a NotFound
		// response code. So the semantics of the endpoint are the same whether or
		// not we get an unexpected error from GetPackage -- we just don't serve a
		// more informative error response.
		log.Errorf(ctx, "error checking for latest package: %v", err)
	}
	s.servePathNotFoundErrorPage(w, r, "package")
}

func (s *Server) servePackagePageWithPackage(ctx context.Context, w http.ResponseWriter, r *http.Request, pkg *internal.VersionedPackage, requestedVersion string) {

	pkgHeader, err := createPackage(&pkg.Package, &pkg.VersionInfo, requestedVersion == internal.LatestVersion)
	if err != nil {
		log.Errorf(ctx, "error creating package header for %s@%s: %v", pkg.Path, pkg.Version, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}

	tab := r.FormValue("tab")
	settings, ok := packageTabLookup[tab]
	if !ok {
		var tab string
		if pkg.Package.IsRedistributable {
			tab = "doc"
		} else {
			tab = "overview"
		}
		http.Redirect(w, r, fmt.Sprintf(r.URL.Path+"?tab=%s", tab), http.StatusFound)
		return
	}
	canShowDetails := pkg.Package.IsRedistributable || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForPackage(ctx, r, tab, s.ds, pkg)
		if err != nil {
			log.Errorf(ctx, "error fetching page for %q: %v", tab, err)
			s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
			return
		}
	}
	page := &DetailsPage{
		basePage: newBasePage(r, packageHTMLTitle(&pkg.Package)),
		Title:    packageTitle(&pkg.Package),
		Settings: settings,
		Header:   pkgHeader,
		BreadcrumbPath: breadcrumbPath(pkgHeader.Path, pkgHeader.Module.ModulePath,
			pkgHeader.Module.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           packageTabSettings,
		PageType:       "pkg",
	}
	s.servePage(ctx, w, settings.TemplateName, page)
}
