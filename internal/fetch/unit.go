// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"path"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
)

// moduleUnits returns all of the units in a given module, along
// with the contents for those units.
func moduleUnits(modulePath, version string,
	pkgs []*goPackage,
	readmes []*internal.Readme,
	d *licenses.Detector) []*internal.Unit {
	pkgLookup := map[string]*goPackage{}
	for _, pkg := range pkgs {
		pkgLookup[pkg.path] = pkg
	}
	dirPaths := unitPaths(modulePath, pkgs)

	readmeLookup := map[string]*internal.Readme{}
	for _, readme := range readmes {
		if path.Dir(readme.Filepath) == "." {
			readmeLookup[modulePath] = readme
		} else if modulePath == stdlib.ModulePath {
			readmeLookup[path.Dir(readme.Filepath)] = readme
		} else {
			readmeLookup[path.Join(modulePath, path.Dir(readme.Filepath))] = readme
		}
	}

	var units []*internal.Unit
	for _, dirPath := range dirPaths {
		suffix := internal.Suffix(dirPath, modulePath)
		if modulePath == stdlib.ModulePath {
			suffix = dirPath
		}
		isRedist, lics := d.PackageInfo(suffix)
		var meta []*licenses.Metadata
		for _, l := range lics {
			meta = append(meta, l.Metadata)
		}
		dir := &internal.Unit{
			UnitMeta: internal.UnitMeta{
				ModulePath:        modulePath,
				Version:           version,
				Path:              dirPath,
				IsRedistributable: isRedist,
				Licenses:          meta,
			},
		}
		if r, ok := readmeLookup[dirPath]; ok {
			dir.Readme = r
		}
		if pkg, ok := pkgLookup[dirPath]; ok {
			dir.Name = pkg.name
			dir.Imports = pkg.imports
			dir.Documentation = &internal.Documentation{
				GOOS:     pkg.goos,
				GOARCH:   pkg.goarch,
				Synopsis: pkg.synopsis,
				HTML:     pkg.documentationHTML,
			}
		}
		units = append(units, dir)
	}
	return units
}

// unitPaths returns the paths for all the units in a module.
func unitPaths(modulePath string, packages []*goPackage) []string {
	shouldContinue := func(p string) bool {
		if modulePath == stdlib.ModulePath {
			return p != "."
		}
		return len(p) > len(modulePath)
	}

	pathSet := map[string]bool{modulePath: true}
	for _, p := range packages {
		for p := p.path; shouldContinue(p); p = path.Dir(p) {
			pathSet[p] = true
		}
	}

	var dirPaths []string
	for d := range pathSet {
		dirPaths = append(dirPaths, d)
	}
	return dirPaths
}
