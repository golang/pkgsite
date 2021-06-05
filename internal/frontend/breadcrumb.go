// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"path"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// displayBreadcrumbs appends additional breadcrumb links for display
// to those for the given unit.
func displayBreadcrumb(um *internal.UnitMeta, requestedVersion string) breadcrumb {
	bc := breadcrumbPath(um.Path, um.ModulePath, requestedVersion)
	if um.ModulePath == stdlib.ModulePath && um.Path != stdlib.ModulePath {
		bc.Links = append([]link{{Href: "/std", Body: "Standard library"}}, bc.Links...)
	}
	bc.Links = append([]link{{Href: "/", Body: "Discover Packages"}}, bc.Links...)
	return bc
}

type breadcrumb struct {
	Links    []link
	Current  string
	CopyData string
}

type link struct {
	Href, Body string
}

// breadcrumbPath builds HTML that displays pkgPath as a sequence of links
// to its parents.
// pkgPath is a slash-separated path, and may be a package import path or a directory.
// modPath is the package's module path. This will be a prefix of pkgPath, except
// within the standard library.
// version is the version for the module, or LatestVersion.
//
// See TestBreadcrumbPath for examples.
func breadcrumbPath(pkgPath, modPath, requestedVersion string) breadcrumb {
	if pkgPath == stdlib.ModulePath {
		return breadcrumb{Current: "Standard library"}
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
	// The first dir is the current page. If it is the only one, leave it
	// as is. Otherwise, use its base. In neither case does it get a link.
	d := dirs[0]
	if len(dirs) > 1 {
		d = path.Base(d)
	}
	b := breadcrumb{Current: d}
	// Make all the other parts into links.
	b.Links = make([]link, len(dirs)-1)
	for i := 1; i < len(dirs); i++ {
		href := "/" + dirs[i]
		if requestedVersion != version.LatestVersion {
			href += "@" + linkVersion(requestedVersion, modPath)
		}
		el := dirs[i]
		if i != len(dirs)-1 {
			el = path.Base(el)
		}
		b.Links[len(b.Links)-i] = link{href, el}
	}
	// Add a "copy" button for the path.
	b.CopyData = pkgPath
	return b
}
