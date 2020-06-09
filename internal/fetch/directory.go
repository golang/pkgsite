// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"path"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/stdlib"
)

// moduleDirectories returns all of the directories in a given module, along
// with the contents for those directories.
func moduleDirectories(modulePath string,
	pkgs []*internal.LegacyPackage,
	readmes []*internal.Readme,
	d *licenses.Detector) []*internal.DirectoryNew {
	pkgLookup := map[string]*internal.LegacyPackage{}
	for _, pkg := range pkgs {
		pkgLookup[pkg.Path] = pkg
	}
	dirPaths := directoryPaths(modulePath, pkgs)

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

	var directories []*internal.DirectoryNew
	for _, dirPath := range dirPaths {
		suffix := strings.TrimPrefix(strings.TrimPrefix(dirPath, modulePath), "/")
		if modulePath == stdlib.ModulePath {
			suffix = dirPath
		}
		isRedist, lics := d.PackageInfo(suffix)
		var meta []*licenses.Metadata
		for _, l := range lics {
			meta = append(meta, l.Metadata)
		}
		dir := &internal.DirectoryNew{
			Path:              dirPath,
			V1Path:            internal.V1Path(modulePath, suffix),
			IsRedistributable: isRedist,
			Licenses:          meta,
		}
		if r, ok := readmeLookup[dirPath]; ok {
			dir.Readme = r
		}
		if pkg, ok := pkgLookup[dirPath]; ok {
			dir.Package = &internal.PackageNew{
				Path:    pkg.Path,
				Name:    pkg.Name,
				Imports: pkg.Imports,
				Documentation: &internal.Documentation{
					GOOS:     pkg.GOOS,
					GOARCH:   pkg.GOARCH,
					Synopsis: pkg.Synopsis,
					HTML:     pkg.DocumentationHTML,
				},
			}
		}
		directories = append(directories, dir)
	}
	return directories
}

// directoryPaths returns the directory paths in a module.
func directoryPaths(modulePath string, packages []*internal.LegacyPackage) []string {
	shouldContinue := func(p string) bool {
		if modulePath == stdlib.ModulePath {
			return p != "."
		}
		return len(p) > len(modulePath)
	}

	pathSet := map[string]bool{modulePath: true}
	for _, p := range packages {
		for p := p.Path; shouldContinue(p); p = path.Dir(p) {
			pathSet[p] = true
		}
	}

	var dirPaths []string
	for d := range pathSet {
		dirPaths = append(dirPaths, d)
	}
	return dirPaths
}
