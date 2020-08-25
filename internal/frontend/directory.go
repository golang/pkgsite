// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
)

// DirectoryPage contains data needed to generate a directory template.
type DirectoryPage struct {
	basePage
	*Directory
}

// DirectoryHeader contains information for the header on a directory page.
type DirectoryHeader struct {
	Module
	Path string
	URL  string
}

// Directory contains information for an individual directory.
type Directory struct {
	DirectoryHeader
	Packages []*Package
}

// serveDirectoryPage serves a directory view for a directory in a module
// version.
func (s *Server) serveDirectoryPage(ctx context.Context, w http.ResponseWriter, r *http.Request, ds internal.DataSource, vdir *internal.Directory, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "serveDirectoryPage for %s@%s", vdir.Path, requestedVersion)
	tab := r.FormValue("tab")
	settings, ok := directoryTabLookup[tab]
	if tab == "" || !ok || settings.Disabled {
		tab = tabSubdirectories
		settings = directoryTabLookup[tab]
	}
	header := createDirectoryHeader(vdir.Path, &vdir.ModuleInfo, vdir.Licenses)
	if requestedVersion == internal.LatestVersion {
		header.URL = constructDirectoryURL(vdir.Path, vdir.ModulePath, internal.LatestVersion)
	}
	details, err := fetchDetailsForDirectory(r, tab, ds, vdir)
	if err != nil {
		return err
	}
	page := &DetailsPage{
		basePage:       s.newBasePage(r, fmt.Sprintf("%s directory", vdir.Path)),
		Name:           vdir.Path,
		Settings:       settings,
		Header:         header,
		Breadcrumb:     breadcrumbPath(vdir.Path, vdir.ModulePath, linkVersion(vdir.Version, vdir.ModulePath)),
		Details:        details,
		CanShowDetails: true,
		Tabs:           directoryTabSettings,
		PageType:       pageTypeDirectory,
	}
	s.servePage(ctx, w, settings.TemplateName, page)
	return nil
}

// fetchDirectoryDetails fetches data for the directory specified by path and
// version from the database and returns a Directory.
//
// includeDirPath indicates whether a package is included if its import path is
// the same as dirPath.
// This argument is needed because on the module "Packages" tab, we want to
// display all packages in the module, even if the import path is the same as
// the module path. However, on the package and directory view's
// "Subdirectories" tab, we do not want to include packages whose import paths
// are the same as the dirPath.
func fetchDirectoryDetails(ctx context.Context, ds internal.DataSource, vdir *internal.Directory, includeDirPath bool) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "fetchDirectoryDetails(%q, %q, %q, %v)",
		vdir.Path, vdir.ModulePath, vdir.Version, vdir.Licenses)

	db, ok := ds.(*postgres.DB)
	if !ok {
		return nil, proxydatasourceNotSupportedErr()
	}
	if includeDirPath && vdir.Path != vdir.ModulePath && vdir.Path != stdlib.ModulePath {
		return nil, fmt.Errorf("includeDirPath can only be set to true if dirPath = modulePath: %w", derrors.InvalidArgument)
	}
	packages, err := db.GetPackagesInDirectory(ctx, vdir.Path, vdir.ModulePath, vdir.Version)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		header := createDirectoryHeader(vdir.Path, &vdir.ModuleInfo, vdir.Licenses)
		return &Directory{DirectoryHeader: *header}, nil
	}
	return createDirectory(vdir.Path, &vdir.ModuleInfo, packages, vdir.Licenses, includeDirPath)
}

// createDirectory constructs a *Directory for the given dirPath.
//
// includeDirPath indicates whether a package is included if its import path is
// the same as dirPath.
// This argument is needed because on the module "Packages" tab, we want to
// display all packages in the mdoule, even if the import path is the same as
// the module path. However, on the package and directory view's
// "Subdirectories" tab, we do not want to include packages whose import paths
// are the same as the dirPath.
func createDirectory(dirPath string, mi *internal.ModuleInfo, pkgMetas []*internal.PackageMeta,
	licmetas []*licenses.Metadata, includeDirPath bool) (_ *Directory, err error) {
	var packages []*Package
	for _, pm := range pkgMetas {
		if !includeDirPath && pm.Path == dirPath {
			continue
		}
		newPkg, err := createPackage(pm, mi, false)
		if err != nil {
			return nil, err
		}
		newPkg.PathAfterDirectory = internal.Suffix(pm.Path, dirPath)
		if newPkg.PathAfterDirectory == "" {
			newPkg.PathAfterDirectory = effectiveName(pm.Path, pm.Name) + " (root)"
		}
		packages = append(packages, newPkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Path < packages[j].Path })
	header := createDirectoryHeader(dirPath, mi, licmetas)

	return &Directory{
		DirectoryHeader: *header,
		Packages:        packages,
	}, nil
}

func createDirectoryHeader(dirPath string, mi *internal.ModuleInfo, licmetas []*licenses.Metadata) (_ *DirectoryHeader) {
	mod := createModule(mi, licmetas, false)
	return &DirectoryHeader{
		Module: *mod,
		Path:   dirPath,
		URL:    constructDirectoryURL(dirPath, mi.ModulePath, linkVersion(mi.Version, mi.ModulePath)),
	}
}

func constructDirectoryURL(dirPath, modulePath, linkVersion string) string {
	if linkVersion == internal.LatestVersion {
		return fmt.Sprintf("/%s", dirPath)
	}
	if dirPath == modulePath || modulePath == stdlib.ModulePath {
		return fmt.Sprintf("/%s@%s", dirPath, linkVersion)
	}
	return fmt.Sprintf("/%s@%s/%s", modulePath, linkVersion, strings.TrimPrefix(dirPath, modulePath+"/"))
}
