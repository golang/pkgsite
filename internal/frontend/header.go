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
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/mod/module"
)

// Package contains information for an individual package.
type Package struct {
	Module
	Path               string // full import path
	PathAfterDirectory string // for display only; used only for directory
	Synopsis           string
	IsRedistributable  bool
	URL                string // relative to this site
	LatestURL          string // link with latest-version placeholder, relative to this site
	Licenses           []LicenseMetadata
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
func createPackage(pkg *internal.Package, mi *internal.ModuleInfo, latestRequested bool) (_ *Package, err error) {
	defer derrors.Wrap(&err, "createPackage(%v, %v)", pkg, mi)

	if pkg == nil || mi == nil {
		return nil, fmt.Errorf("package and version info must not be nil")
	}

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
		Synopsis:          pkg.Synopsis,
		IsRedistributable: pkg.IsRedistributable,
		Licenses:          transformLicenseMetadata(pkg.Licenses),
		Module:            *m,
		URL:               constructPackageURL(pkg.Path, mi.ModulePath, urlVersion),
		LatestURL:         constructPackageURL(pkg.Path, mi.ModulePath, middleware.LatestVersionPlaceholder),
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
		CommitTime:        elapsedTime(mi.CommitTime),
		IsRedistributable: mi.IsRedistributable,
		Licenses:          transformLicenseMetadata(licmetas),
		URL:               constructModuleURL(mi.ModulePath, urlVersion),
		LatestURL:         constructModuleURL(mi.ModulePath, middleware.LatestVersionPlaceholder),
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

// packageHTMLTitle constructs the details page title for pkg.
// The string will appear in the <title> element (and thus
// the browser tab).
func packageHTMLTitle(pkg *internal.Package) string {
	if pkg.Name != "main" {
		return pkg.Name + " package"
	}
	return effectiveName(pkg) + " command"
}

// packageTitle returns the package title as it will
// appear in the heading at the top of the page.
func packageTitle(pkg *internal.Package) string {
	if pkg.Name != "main" {
		return "package " + pkg.Name
	}
	return "command " + effectiveName(pkg)
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
	// Include the path as a breadcrumb.
	// We also add a "copy" button for the string `import "path"`.
	//
	// We need the 'DetailsHeader-pathInput' input element to copy it to the clipboard.
	// - Its value attribute is delimited with single quotes because the value
	//   contains double quotes.
	// - Setting its type="hidden" doesn't work, so we position it off screen.
	//
	// Inline the svg for the "copy" icon because when it was in a separate file
	// referenced by an img tag, it was loaded asynchronously and the page
	// jerked when it was finally loaded and its height was known.
	f := `<div class="DetailsHeader-breadcrumb">
%s
<button class="ImageButton js-detailsHeaderCopyPath" aria-label="Copy path to clipboard">
  <svg fill="#00add8" width="13px" height="15px" viewBox="0 0 13 15" version="1.1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">
    <!-- Generator: Sketch 58 (84663) - https://sketch.com -->
    <title>Copy path to clipboard</title>
    <desc>Created with Sketch.</desc>
    <g id="Symbols" stroke="none" stroke-width="1" fill-rule="evenodd">
        <g id="go/header-package" transform="translate(-359.000000, -12.000000)">
            <path d="M367,12 L361,12 C359.896,12 359,12.896 359,14 L359,22 C359,23.104 359.896,24 361,24 L361,22 L361,14 L367,14 L369,14 C369,12.896 368.104,12 367,12 L367,12 Z M370,15 L364,15 C362.896,15 362,15.896 362,17 L362,25 C362,26.104 362.896,27 364,27 L370,27 C371.104,27 372,26.104 372,25 L372,17 C372,15.896 371.104,15 370,15 L370,15 Z M364,25 L370,25 L370,17 L364,17 L364,25 Z" id="ic_copy"></path>
        </g>
    </g>
  </svg>
</button>
<input class="DetailsHeader-pathInput js-detailsHeaderPathInput" role="presentation" tabindex="-1" value='%s'>
</div>`

	return template.HTML(fmt.Sprintf(f,
		strings.Join(elems, `<span class="DetailsHeader-breadcrumbDivider">/</span>`),
		`import "`+pkgPath+`"`))
}

// moduleHTMLTitle constructs the <title> contents, for tabs in the browser.
func moduleHTMLTitle(modulePath string) string {
	if modulePath == stdlib.ModulePath {
		return "stdlib"
	}
	return modulePath + " module"
}

// moduleTitle constructs the title that will appear at the top of the module
// page.
func moduleTitle(modulePath string) string {
	if modulePath == stdlib.ModulePath {
		return "Standard library"
	}
	return "module " + modulePath
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
