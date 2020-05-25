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
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
)

// DirectoryPage contains data needed to generate a directory template.
type DirectoryPage struct {
	basePage
	*Directory
	BreadcrumbPath template.HTML
}

// Directory contains information for an individual directory.
type Directory struct {
	Module
	Path     string
	Packages []*Package
	URL      string
}

// serveDirectoryPage returns a directory view. It is called by
// servePackagePage when an attempt to fetch a package path at any version
// returns a 404.
func (s *Server) serveDirectoryPage(w http.ResponseWriter, r *http.Request, dirPath, modulePath, requestedVersion string) error {
	var ctx = r.Context()

	dbDir, err := s.ds.GetDirectory(ctx, dirPath, modulePath, requestedVersion, internal.AllFields)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return pathNotFoundError(ctx, "package", dirPath, requestedVersion)
		}
		return fmt.Errorf("serveDirectoryPage for %s@%s: %v", dirPath, requestedVersion, err)
	}
	return s.serveDirectoryPageWithDirectory(ctx, w, r, dbDir, requestedVersion)
}

func (s *Server) serveDirectoryPageWithDirectory(ctx context.Context, w http.ResponseWriter, r *http.Request, dbDir *internal.Directory, requestedVersion string) error {
	tab := r.FormValue("tab")
	settings, ok := directoryTabLookup[tab]
	if tab == "" || !ok || settings.Disabled {
		tab = "subdirectories"
		settings = directoryTabLookup[tab]
	}
	licenses, err := s.ds.GetModuleLicenses(ctx, dbDir.ModulePath, dbDir.Version)
	if err != nil {
		return fmt.Errorf("serveDirectoryPage for %s@%s: %v", dbDir.Path, requestedVersion, err)
	}
	header, err := createDirectory(dbDir, licensesToMetadatas(licenses), false)
	if err != nil {
		return fmt.Errorf("serveDirectoryPage for %s@%s: %v", dbDir.Path, requestedVersion, err)
	}
	if requestedVersion == internal.LatestVersion {
		header.URL = constructDirectoryURL(dbDir.Path, dbDir.ModulePath, internal.LatestVersion)
	}

	details, err := constructDetailsForDirectory(r, tab, dbDir, licenses)
	if err != nil {
		return fmt.Errorf("serveDirectoryPage for %s@%s: %v", dbDir.Path, requestedVersion, err)
	}

	page := &DetailsPage{
		basePage:       s.newBasePage(r, fmt.Sprintf("%s directory", dbDir.Path)),
		Title:          fmt.Sprintf("directory %s", dbDir.Path),
		Settings:       settings,
		Header:         header,
		BreadcrumbPath: breadcrumbPath(dbDir.Path, dbDir.ModulePath, linkVersion(dbDir.Version, dbDir.ModulePath)),
		Details:        details,
		CanShowDetails: true,
		Tabs:           directoryTabSettings,
		PageType:       "dir",
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
func fetchDirectoryDetails(ctx context.Context, ds internal.DataSource, dirPath string, mi *internal.ModuleInfo,
	licmetas []*licenses.Metadata, includeDirPath bool) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "s.ds.fetchDirectoryDetails(%q, %q, %q, %v)", dirPath, mi.ModulePath, mi.Version, licmetas)

	if includeDirPath && dirPath != mi.ModulePath && dirPath != stdlib.ModulePath {
		return nil, fmt.Errorf("includeDirPath can only be set to true if dirPath = modulePath: %w", derrors.InvalidArgument)
	}

	if dirPath == stdlib.ModulePath {
		pkgs, err := ds.GetPackagesInModule(ctx, stdlib.ModulePath, mi.Version)
		if err != nil {
			return nil, err
		}
		return createDirectory(&internal.Directory{
			ModuleInfo: *mi,
			Path:       dirPath,
			Packages:   pkgs,
		}, licmetas, includeDirPath)
	}

	dbDir, err := ds.GetDirectory(ctx, dirPath, mi.ModulePath, mi.Version, internal.AllFields)
	if errors.Is(err, derrors.NotFound) {
		return createDirectory(&internal.Directory{
			ModuleInfo: *mi,
			Path:       dirPath,
			Packages:   nil,
		}, licmetas, includeDirPath)
	}
	if err != nil {
		return nil, err
	}
	return createDirectory(dbDir, licmetas, includeDirPath)
}

// createDirectory constructs a *Directory from the provided dbDir and licmetas.
//
// includeDirPath indicates whether a package is included if its import path is
// the same as dirPath.
// This argument is needed because on the module "Packages" tab, we want to
// display all packages in the mdoule, even if the import path is the same as
// the module path. However, on the package and directory view's
// "Subdirectories" tab, we do not want to include packages whose import paths
// are the same as the dirPath.
func createDirectory(dbDir *internal.Directory, licmetas []*licenses.Metadata, includeDirPath bool) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "createDirectory(%q, %q, %t)", dbDir.Path, dbDir.Version, includeDirPath)

	var packages []*Package
	for _, pkg := range dbDir.Packages {
		if !includeDirPath && pkg.Path == dbDir.Path {
			continue
		}
		newPkg, err := createPackage(pkg, &dbDir.ModuleInfo, false)
		if err != nil {
			return nil, err
		}
		if pkg.IsRedistributable {
			newPkg.Synopsis = pkg.Synopsis
		}
		newPkg.PathAfterDirectory = strings.TrimPrefix(strings.TrimPrefix(pkg.Path, dbDir.Path), "/")
		if newPkg.PathAfterDirectory == "" {
			newPkg.PathAfterDirectory = effectiveName(pkg) + " (root)"
		}
		packages = append(packages, newPkg)
	}
	mod := createModule(&dbDir.ModuleInfo, licmetas, false)
	sort.Slice(packages, func(i, j int) bool { return packages[i].Path < packages[j].Path })

	return &Directory{
		Module:   *mod,
		Path:     dbDir.Path,
		Packages: packages,
		URL:      constructDirectoryURL(dbDir.Path, dbDir.ModulePath, linkVersion(dbDir.Version, dbDir.ModulePath)),
	}, nil
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
