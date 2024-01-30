// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"path"
	"sort"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
)

// moduleUnit returns the requested unit in a given module, along
// with the contents for the unit.
func moduleUnit(modulePath string, unitMeta *internal.UnitMeta,
	pkg *goPackage,
	readme *internal.Readme,
	d *licenses.Detector) *internal.Unit {

	suffix := internal.Suffix(unitMeta.Path, modulePath)
	if modulePath == stdlib.ModulePath {
		suffix = unitMeta.Path
	}
	isRedist, lics := d.PackageInfo(suffix)
	var meta []*licenses.Metadata
	for _, l := range lics {
		meta = append(meta, l.Metadata)
	}
	unit := &internal.Unit{
		UnitMeta:          *unitMeta,
		Licenses:          meta,
		IsRedistributable: isRedist,
	}
	if readme != nil {
		unit.Readme = readme
	}
	if pkg != nil {
		unit.Name = pkg.name
		unit.Imports = pkg.imports
		unit.Documentation = pkg.docs
		var bcs []internal.BuildContext
		for _, d := range unit.Documentation {
			bcs = append(bcs, internal.BuildContext{GOOS: d.GOOS, GOARCH: d.GOARCH})
		}
		sort.Slice(bcs, func(i, j int) bool {
			return internal.CompareBuildContexts(bcs[i], bcs[j]) < 0
		})
		unit.BuildContexts = bcs
	}
	return unit
}

// moduleUnitMetas returns UnitMetas for all the units in a given module.
func moduleUnitMetas(minfo internal.ModuleInfo, pkgs []*packageMeta) []*internal.UnitMeta {
	pkgLookup := map[string]*packageMeta{}
	for _, pkg := range pkgs {
		pkgLookup[pkg.path] = pkg
	}
	dirPaths := unitPaths(minfo.ModulePath, pkgs)

	var ums []*internal.UnitMeta
	for _, dirPath := range dirPaths {
		um := &internal.UnitMeta{
			ModuleInfo: minfo,
			Path:       dirPath,
		}
		if pkg, ok := pkgLookup[dirPath]; ok {
			um.Name = pkg.name
		}
		ums = append(ums, um)
	}
	return ums
}

// unitPaths returns the paths for all the units in a module.
func unitPaths(modulePath string, packageMetas []*packageMeta) []string {
	shouldContinue := func(p string) bool {
		if modulePath == stdlib.ModulePath {
			return p != "."
		}
		return len(p) > len(modulePath)
	}

	pathSet := map[string]bool{modulePath: true}
	for _, p := range packageMetas {
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
