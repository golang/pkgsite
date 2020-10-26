// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sample provides functionality for generating sample values of
// the types contained in the internal package.
package sample

import (
	"fmt"
	"path"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
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
	return AddPackage(LegacyModule(ModulePath, VersionString), ModulePath+"/"+Suffix)
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
		lp := LegacyPackage(modulePath, s)
		if s != "" {
			LegacyAddPackage(m, lp)
		} else {
			u := UnitForPackage(lp.Path, modulePath, version, lp.Name, lp.IsRedistributable)
			m.Units[0].Documentation = u.Documentation
			m.Units[0].Name = u.Name
		}
	}
	return m
}

func LegacyAddPackage(m *internal.Module, p *internal.LegacyPackage) *internal.Module {
	if m.ModulePath != stdlib.ModulePath && !strings.HasPrefix(p.Path, m.ModulePath) {
		panic(fmt.Sprintf("package path %q not a prefix of module path %q",
			p.Path, m.ModulePath))
	}
	AddUnit(m, UnitForPackage(p.Path, m.ModulePath, m.Version, p.Name, p.IsRedistributable))
	minLen := len(m.ModulePath)
	if m.ModulePath == stdlib.ModulePath {
		minLen = 1
	}
	for pth := p.Path; len(pth) > minLen; pth = path.Dir(pth) {
		found := false
		for _, u := range m.Units {
			if u.Path == pth {
				found = true
				break
			}
		}
		if !found {
			AddUnit(m, UnitEmpty(pth, m.ModulePath, m.Version))
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
