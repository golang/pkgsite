// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/postgres"
)

// legacyFetchDetailsForModule returns tab details by delegating to the correct detail
// handler.
func legacyFetchDetailsForModule(r *http.Request, tab string, ds internal.DataSource, mi *internal.ModuleInfo, licenses []*licenses.License, readme *internal.Readme) (interface{}, error) {
	ctx := r.Context()
	switch tab {
	case "packages":
		return legacyFetchDirectoryDetails(ctx, ds, mi.ModulePath, mi, licensesToMetadatas(licenses), true)
	case tabLicenses:
		return &LicensesDetails{Licenses: transformLicenses(mi.ModulePath, mi.Version, licenses)}, nil
	case tabVersions:
		return fetchModuleVersionsDetails(ctx, ds, mi.ModulePath)
	case tabOverview:
		return constructOverviewDetails(ctx, mi, readme, mi.IsRedistributable, urlIsVersioned(r.URL))
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// legacyFetchDetailsForDirectory returns tab details by delegating to the correct
// detail handler.
func legacyFetchDetailsForDirectory(r *http.Request, tab string, dir *internal.LegacyDirectory, licenses []*licenses.License) (interface{}, error) {
	switch tab {
	case tabOverview:
		readme := &internal.Readme{Filepath: dir.LegacyReadmeFilePath, Contents: dir.LegacyReadmeContents}
		return constructOverviewDetails(r.Context(), &dir.ModuleInfo, readme, dir.LegacyModuleInfo.IsRedistributable, urlIsVersioned(r.URL))
	case tabSubdirectories:
		// Ideally we would just use fetchDirectoryDetails here so that it
		// follows the same code path as fetchDetailsForModule and
		// fetchDetailsForPackage. However, since we already have the directory
		// and licenses info, it doesn't make sense to call
		// postgres.GetDirectory again.
		return legacyCreateDirectory(dir, licensesToMetadatas(licenses), false)
	case tabLicenses:
		return &LicensesDetails{Licenses: transformLicenses(dir.ModulePath, dir.Version, licenses)}, nil
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}

// legacyFetchDetailsForPackage returns tab details by delegating to the correct detail
// handler.
func legacyFetchDetailsForPackage(r *http.Request, tab string, ds internal.DataSource, pkg *internal.LegacyVersionedPackage) (interface{}, error) {
	ctx := r.Context()
	switch tab {
	case tabDoc:
		return legacyFetchDocumentationDetails(pkg), nil
	case tabVersions:
		return legacyFetchPackageVersionsDetails(ctx, ds, pkg.Path, pkg.V1Path, pkg.ModulePath)
	case tabSubdirectories:
		return legacyFetchDirectoryDetails(ctx, ds, pkg.Path, &pkg.ModuleInfo, pkg.Licenses, false)
	case tabImports:
		return fetchImportsDetails(ctx, ds, pkg.Path, pkg.ModulePath, pkg.Version)
	case tabImportedBy:
		db, ok := ds.(*postgres.DB)
		if !ok {
			// The proxydatasource does not support the imported by page.
			return nil, proxydatasourceNotSupportedErr()
		}
		return fetchImportedByDetails(ctx, db, pkg.Path, pkg.ModulePath)
	case tabLicenses:
		return legacyFetchPackageLicensesDetails(ctx, ds, pkg.Path, pkg.ModulePath, pkg.Version)
	case tabOverview:
		return legacyFetchPackageOverviewDetails(ctx, pkg, urlIsVersioned(r.URL))
	}
	return nil, fmt.Errorf("BUG: unable to fetch details: unknown tab %q", tab)
}
