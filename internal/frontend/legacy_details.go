// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
)

// legacyFetchDocumentationDetails returns a DocumentationDetails constructed
// from pkg.
func legacyFetchDocumentationDetails(pkg *internal.LegacyVersionedPackage) *DocumentationDetails {
	return &DocumentationDetails{
		GOOS:          pkg.GOOS,
		GOARCH:        pkg.GOARCH,
		Documentation: pkg.DocumentationHTML,
	}
}

// legacyFetchPackageOverviewDetails uses data for the given package to return
// an OverviewDetails.
func legacyFetchPackageOverviewDetails(ctx context.Context, pkg *internal.LegacyVersionedPackage, versionedLinks bool) (*OverviewDetails, error) {
	od, err := constructOverviewDetails(ctx, &pkg.ModuleInfo, &internal.Readme{Filepath: pkg.LegacyReadmeFilePath, Contents: pkg.LegacyReadmeContents},
		pkg.LegacyPackage.IsRedistributable, versionedLinks)
	if err != nil {
		return nil, err
	}
	od.PackageSourceURL = pkg.SourceInfo.DirectoryURL(packageSubdir(pkg.Path, pkg.ModulePath))
	if !pkg.LegacyPackage.IsRedistributable {
		od.Redistributable = false
	}
	return od, nil
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

// legacyFetchPackageVersionsDetails builds a version hierarchy for all module
// versions containing a package path with v1 import path matching the given v1 path.
func legacyFetchPackageVersionsDetails(ctx context.Context, ds internal.DataSource, pkgPath, v1Path, modulePath string) (*VersionsDetails, error) {
	versions, err := ds.LegacyGetTaggedVersionsForPackageSeries(ctx, pkgPath)
	if err != nil {
		return nil, err
	}
	// If no tagged versions for the package series are found, fetch the
	// pseudo-versions instead.
	if len(versions) == 0 {
		versions, err = ds.LegacyGetPsuedoVersionsForPackageSeries(ctx, pkgPath)
		if err != nil {
			return nil, err
		}
	}

	linkify := func(mi *internal.ModuleInfo) string {
		// Here we have only version information, but need to construct the full
		// import path of the package corresponding to this version.
		var versionPath string
		if mi.ModulePath == stdlib.ModulePath {
			versionPath = pkgPath
		} else {
			versionPath = pathInVersion(v1Path, mi)
		}
		return constructPackageURL(versionPath, mi.ModulePath, linkVersion(mi.Version, mi.ModulePath))
	}
	return buildVersionDetails(modulePath, versions, linkify), nil
}

// legacyFetchPackageLicensesDetails fetches license data for the package version specified by
// path and version from the database and returns a LicensesDetails.
func legacyFetchPackageLicensesDetails(ctx context.Context, ds internal.DataSource, pkgPath, modulePath, resolvedVersion string) (*LicensesDetails, error) {
	dsLicenses, err := ds.LegacyGetPackageLicenses(ctx, pkgPath, modulePath, resolvedVersion)
	if err != nil {
		return nil, err
	}
	return &LicensesDetails{Licenses: transformLicenses(modulePath, resolvedVersion, dsLicenses)}, nil
}
