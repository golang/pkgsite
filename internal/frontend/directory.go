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
	Packages      []*Package
	NestedModules []*internal.ModuleInfo
}

// serveDirectoryPage serves a directory view for a directory in a module
// version.
func (s *Server) serveDirectoryPage(ctx context.Context, w http.ResponseWriter, r *http.Request, ds internal.DataSource,
	um *internal.UnitMeta, requestedVersion string) (err error) {
	defer derrors.Wrap(&err, "serveDirectoryPage for %s@%s", um.Path, requestedVersion)
	tab := r.FormValue("tab")
	settings, ok := directoryTabLookup[tab]
	if tab == "" || !ok || settings.Disabled {
		tab = tabSubdirectories
		settings = directoryTabLookup[tab]
	}
	mi := &internal.ModuleInfo{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
		IsRedistributable: um.IsRedistributable,
	}
	header := createDirectoryHeader(um.Path, mi, um.Licenses)
	if requestedVersion == internal.LatestVersion {
		header.URL = constructDirectoryURL(um.Path, um.ModulePath, internal.LatestVersion)
	}
	details, err := fetchDetailsForDirectory(r, tab, ds, um)
	if err != nil {
		return err
	}
	linkver := linkVersion(um.Version, um.ModulePath)
	page := &DetailsPage{
		basePage:         s.newBasePage(r, fmt.Sprintf("%s directory", um.Path)),
		Name:             um.Path,
		Settings:         settings,
		Header:           header,
		Breadcrumb:       breadcrumbPath(um.Path, um.ModulePath, linkver),
		Details:          details,
		CanShowDetails:   true,
		Tabs:             directoryTabSettings,
		PageType:         pageTypeDirectory,
		CanonicalURLPath: constructPackageURL(um.Path, um.ModulePath, linkver),
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
func fetchDirectoryDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta, includeDirPath bool) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "fetchDirectoryDetails(%q, %q, %q, %v, %t)",
		um.Path, um.ModulePath, um.Version, um.Licenses, includeDirPath)

	if includeDirPath && um.Path != um.ModulePath && um.Path != stdlib.ModulePath {
		return nil, fmt.Errorf("includeDirPath can only be set to true if dirPath = modulePath: %w", derrors.InvalidArgument)
	}
	u, err := ds.GetUnit(ctx, um, internal.WithSubdirectories)
	mi := &internal.ModuleInfo{
		ModulePath:        um.ModulePath,
		Version:           um.Version,
		CommitTime:        um.CommitTime,
		IsRedistributable: um.IsRedistributable,
	}
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		header := createDirectoryHeader(um.Path, mi, um.Licenses)
		return &Directory{DirectoryHeader: *header}, nil
	}
	nestedModules, err := ds.GetNestedModules(ctx, um.Path)
	if err != nil {
		return nil, err
	}
	return createDirectory(um.Path, mi, u.Subdirectories, nestedModules, um.Licenses, includeDirPath)
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
func createDirectory(dirPath string, mi *internal.ModuleInfo, pkgMetas []*internal.PackageMeta, nestedModules []*internal.ModuleInfo,
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
		newPkg.Synopsis = pm.Synopsis
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
		NestedModules:   nestedModules,
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
