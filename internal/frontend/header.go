// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"path"
	"strings"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
)

// Package contains information for an individual package.
type Package struct {
	Module
	Path               string // full import path
	URL                string // relative to this site
	LatestURL          string // link with latest-version placeholder, relative to this site
	IsRedistributable  bool
	Licenses           []LicenseMetadata
	PathAfterDirectory string // for display on the directories tab; used by Directory
	Synopsis           string // for display on the directories tab; used by Directory
}

// Module contains information for an individual module.
type Module struct {
	DisplayVersion    string
	LinkVersion       string
	ModulePath        string
	CommitTime        string
	IsRedistributable bool
	URL               string // relative to this site
	LatestURL         string // link with latest-version placeholder, relative to this site
	Licenses          []LicenseMetadata
}

// createPackage returns a *Package based on the fields of the specified
// internal package and version info.
//
// latestRequested indicates whether the user requested the latest
// version of the package. If so, the returned Package.URL will have the
// structure /<path> instead of /<path>@<version>.
func createPackage(pkg *internal.PackageMeta, mi *internal.ModuleInfo, latestRequested bool) (_ *Package, err error) {
	defer derrors.Wrap(&err, "createPackage(%v, %v, %t)", pkg, mi, latestRequested)

	var modLicenses []*licenses.Metadata
	for _, lm := range pkg.Licenses {
		if path.Dir(lm.FilePath) == "." {
			modLicenses = append(modLicenses, lm)
		}
	}

	m := createModule(mi, modLicenses, latestRequested)
	urlVersion := m.LinkVersion
	if latestRequested {
		urlVersion = internal.LatestVersion
	}
	return &Package{
		Path:              pkg.Path,
		IsRedistributable: pkg.IsRedistributable,
		Licenses:          transformLicenseMetadata(pkg.Licenses),
		Module:            *m,
		URL:               constructPackageURL(pkg.Path, mi.ModulePath, urlVersion),
		LatestURL:         constructPackageURL(pkg.Path, mi.ModulePath, middleware.LatestMinorVersionPlaceholder),
	}, nil
}

// createModule returns a *Module based on the fields of the specified
// versionInfo.
//
// latestRequested indicates whether the user requested the latest
// version of the package. If so, the returned Module.URL will have the
// structure /<path> instead of /<path>@<version>.
func createModule(mi *internal.ModuleInfo, licmetas []*licenses.Metadata, latestRequested bool) *Module {
	urlVersion := linkVersion(mi.Version, mi.ModulePath)
	if latestRequested {
		urlVersion = internal.LatestVersion
	}
	return &Module{
		DisplayVersion:    displayVersion(mi.Version, mi.ModulePath),
		LinkVersion:       linkVersion(mi.Version, mi.ModulePath),
		ModulePath:        mi.ModulePath,
		CommitTime:        absoluteTime(mi.CommitTime),
		IsRedistributable: mi.IsRedistributable,
		Licenses:          transformLicenseMetadata(licmetas),
		URL:               constructModuleURL(mi.ModulePath, urlVersion),
		LatestURL:         constructModuleURL(mi.ModulePath, middleware.LatestMinorVersionPlaceholder),
	}
}

func constructModuleURL(modulePath, linkVersion string) string {
	url := "/"
	if modulePath != stdlib.ModulePath {
		url += "mod/"
	}
	url += modulePath
	if linkVersion != internal.LatestVersion {
		url += "@" + linkVersion
	}
	return url
}

func constructPackageURL(pkgPath, modulePath, linkVersion string) string {
	if linkVersion == internal.LatestVersion {
		return "/" + pkgPath
	}
	if pkgPath == modulePath || modulePath == stdlib.ModulePath {
		return fmt.Sprintf("/%s@%s", pkgPath, linkVersion)
	}
	return fmt.Sprintf("/%s@%s/%s", modulePath, linkVersion, strings.TrimPrefix(pkgPath, modulePath+"/"))
}

// packageHTMLTitle constructs the details page title for pkg.
// The string will appear in the <title> element (and thus
// the browser tab).
func packageHTMLTitle(pkgPath, pkgName string) string {
	if pkgName != "main" {
		return pkgName + " package"
	}
	return effectiveName(pkgPath, pkgName) + " command"
}

// moduleHTMLTitle constructs the <title> contents, for tabs in the browser.
func moduleHTMLTitle(modulePath string) string {
	if modulePath == stdlib.ModulePath {
		return "stdlib"
	}
	return modulePath + " module"
}

// absoluteTime takes a date and returns returns a human-readable,
// date with the format mmm d, yyyy:
func absoluteTime(date time.Time) string {
	return date.Format("Jan _2, 2006")
}
