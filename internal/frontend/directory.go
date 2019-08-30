// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"log"
	"net/http"
	"strings"

	"golang.org/x/discovery/internal/derrors"
)

// DirectoryPage contains data for directory template.
type DirectoryPage struct {
	basePage
	Directory  string
	Version    string
	ModulePath string
	Packages   []*Package
}

func (s *Server) serveDirectoryPage(w http.ResponseWriter, r *http.Request, dirPath, version string) {
	var ctx = r.Context()
	page, err := fetchPackagesInDirectory(ctx, s.ds, dirPath, version)
	if err != nil {
		status := derrors.ToHTTPStatus(err)
		if status == http.StatusInternalServerError {
			log.Printf("serveDirectoryPage(w, r, %q, %q): %v", dirPath, version, err)
		}
		s.serveErrorPage(w, r, status, nil)
		return
	}
	page.basePage = newBasePage(r, dirPath)
	s.servePage(w, "directory.tmpl", page)
}

// fetchPackagesInDirectory fetches data for the module version specified by pkgPath and pkgversion
// from the database and returns a ModuleDetails.
func fetchPackagesInDirectory(ctx context.Context, ds DataSource, dirPath, version string) (_ *DirectoryPage, err error) {
	defer derrors.Wrap(&err, "fetchPackagesInDirectory(ctx, db, %q, %q)", dirPath, version)

	dir, err := ds.GetDirectory(ctx, dirPath, version)
	if err != nil {
		return nil, err
	}

	var packages []*Package
	for _, p := range dir.Packages {
		newPkg, err := createPackage(&p.Package, &p.VersionInfo)
		if err != nil {
			return nil, err
		}
		if p.IsRedistributable() {
			newPkg.Synopsis = p.Synopsis
		}
		newPkg.Suffix = strings.TrimPrefix(strings.TrimPrefix(p.Path, dirPath), "/")
		packages = append(packages, newPkg)
	}
	return &DirectoryPage{
		Directory:  dirPath,
		Packages:   packages,
		ModulePath: dir.ModulePath,
		Version:    dir.Version,
	}, nil
}
