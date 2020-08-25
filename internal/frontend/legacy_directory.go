// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
)

func (s *Server) legacyServeDirectoryPage(ctx context.Context, w http.ResponseWriter, r *http.Request, ds internal.DataSource, dbDir *internal.LegacyDirectory, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "legacyServeDirectoryPage for %s@%s", dbDir.Path, requestedVersion)
	tab := r.FormValue("tab")
	settings, ok := directoryTabLookup[tab]
	if tab == "" || !ok || settings.Disabled {
		tab = tabSubdirectories
		settings = directoryTabLookup[tab]
	}
	licenses, err := ds.LegacyGetModuleLicenses(ctx, dbDir.ModulePath, dbDir.Version)
	if err != nil {
		return err
	}
	header, err := legacyCreateDirectory(dbDir, licensesToMetadatas(licenses), false)
	if err != nil {
		return err
	}
	if requestedVersion == internal.LatestVersion {
		header.URL = constructDirectoryURL(dbDir.Path, dbDir.ModulePath, internal.LatestVersion)
	}

	details, err := legacyFetchDetailsForDirectory(r, tab, dbDir, licenses)
	if err != nil {
		return err
	}
	page := &DetailsPage{
		basePage:       s.newBasePage(r, fmt.Sprintf("%s directory", dbDir.Path)),
		Name:           dbDir.Path,
		Settings:       settings,
		Header:         header,
		Breadcrumb:     breadcrumbPath(dbDir.Path, dbDir.ModulePath, linkVersion(dbDir.Version, dbDir.ModulePath)),
		Details:        details,
		CanShowDetails: true,
		Tabs:           directoryTabSettings,
		PageType:       pageTypeDirectory,
		CanonicalURLPath: constructPackageURL(
			dbDir.Path,
			dbDir.ModulePath,
			linkVersion(dbDir.Version, dbDir.ModulePath),
		),
	}
	s.servePage(ctx, w, settings.TemplateName, page)
	return nil
}

// legacyFetchDirectoryDetails fetches data for the directory specified by path and
// version from the database and returns a Directory.
//
// includeDirPath indicates whether a package is included if its import path is
// the same as dirPath.
// This argument is needed because on the module "Packages" tab, we want to
// display all packages in the module, even if the import path is the same as
// the module path. However, on the package and directory view's
// "Subdirectories" tab, we do not want to include packages whose import paths
// are the same as the dirPath.
func legacyFetchDirectoryDetails(ctx context.Context, ds internal.DataSource, dirPath string, mi *internal.ModuleInfo,
	licmetas []*licenses.Metadata, includeDirPath bool) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "legacyfetchDirectoryDetails(%q, %q, %q, %v)", dirPath, mi.ModulePath, mi.Version, licmetas)

	if includeDirPath && dirPath != mi.ModulePath && dirPath != stdlib.ModulePath {
		return nil, fmt.Errorf("includeDirPath can only be set to true if dirPath = modulePath: %w", derrors.InvalidArgument)
	}

	if dirPath == stdlib.ModulePath {
		pkgs, err := ds.LegacyGetPackagesInModule(ctx, stdlib.ModulePath, mi.Version)
		if err != nil {
			return nil, err
		}
		return legacyCreateDirectory(&internal.LegacyDirectory{
			LegacyModuleInfo: internal.LegacyModuleInfo{ModuleInfo: *mi},
			Path:             dirPath,
			Packages:         pkgs,
		}, licmetas, includeDirPath)
	}

	dbDir, err := ds.LegacyGetDirectory(ctx, dirPath, mi.ModulePath, mi.Version, internal.AllFields)
	if errors.Is(err, derrors.NotFound) {
		return legacyCreateDirectory(&internal.LegacyDirectory{
			LegacyModuleInfo: internal.LegacyModuleInfo{ModuleInfo: *mi},
			Path:             dirPath,
			Packages:         nil,
		}, licmetas, includeDirPath)
	}
	if err != nil {
		return nil, err
	}
	return legacyCreateDirectory(dbDir, licmetas, includeDirPath)
}

// legacyCreateDirectory constructs a *Directory for the given dirPath.
func legacyCreateDirectory(dbDir *internal.LegacyDirectory, licmetas []*licenses.Metadata, includeDirPath bool) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "legacyCreateDirectory(%q, %q, %t)", dbDir.Path, dbDir.Version, includeDirPath)
	var packages []*internal.PackageMeta
	for _, pkg := range dbDir.Packages {
		newPkg := internal.PackageMetaFromLegacyPackage(pkg)
		packages = append(packages, newPkg)
	}
	return createDirectory(dbDir.Path, &dbDir.ModuleInfo, packages, licmetas, includeDirPath)
}
