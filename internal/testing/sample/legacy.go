// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sample provides functionality for generating sample values of
// the types contained in the internal package.
package sample

import (
	"path"

	"golang.org/x/pkgsite/internal"
)

// LegacyPackage constructs a package with the given module path and suffix.
//
// If modulePath is the standard library, the package path is the
// suffix, which must not be empty. Otherwise, the package path
// is the concatenation of modulePath and suffix.
//
// The package name is last component of the package path.
func LegacyPackage(modulePath, suffix string) *internal.LegacyPackage {
	p := constructFullPath(modulePath, suffix)
	return &internal.LegacyPackage{
		Name:              path.Base(p),
		Path:              p,
		V1Path:            internal.V1Path(p, modulePath),
		Synopsis:          Synopsis,
		IsRedistributable: true,
		Licenses:          LicenseMetadata,
		DocumentationHTML: DocumentationHTML,
		Imports:           Imports,
		GOOS:              GOOS,
		GOARCH:            GOARCH,
	}
}

func LegacyDefaultModule() *internal.Module {
	fp := constructFullPath(ModulePath, Suffix)
	return AddPackage(LegacyModule(ModulePath, VersionString), UnitForPackage(fp, ModulePath, VersionString, path.Base(fp), true))
}

// LegacyModule creates a Module with the given path and version.
// The list of suffixes is used to create Units within the module.
func LegacyModule(modulePath, version string, suffixes ...string) *internal.Module {
	mi := ModuleInfo(modulePath, version)
	m := &internal.Module{
		ModuleInfo: *mi,
		Licenses:   Licenses,
	}
	m.Units = []*internal.Unit{UnitForModuleRoot(mi)}
	for _, s := range suffixes {
		fp := constructFullPath(modulePath, s)
		lp := UnitForPackage(fp, modulePath, VersionString, path.Base(fp), m.IsRedistributable)
		if s != "" {
			AddPackage(m, lp)
		} else {
			u := UnitForPackage(lp.Path, modulePath, version, lp.Name, lp.IsRedistributable)
			m.Units[0].Documentation = u.Documentation
			m.Units[0].Name = u.Name
		}
	}
	return m
}

func UnitForModuleRoot(m *internal.ModuleInfo) *internal.Unit {
	u := &internal.Unit{
		UnitMeta:        *UnitMeta(m.ModulePath, m.ModulePath, m.Version, "", m.IsRedistributable),
		LicenseContents: Licenses,
	}
	u.Readme = &internal.Readme{
		Filepath: ReadmeFilePath,
		Contents: ReadmeContents,
	}
	return u
}
