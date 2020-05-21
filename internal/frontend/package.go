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

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
)

// handlePackageDetails handles requests for package details pages. It expects
// paths of the form "/<path>[@<version>?tab=<tab>]".
func (s *Server) servePackageDetails(w http.ResponseWriter, r *http.Request) error {
	pkgPath, modulePath, version, err := parseDetailsURLPath(r.URL.Path)
	if err != nil {
		return &serverError{
			status: http.StatusBadRequest,
			err:    fmt.Errorf("handlePackageDetails: %v", err),
		}
	}
	return s.servePackagePage(w, r, pkgPath, modulePath, version)
}

// handlePackageDetailsRedirect redirects all redirects to "/pkg" to "/".
func (s *Server) handlePackageDetailsRedirect(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/pkg")
	http.Redirect(w, r, urlPath, http.StatusMovedPermanently)
}

// servePackagePage serves details pages for the package with import path
// pkgPath, in the module specified by modulePath and version.
func (s *Server) servePackagePage(w http.ResponseWriter, r *http.Request, pkgPath, modulePath, version string) error {
	ctx := r.Context()
	if err := checkPathAndVersion(ctx, s.ds, pkgPath, version); err != nil {
		return err
	}

	if experiment.IsActive(ctx, internal.ExperimentUseDirectories) {
		return s.servePackagePageNew(w, r, pkgPath, modulePath, version)
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
		return s.servePackagePageWithPackage(ctx, w, r, pkg, version)
	}
	if !errors.Is(err, derrors.NotFound) {
		return err
	}
	if version == internal.LatestVersion {
		// If we've already checked the latest version, then we know that this path
		// is not a package at any version, so just skip ahead and serve the
		// directory page.
		return s.serveDirectoryPage(w, r, pkgPath, modulePath, version)
	}
	dir, err := s.ds.GetDirectory(ctx, pkgPath, modulePath, version, internal.AllFields)
	if err == nil {
		return s.serveDirectoryPageWithDirectory(ctx, w, r, dir, version)
	}
	if !errors.Is(err, derrors.NotFound) {
		// The only error we expect is NotFound, so serve an 500 here, otherwise
		// whatever response we resolve below might be inconsistent or misleading.
		return fmt.Errorf("checking for directory: %v", err)
	}
	_, err = s.ds.GetPackage(ctx, pkgPath, modulePath, internal.LatestVersion)
	if err == nil {
		return &serverError{
			status: http.StatusNotFound,
			epage: &errorPage{
				Message: fmt.Sprintf("Package %s@%s is not available.", pkgPath, displayVersion(version, modulePath)),
				SecondaryMessage: template.HTML(
					fmt.Sprintf(`There are other versions of this package that are! To view them, `+
						`<a href="/%s?tab=versions">click here</a>.`,
						pkgPath)),
			},
		}
	}
	if !errors.Is(err, derrors.NotFound) {
		// Unlike the error handling for GetDirectory above, we don't serve an
		// InternalServerError here. The reasoning for this is that regardless of
		// the result of GetPackage(..., "latest"), we're going to serve a NotFound
		// response code. So the semantics of the endpoint are the same whether or
		// not we get an unexpected error from GetPackage -- we just don't serve a
		// more informative error response.
		log.Errorf(ctx, "error checking for latest package: %v", err)
		return nil
	}
	return pathNotFoundError("package")
}

func (s *Server) servePackagePageWithPackage(ctx context.Context, w http.ResponseWriter, r *http.Request, pkg *internal.VersionedPackage, requestedVersion string) error {
	pkgHeader, err := createPackage(&pkg.Package, &pkg.ModuleInfo, requestedVersion == internal.LatestVersion)
	if err != nil {
		return fmt.Errorf("creating package header for %s@%s: %v", pkg.Path, pkg.Version, err)
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
		return nil
	}
	canShowDetails := pkg.Package.IsRedistributable || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForPackage(ctx, r, tab, s.ds, pkg)
		if err != nil {
			return fmt.Errorf("fetching page for %q: %v", tab, err)
		}
	}
	page := &DetailsPage{
		basePage: s.newBasePage(r, packageHTMLTitle(&pkg.Package)),
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
	return nil
}

func (s *Server) servePackagePageNew(w http.ResponseWriter, r *http.Request, fullPath, inModulePath, inVersion string) (err error) {
	defer derrors.Wrap(&err, "servePackagePageNew(w, r, %q, %q, %q)", fullPath, inModulePath, inVersion)

	ctx := r.Context()
	modulePath, version, _, err := s.ds.GetPathInfo(ctx, fullPath, inModulePath, inVersion)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			return err
		}
		if inVersion == internal.LatestVersion {
			if !experiment.IsActive(ctx, internal.ExperimentUseDirectories) {
				return pathNotFoundError("package")
			}
			// TODO(b/149933479) add a case for this to TestServer, after we
			// switch over to the paths-based data model.
			path, err := s.stdlibPathForShortcut(ctx, fullPath)
			if path == "" {
				if err != nil {
					// Log the error, but prefer a "path not found" error for a better user experience.
					log.Error(ctx, err)
				}
				return pathNotFoundError("package")
			}
			http.Redirect(w, r, path, http.StatusFound)
			return nil
		}
		// We couldn't find a path at the given version, but if there's one at the latest version
		// we can provide a link to it.
		modulePath, version, _, err = s.ds.GetPathInfo(ctx, fullPath, inModulePath, internal.LatestVersion)
		if err != nil {
			if errors.Is(err, derrors.NotFound) {
				return pathNotFoundError("package")
			}
			return err
		}
		return &serverError{
			status: http.StatusNotFound,
			epage: &errorPage{
				Message: fmt.Sprintf("Package %s@%s is not available.", fullPath, displayVersion(version, modulePath)),
				SecondaryMessage: template.HTML(
					fmt.Sprintf(`There are other versions of this package that are! To view them, `+
						`<a href="/%s?tab=versions">click here</a>.`,
						fullPath)),
			},
		}
	}
	vdir, err := s.ds.GetDirectoryNew(ctx, fullPath, modulePath, version)
	if err != nil {
		return err
	}
	if vdir.Package != nil {
		return s.servePackagePageWithVersionedDirectory(ctx, w, r, vdir, inVersion)
	}
	dir, err := s.ds.GetDirectory(ctx, fullPath, modulePath, version, internal.AllFields)
	if err != nil {
		return err
	}
	return s.serveDirectoryPageWithDirectory(ctx, w, r, dir, inVersion)
}

// stdlibPathForShortcut returns a path in the stdlib that shortcut should redirect to,
// or the empty string if there is no such path.
func (s *Server) stdlibPathForShortcut(ctx context.Context, shortcut string) (path string, err error) {
	defer derrors.Wrap(&err, "stdlibPathForShortcut(ctx, %q)", shortcut)
	if !stdlib.Contains(shortcut) {
		return "", nil
	}
	matches, err := s.ds.GetStdlibPathsWithSuffix(ctx, shortcut)
	if err != nil {
		return "", err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	// No matches, or ambiguous.
	return "", nil
}

func (s *Server) servePackagePageWithVersionedDirectory(ctx context.Context, w http.ResponseWriter, r *http.Request, vdir *internal.VersionedDirectory, requestedVersion string) error {

	pkgHeader, err := createPackageNew(vdir, requestedVersion == internal.LatestVersion)
	if err != nil {
		return fmt.Errorf("creating package header for %s@%s: %v", vdir.Path, vdir.Version, err)
	}

	tab := r.FormValue("tab")
	settings, ok := packageTabLookup[tab]
	if !ok {
		var tab string
		if vdir.DirectoryNew.IsRedistributable {
			tab = "doc"
		} else {
			tab = "overview"
		}
		http.Redirect(w, r, fmt.Sprintf(r.URL.Path+"?tab=%s", tab), http.StatusFound)
		return nil
	}
	canShowDetails := vdir.DirectoryNew.IsRedistributable || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForVersionedDirectory(ctx, r, tab, s.ds, vdir)
		if err != nil {
			return fmt.Errorf("fetching page for %q: %v", tab, err)
		}
	}
	page := &DetailsPage{
		basePage: s.newBasePage(r, packageHTMLTitleNew(vdir.Package)),
		Title:    packageTitleNew(vdir.Package),
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
	return nil
}
