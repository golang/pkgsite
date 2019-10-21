// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"html/template"
	"path"
	"strings"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/thirdparty/module"
)

// Package contains information for an individual package.
type Package struct {
	Module
	Path              string
	Suffix            string
	Synopsis          string
	IsRedistributable bool
	URL               string
	Licenses          []LicenseMetadata
}

// Module contains information for an individual module.
type Module struct {
	Version           string
	Path              string
	CommitTime        string
	IsRedistributable bool
	URL               string
	Licenses          []LicenseMetadata
}

// createPackage returns a *Package based on the fields of the specified
// internal package and version info.
func createPackage(pkg *internal.Package, vi *internal.VersionInfo) (_ *Package, err error) {
	defer derrors.Wrap(&err, "createPackage(%v, %v)", pkg, vi)

	if pkg == nil || vi == nil {
		return nil, fmt.Errorf("package and version info must not be nil")
	}

	suffix := strings.TrimPrefix(strings.TrimPrefix(pkg.Path, vi.ModulePath), "/")
	if suffix == "" {
		suffix = effectiveName(pkg) + " (root)"
	}

	var modLicenses []*license.Metadata
	for _, lm := range pkg.Licenses {
		if path.Dir(lm.FilePath) == "." {
			modLicenses = append(modLicenses, lm)
		}
	}

	m, err := createModule(vi, modLicenses)
	if err != nil {
		return nil, err
	}
	return &Package{
		Path:              pkg.Path,
		Suffix:            suffix,
		Synopsis:          pkg.Synopsis,
		IsRedistributable: pkg.IsRedistributable(),
		Licenses:          transformLicenseMetadata(pkg.Licenses),
		Module:            *m,
		URL:               constructPackageURL(pkg.Path, vi.ModulePath, m.Version),
	}, nil
}

// createModule returns a *Module based on the fields of the specified
// versionInfo.
func createModule(vi *internal.VersionInfo, licmetas []*license.Metadata) (_ *Module, err error) {
	defer derrors.Wrap(&err, "createModule(%v, %v)", vi, licmetas)

	formattedVersion := vi.Version
	if vi.ModulePath == stdlib.ModulePath {
		formattedVersion, err = stdlib.TagForVersion(vi.Version)
		if err != nil {
			return nil, err
		}
	}
	return &Module{
		Version:           formattedVersion,
		Path:              vi.ModulePath,
		CommitTime:        elapsedTime(vi.CommitTime),
		IsRedistributable: license.AreRedistributable(licmetas),
		Licenses:          transformLicenseMetadata(licmetas),
		URL:               constructModuleURL(vi.ModulePath, formattedVersion),
	}, nil
}

func constructModuleURL(modulePath, formattedVersion string) string {
	url := "/"
	if modulePath != stdlib.ModulePath {
		url += "mod/"
	}
	url += modulePath
	if formattedVersion != internal.LatestVersion {
		url += "@" + formattedVersion
	}
	return url
}

func constructPackageURL(pkgPath, modulePath, formattedVersion string) string {
	if formattedVersion == internal.LatestVersion {
		return "/" + pkgPath
	}
	if pkgPath == modulePath || modulePath == stdlib.ModulePath {
		return fmt.Sprintf("/%s@%s", pkgPath, formattedVersion)
	}
	return fmt.Sprintf("/%s@%s/%s", modulePath, formattedVersion, strings.TrimPrefix(pkgPath, modulePath+"/"))
}

// effectiveName returns either the command name or package name.
func effectiveName(pkg *internal.Package) string {
	if pkg.Name != "main" {
		return pkg.Name
	}
	var prefix string // package path without version
	if pkg.Path[len(pkg.Path)-3:] == "/v1" {
		prefix = pkg.Path[:len(pkg.Path)-3]
	} else {
		prefix, _, _ = module.SplitPathVersion(pkg.Path)
	}
	_, base := path.Split(prefix)
	return base
}

// packageTitle constructs the details page title for pkg.
// The string will appear in the <title> and <h1> element.
func packageTitle(pkg *internal.Package) string {
	if pkg.Name != "main" {
		return "Package " + pkg.Name
	}
	return "Command " + effectiveName(pkg)
}

// breadcrumbPath builds HTML that displays pkgPath as a sequence of links
// to its parents.
// pkgPath is a slash-separated path, and may be a package import path or a directory.
// modPath is the package's module path. This will be a prefix of pkgPath, except
// within the standard library.
// version is the version for the module, or LatestVersion.
//
// See TestBreadcrumbPath for examples.
func breadcrumbPath(pkgPath, modPath, version string) template.HTML {
	if pkgPath == stdlib.ModulePath {
		return template.HTML(`<div class="DetailsHeader-breadcrumb"><span class="DetailsHeader-breadcrumbCurrent">Standard library</span></div>`)
	}

	// Obtain successive prefixes of pkgPath, stopping at modPath,
	// or for the stdlib, at the end.
	minLen := len(modPath) - 1
	if modPath == stdlib.ModulePath {
		minLen = 1
	}
	var dirs []string
	for dir := pkgPath; len(dir) > minLen && len(path.Dir(dir)) < len(dir); dir = path.Dir(dir) {
		dirs = append(dirs, dir)
	}
	// Construct the path elements of the result.
	// They will be in reverse order of dirs.
	elems := make([]string, len(dirs))
	// The first dir is the current page. If it is the only one, leave it
	// as is. Otherwise, use its base. In neither case does it get a link.
	d := dirs[0]
	if len(dirs) > 1 {
		d = path.Base(d)
	}
	elems[len(elems)-1] = fmt.Sprintf(`<span class="DetailsHeader-breadcrumbCurrent">%s</span>`, template.HTMLEscapeString(d))
	// Make all the other parts into links.
	for i := 1; i < len(dirs); i++ {
		href := "/" + dirs[i]
		if version != internal.LatestVersion {
			href += "@" + version
		}
		el := dirs[i]
		if i != len(dirs)-1 {
			el = path.Base(el)
		}
		elems[len(elems)-i-1] = fmt.Sprintf(`<a href="%s">%s</a>`, template.HTMLEscapeString(href), template.HTMLEscapeString(el))
	}
	// Include the path as a breadcrumb, and also a "copy" button for the path.
	// We need the 'path' input element to copy it to the clipboard.
	// Setting its type="hidden" doesn't work, so we position it off screen.
	f := `<div class="DetailsHeader-breadcrumb">
%s
<img id="DetailsHeader-copyPath" role="button" src="/static/img/ic_copy.svg" alt="Copy path to clipboard">
<input id="DetailsHeader-path" value="%s"/>
</div>`

	return template.HTML(fmt.Sprintf(f,
		strings.Join(elems, `<span class="DetailsHeader-breadcrumbDivider">/</span>`),
		pkgPath))
}

// moduleTitle constructs the details page title for pkg.
func moduleTitle(modulePath string) string {
	if modulePath == stdlib.ModulePath {
		return "Standard library"
	}
	return "Module " + modulePath
}

// elapsedTime takes a date and returns returns human-readable,
// relative timestamps based on the following rules:
// (1) 'X hours ago' when X < 6
// (2) 'today' between 6 hours and 1 day ago
// (3) 'Y days ago' when Y < 6
// (4) A date formatted like "Jan 2, 2006" for anything further back
func elapsedTime(date time.Time) string {
	elapsedHours := int(time.Since(date).Hours())
	if elapsedHours == 1 {
		return "1 hour ago"
	} else if elapsedHours < 6 {
		return fmt.Sprintf("%d hours ago", elapsedHours)
	}

	elapsedDays := elapsedHours / 24
	if elapsedDays < 1 {
		return "today"
	} else if elapsedDays == 1 {
		return "1 day ago"
	} else if elapsedDays < 6 {
		return fmt.Sprintf("%d days ago", elapsedDays)
	}

	return date.Format("Jan _2, 2006")
}
