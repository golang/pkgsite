// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"
)

// DetailsPage contains data for a package of module details template.
type DetailsPage struct {
	basePage
	Title          string
	CanShowDetails bool
	Settings       TabSettings
	Details        interface{}
	Header         interface{}
	BreadcrumbPath template.HTML
	Tabs           []TabSettings

	// PageType is either "mod", "dir", or "pkg" depending on the details
	// handler.
	PageType string
}

func (s *Server) handleDetails(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.staticPageHandler("index.tmpl", "go.dev")(w, r)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "@", 2)
	if stdlib.Contains(parts[0]) {
		s.handleStdLib(w, r)
		return
	}
	s.handlePackageDetails(w, r)
}

// handlePackageDetails handles requests for package details pages. It expects
// paths of the form "/<path>[@<version>?tab=<tab>]".
func (s *Server) handlePackageDetails(w http.ResponseWriter, r *http.Request) {
	pkgPath, modulePath, version, err := parseDetailsURLPath(r.URL.Path)
	if err != nil {
		log.Errorf("handlePackageDetails: %v", err)
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

// handleModuleDetails handles requests for non-stdlib module details pages. It
// expects paths of the form "/mod/<module-path>[@<version>?tab=<tab>]".
// stdlib module pages are handled at "/std".
func (s *Server) handleModuleDetails(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/mod")
	path, _, version, err := parseDetailsURLPath(urlPath)
	if err != nil {
		log.Infof("handleModuleDetails: %v", err)
		s.serveErrorPage(w, r, http.StatusBadRequest, nil)
		return
	}
	s.serveModulePage(w, r, path, version)
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
	if !xerrors.Is(err, derrors.NotFound) {
		log.Error(err)
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
	if !xerrors.Is(err, derrors.NotFound) {
		// The only error we expect is NotFound, so serve an 500 here, otherwise
		// whatever response we resolve below might be inconsistent or misleading.
		log.Errorf("error checking for directory: %v", err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}
	_, err = s.ds.GetPackage(ctx, pkgPath, modulePath, internal.LatestVersion)
	if err == nil {
		epage := &errorPage{
			Message: fmt.Sprintf("Package %s@%s is not available.", pkgPath, displayVersion(version, modulePath)),
			SecondaryMessage: template.HTML(
				fmt.Sprintf(`There are other versions of this package that are! To view them, `+
					`<a href="/%s?tab=versions">click here</a>.</p>`,
					pkgPath)),
		}
		s.serveErrorPage(w, r, http.StatusNotFound, epage)
		return
	}
	if !xerrors.Is(err, derrors.NotFound) {
		// Unlike the error handling for GetDirectory above, we don't serve an
		// InternalServerError here. The reasoning for this is that regardless of
		// the result of GetPackage(..., "latest"), we're going to serve a NotFound
		// response code. So the semantics of the endpoint are the same whether or
		// not we get an unexpected error from GetPackage -- we just don't serve a
		// more informative error response.
		log.Errorf("error checking for latest package: %v", err)
	}
	s.serveErrorPage(w, r, http.StatusNotFound, nil)
}

func (s *Server) servePackagePageWithPackage(ctx context.Context, w http.ResponseWriter, r *http.Request, pkg *internal.VersionedPackage, requestedVersion string) {

	pkgHeader, err := createPackage(&pkg.Package, &pkg.VersionInfo, requestedVersion == internal.LatestVersion)
	if err != nil {
		log.Errorf("error creating package header for %s@%s: %v", pkg.Path, pkg.Version, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}

	tab := r.FormValue("tab")
	settings, ok := packageTabLookup[tab]
	if !ok {
		var tab string
		if pkg.IsRedistributable() {
			tab = "doc"
		} else {
			tab = "overview"
		}
		http.Redirect(w, r, fmt.Sprintf(r.URL.Path+"?tab=%s", tab), http.StatusFound)
		return
	}
	canShowDetails := pkg.IsRedistributable() || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForPackage(ctx, r, tab, s.ds, pkg)
		if err != nil {
			log.Errorf("error fetching page for %q: %v", tab, err)
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
	s.servePage(w, settings.TemplateName, page)
}

// serveModulePage serves details pages for the module specified by modulePath
// and version.
func (s *Server) serveModulePage(w http.ResponseWriter, r *http.Request, modulePath, version string) {
	ctx := r.Context()
	if code, epage := checkPathAndVersion(ctx, s.ds, modulePath, version); code != http.StatusOK {
		s.serveErrorPage(w, r, code, epage)
		return
	}
	// This function handles top level behavior related to the existence of the
	// requested modulePath@version:
	// TODO: fix
	//   1. If the module version exists, serve it.
	//   2. else if we got any unexpected error, serve a server error
	//   3. else if the error is NotFound, serve the directory page
	//   3. else, we didn't find the module so there are two cases:
	//     a. We don't know anything about this module: just serve a 404
	//     b. We have valid versions for this module path, but `version` isn't
	//        one of them. Serve a 404 but recommend the other versions.
	vi, err := s.ds.GetVersionInfo(ctx, modulePath, version)
	if err == nil {
		s.serveModulePageWithModule(ctx, w, r, vi, version)
		return
	}
	if !xerrors.Is(err, derrors.NotFound) {
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}
	if version != internal.LatestVersion {
		if _, err := s.ds.GetVersionInfo(ctx, modulePath, internal.LatestVersion); err != nil {
			log.Errorf("error checking for latest module: %v", err)
		} else {
			epage := &errorPage{
				Message: fmt.Sprintf("Module %s@%s is not available.", modulePath, displayVersion(version, modulePath)),
				SecondaryMessage: template.HTML(
					fmt.Sprintf(`There are other versions of this module that are! To view them, `+
						`<a href="/mod/%s?tab=versions">click here</a>.</p>`,
						modulePath)),
			}
			s.serveErrorPage(w, r, http.StatusNotFound, epage)
			return
		}
	}
	s.serveErrorPage(w, r, http.StatusNotFound, nil)
}

func (s *Server) serveModulePageWithModule(ctx context.Context, w http.ResponseWriter, r *http.Request, vi *internal.VersionInfo, requestedVersion string) {
	licenses, err := s.ds.GetModuleLicenses(ctx, vi.ModulePath, vi.Version)
	if err != nil {
		log.Errorf("error getting module licenses: %v", err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}

	modHeader := createModule(vi, license.ToMetadatas(licenses), requestedVersion == internal.LatestVersion)
	tab := r.FormValue("tab")
	settings, ok := moduleTabLookup[tab]
	if !ok {
		tab = "overview"
		settings = moduleTabLookup["overview"]
	}
	canShowDetails := modHeader.IsRedistributable || settings.AlwaysShowDetails
	var details interface{}
	if canShowDetails {
		var err error
		details, err = fetchDetailsForModule(ctx, r, tab, s.ds, vi, licenses)
		if err != nil {
			log.Errorf("error fetching page for %q: %v", tab, err)
			s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
			return
		}
	}
	page := &DetailsPage{
		basePage:       newBasePage(r, moduleHTMLTitle(vi.ModulePath)),
		Title:          moduleTitle(vi.ModulePath),
		Settings:       settings,
		Header:         modHeader,
		BreadcrumbPath: breadcrumbPath(modHeader.ModulePath, modHeader.ModulePath, modHeader.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           moduleTabSettings,
		PageType:       "mod",
	}
	s.servePage(w, settings.TemplateName, page)
}

// checkPathAndVersion verifies that the requested path and version are
// acceptable. The given path may be a module or package path.
func checkPathAndVersion(ctx context.Context, ds internal.DataSource, path, version string) (int, *errorPage) {
	if version != internal.LatestVersion && !semver.IsValid(version) {
		return http.StatusBadRequest, &errorPage{
			Message:          fmt.Sprintf("%q is not a valid semantic version.", version),
			SecondaryMessage: suggestedSearch(path),
		}
	}
	excluded, err := ds.IsExcluded(ctx, path)
	if err != nil {
		log.Errorf("error checking excluded path: %v", err)
		return http.StatusInternalServerError, nil
	}
	if excluded {
		// Return NotFound; don't let the user know that the package was excluded.
		return http.StatusNotFound, nil
	}
	return http.StatusOK, nil
}

// parseDetailsURLPath returns the modulePath (if known),
// pkgPath and version specified by urlPath.
// urlPath is assumed to be a valid path following the structure:
//   /<module-path>[@<version>/<suffix>]
//
// If <version> is not specified, internal.LatestVersion is used for the
// version. modulePath can only be determined if <version> is specified.
//
// Leading and trailing slashes in the urlPath are trimmed.
func parseDetailsURLPath(urlPath string) (pkgPath, modulePath, version string, err error) {
	defer derrors.Wrap(&err, "parseDetailsURLPath(%q)", urlPath)

	// This splits urlPath into either:
	//   /<module-path>[/<suffix>]
	// or
	//   /<module-path>, @<version>/<suffix>
	// or
	//  /<module-path>/<suffix>, @<version>
	// TODO(b/140191811) The last URL route should redirect.
	parts := strings.SplitN(urlPath, "@", 2)
	basePath := strings.TrimSuffix(strings.TrimPrefix(parts[0], "/"), "/")
	if len(parts) == 1 {
		modulePath = internal.UnknownModulePath
		version = internal.LatestVersion
		pkgPath = basePath
	} else {
		// Parse the version and suffix from parts[1].
		endParts := strings.Split(parts[1], "/")
		suffix := strings.Join(endParts[1:], "/")
		version = endParts[0]
		if version == internal.LatestVersion {
			return "", "", "", fmt.Errorf("invalid version: %q", version)
		}
		if suffix == "" {
			modulePath = internal.UnknownModulePath
			pkgPath = basePath
		} else {
			modulePath = basePath
			pkgPath = basePath + "/" + suffix
		}
	}
	if err := module.CheckImportPath(pkgPath); err != nil {
		return "", "", "", fmt.Errorf("malformed path %q: %v", pkgPath, err)
	}
	if stdlib.Contains(pkgPath) {
		modulePath = stdlib.ModulePath
	}
	return pkgPath, modulePath, version, nil
}

// LatestVersion returns the latest version of the package or module.
// The linkable form of the version is returned.
// It returns the empty string on error.
// It is intended to be used as an argument to middleware.LatestVersion.
func (s *Server) LatestVersion(ctx context.Context, packagePath, modulePath, pageType string) string {
	v, err := s.latestVersion(ctx, packagePath, modulePath, pageType)
	if err != nil {
		// We get NotFound errors from directories; they clutter the log.
		if !xerrors.Is(err, derrors.NotFound) {
			log.Errorf("GetLatestVersion: %v", err)
		}
		return ""
	}
	return v
}

func (s *Server) latestVersion(ctx context.Context, packagePath, modulePath, pageType string) (_ string, err error) {
	defer derrors.Wrap(&err, "latestVersion(ctx, %q, %q)", modulePath, packagePath)

	var vi *internal.VersionInfo
	switch pageType {
	case "mod":
		vi, err = s.ds.GetVersionInfo(ctx, modulePath, internal.LatestVersion)
		if err != nil {
			return "", err
		}
	case "pkg":
		pkg, err := s.ds.GetPackage(ctx, packagePath, modulePath, internal.LatestVersion)
		if err != nil {
			return "", err
		}
		vi = &pkg.VersionInfo
	default:
		// For directories we don't have a well-defined latest version.
		return "", nil
	}
	return linkVersion(vi.Version, modulePath), nil
}
