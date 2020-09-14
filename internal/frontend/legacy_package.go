// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// legacyServePackagePage serves details pages for the package with import path
// pkgPath, in the module specified by modulePath and version.
func (s *Server) legacyServePackagePage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, pkgPath, modulePath, requestedVersion, resolvedVersion string) (err error) {
	ctx := r.Context()

	// This function handles top level behavior related to the existence of the
	// requested pkgPath@version.
	//   1. If a package exists at this version, serve it.
	//   2. If there is a directory at this version, serve it.
	//   3. If there is another version that contains this package path: serve a
	//      404 and suggest these versions.
	//   4. Just serve a 404
	pkg, err := ds.LegacyGetPackage(ctx, pkgPath, modulePath, resolvedVersion)
	if err == nil {
		return s.legacyServePackagePageWithPackage(w, r, ds, pkg, requestedVersion)
	}
	if !errors.Is(err, derrors.NotFound) {
		return err
	}
	if requestedVersion == internal.LatestVersion {
		// If we've already checked the latest version, then we know that this path
		// is not a package at any version, so just skip ahead and serve the
		// directory page.
		dbDir, err := ds.LegacyGetDirectory(ctx, pkgPath, modulePath, resolvedVersion, internal.AllFields)
		if err != nil {
			if errors.Is(err, derrors.NotFound) {
				return pathNotFoundError(ctx, "package", pkgPath, requestedVersion)
			}
			return err
		}
		return s.legacyServeDirectoryPage(ctx, w, r, ds, dbDir, requestedVersion)
	}
	dir, err := ds.LegacyGetDirectory(ctx, pkgPath, modulePath, resolvedVersion, internal.AllFields)
	if err == nil {
		return s.legacyServeDirectoryPage(ctx, w, r, ds, dir, requestedVersion)
	}
	if !errors.Is(err, derrors.NotFound) {
		// The only error we expect is NotFound, so serve an 500 here, otherwise
		// whatever response we resolve below might be inconsistent or misleading.
		return fmt.Errorf("checking for directory: %v", err)
	}
	_, err = ds.LegacyGetPackage(ctx, pkgPath, modulePath, internal.LatestVersion)
	if err == nil {
		return pathFoundAtLatestError(ctx, "package", pkgPath, requestedVersion)
	}
	if !errors.Is(err, derrors.NotFound) {
		// Unlike the error handling for LegacyGetDirectory above, we don't serve an
		// InternalServerError here. The reasoning for this is that regardless of
		// the result of LegacyGetPackage(..., "latest"), we're going to serve a NotFound
		// response code. So the semantics of the endpoint are the same whether or
		// not we get an unexpected error from GetPackage -- we just don't serve a
		// more informative error response.
		log.Errorf(ctx, "error checking for latest package: %v", err)
		return nil
	}
	return pathNotFoundError(ctx, "package", pkgPath, requestedVersion)
}

func (s *Server) legacyServePackagePageWithPackage(w http.ResponseWriter, r *http.Request, ds internal.DataSource, pkg *internal.LegacyVersionedPackage, requestedVersion string) (err error) {
	defer func() {
		if _, ok := err.(*serverError); !ok {
			derrors.Wrap(&err, "legacyServePackagePageWithPackage(w, r, %q, %q, %q)", pkg.Path, pkg.ModulePath, requestedVersion)
		}
	}()
	pkgHeader, err := createPackage(
		packageMetaFromLegacyPackage(&pkg.LegacyPackage),
		&pkg.ModuleInfo,
		requestedVersion == internal.LatestVersion)
	if err != nil {
		return fmt.Errorf("creating package header for %s@%s: %v", pkg.Path, pkg.Version, err)
	}

	settings, err := packageSettings(r.FormValue("tab"))
	if err != nil {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return nil
	}
	canShowDetails := pkg.LegacyPackage.IsRedistributable || settings.AlwaysShowDetails

	var details interface{}
	if canShowDetails {
		var err error
		details, err = legacyFetchDetailsForPackage(r, settings.Name, ds, pkg)
		if err != nil {
			return fmt.Errorf("fetching page for %q: %v", settings.Name, err)
		}
	}

	var (
		pageType = pageTypePackage
		pageName = pkg.Name
	)
	if pkg.Name == "main" {
		pageName = effectiveName(pkg.Path, pkg.Name)
		pageType = pageTypeCommand
	}
	page := &DetailsPage{
		basePage: s.newBasePage(r, packageHTMLTitle(pkg.Path, pkg.Name)),
		Name:     pageName,
		Settings: *settings,
		Header:   pkgHeader,
		Breadcrumb: breadcrumbPath(pkgHeader.Path, pkgHeader.Module.ModulePath,
			pkgHeader.Module.LinkVersion),
		Details:        details,
		CanShowDetails: canShowDetails,
		Tabs:           packageTabSettings,
		PageType:       pageType,
		CanonicalURLPath: constructPackageURL(
			pkg.Path,
			pkg.ModulePath,
			linkVersion(pkg.Version, pkg.ModulePath),
		),
	}
	page.basePage.AllowWideContent = settings.Name == tabDoc
	s.servePage(r.Context(), w, settings.TemplateName, page)
	return nil
}

// packageMetaFromLegacyPackage returns a PackageMeta based on data from a
// LegacyPackage.
func packageMetaFromLegacyPackage(pkg *internal.LegacyPackage) *internal.PackageMeta {
	return &internal.PackageMeta{
		Path:              pkg.Path,
		IsRedistributable: pkg.IsRedistributable,
		Name:              pkg.Name,
		Synopsis:          pkg.Synopsis,
		Licenses:          pkg.Licenses,
	}
}
