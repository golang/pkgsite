// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
)

// DirectoryPage contains data needed to generate a directory template.
type DirectoryPage struct {
	basePage
	*Directory
	BreadcrumbPath template.HTML
}

// DirectoryDetails contains data needed represent the directory view on
// package and module pages.
type DirectoryDetails struct {
	*Directory
	ModulePath string
}

// Directory contains information for an individual directory.
type Directory struct {
	Path     string
	Version  string
	Packages []*Package
}

func (s *Server) serveDirectoryPage(w http.ResponseWriter, r *http.Request, dirPath, version string) {
	var ctx = r.Context()
	page, err := fetchPackagesInDirectory(ctx, s.ds, dirPath, version)
	if err != nil {
		status := derrors.ToHTTPStatus(err)
		if status == http.StatusInternalServerError {
			log.Errorf("serveDirectoryPage(w, r, %q, %q): %v", dirPath, version, err)
		}
		s.serveErrorPage(w, r, status, nil)
		return
	}
	page.basePage = newBasePage(r, dirPath)
	s.servePage(w, "directory.tmpl", page)
}

// fetchPackagesInDirectory fetches data for the module version specified by
// pkgPath and pkgversion from the database and returns a DirectoryPage.
func fetchPackagesInDirectory(ctx context.Context, ds DataSource, dirPath, version string) (_ *DirectoryPage, err error) {
	defer derrors.Wrap(&err, "fetchPackagesInDirectory(ctx, db, %q, %q)", dirPath, version)

	dbDir, err := ds.GetDirectory(ctx, dirPath, version)
	if err != nil {
		return nil, err
	}
	dir, err := createDirectory(dirPath, dbDir.Version, dbDir.Packages)
	if err != nil {
		return nil, err
	}
	return &DirectoryPage{
		Directory:      dir,
		BreadcrumbPath: breadcrumbPath(dbDir.Path, dbDir.ModulePath, dbDir.Version),
	}, nil
}

// fetchPackageDirectoryDetails fetches all packages in the directory dirPath
// from the database and returns a DirectoryDetails. The package paths returned
// do not include dirPath.
func fetchPackageDirectoryDetails(ctx context.Context, ds DataSource, dirPath string, vi *internal.VersionInfo) (_ *DirectoryDetails, err error) {
	defer derrors.Wrap(&err, "fetchPackageDirectoryDetails(ctx, ds, %v)", vi)

	dbPackages, err := ds.GetPackagesInVersion(ctx, vi.ModulePath, vi.Version)
	if err != nil {
		return nil, err
	}

	var packages []*internal.VersionedPackage
	for _, p := range dbPackages {
		if !strings.HasPrefix(p.Path, dirPath) || p.Path == dirPath {
			// Only include packages that are a subdirectory of dirPath.
			continue
		}
		vp := &internal.VersionedPackage{
			Package:     *p,
			VersionInfo: *vi,
		}
		packages = append(packages, vp)
	}
	dir, err := createDirectory(dirPath, vi.Version, packages)
	if err != nil {
		return nil, err
	}
	return &DirectoryDetails{Directory: dir, ModulePath: vi.ModulePath}, nil
}

// fetchModuleDirectoryDetails fetches all packages in the module version from the
// database and returns a DirectoryDetails.
func fetchModuleDirectoryDetails(ctx context.Context, ds DataSource, vi *internal.VersionInfo) (_ *DirectoryDetails, err error) {
	defer derrors.Wrap(&err, "fetchModuleDirectoryDetails(ctx, ds, %v)", vi)

	dbPackages, err := ds.GetPackagesInVersion(ctx, vi.ModulePath, vi.Version)
	if err != nil {
		return nil, err
	}
	var packages []*internal.VersionedPackage
	for _, p := range dbPackages {
		vp := &internal.VersionedPackage{
			Package:     *p,
			VersionInfo: *vi,
		}
		packages = append(packages, vp)
	}
	dir, err := createDirectory(vi.ModulePath, vi.Version, packages)
	if err != nil {
		return nil, err
	}
	return &DirectoryDetails{Directory: dir, ModulePath: vi.ModulePath}, nil
}

func createDirectory(dirPath, version string, versionedPackages []*internal.VersionedPackage) (_ *Directory, err error) {
	defer derrors.Wrap(&err, "createDirectory(%q, %q, packages)", dirPath, version)

	var packages []*Package
	for _, pkg := range versionedPackages {
		newPkg, err := createPackage(&pkg.Package, &pkg.VersionInfo)
		if err != nil {
			return nil, err
		}
		if pkg.IsRedistributable() {
			newPkg.Synopsis = pkg.Synopsis
		}
		newPkg.Suffix = strings.TrimPrefix(strings.TrimPrefix(pkg.Path, dirPath), "/")
		if newPkg.Suffix == "" {
			newPkg.Suffix = effectiveName(&pkg.Package) + " (root)"
		}
		packages = append(packages, newPkg)
	}

	sort.Slice(packages, func(i, j int) bool { return packages[i].Path < packages[j].Path })
	return &Directory{
		Path:     dirPath,
		Version:  version,
		Packages: packages,
	}, nil
}
