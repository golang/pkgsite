// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
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

// legacyCreateDirectory constructs a *Directory for the given dirPath.
func legacyCreateDirectory(dbDir *internal.LegacyDirectory, licmetas []*licenses.Metadata, includeDirPath bool) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "legacyCreateDirectory(%q, %q, %t)", dbDir.Path, dbDir.Version, includeDirPath)
	var packages []*internal.PackageMeta
	for _, pkg := range dbDir.Packages {
		newPkg := packageMetaFromLegacyPackage(pkg)
		packages = append(packages, newPkg)
	}
	return createDirectory(dbDir.Path, &dbDir.ModuleInfo, packages, licmetas, includeDirPath)
}
