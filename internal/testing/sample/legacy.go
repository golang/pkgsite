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
	"golang.org/x/pkgsite/internal/licenses"
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

func LegacyModuleInfo(modulePath, versionString string) *internal.LegacyModuleInfo {
	mi := ModuleInfoReleaseType(modulePath, versionString)
	return &internal.LegacyModuleInfo{
		ModuleInfo:           *mi,
		LegacyReadmeFilePath: ReadmeFilePath,
		LegacyReadmeContents: ReadmeContents,
	}
}

func DefaultModule() *internal.Module {
	return AddPackage(
		Module(ModulePath, VersionString),
		LegacyPackage(ModulePath, Suffix))
}

// Module creates a Module with the given path and version.
// The list of suffixes is used to create LegacyPackages within the module.
func Module(modulePath, version string, suffixes ...string) *internal.Module {
	mi := LegacyModuleInfo(modulePath, version)
	m := &internal.Module{
		LegacyModuleInfo: *mi,
		LegacyPackages:   nil,
		Licenses:         Licenses,
	}
	m.Units = []*internal.Unit{UnitForModuleRoot(mi, LicenseMetadata)}
	for _, s := range suffixes {
		lp := LegacyPackage(modulePath, s)
		if s != "" {
			AddPackage(m, lp)
		} else {
			m.LegacyPackages = append(m.LegacyPackages, lp)
			u := UnitForPackage(lp, modulePath, version)
			m.Units[0].Documentation = u.Documentation
			m.Units[0].Name = u.Name
		}
	}
	return m
}

func AddPackage(m *internal.Module, p *internal.LegacyPackage) *internal.Module {
	if m.ModulePath != stdlib.ModulePath && !strings.HasPrefix(p.Path, m.ModulePath) {
		panic(fmt.Sprintf("package path %q not a prefix of module path %q",
			p.Path, m.ModulePath))
	}
	m.LegacyPackages = append(m.LegacyPackages, p)
	AddUnit(m, UnitForPackage(p, m.ModulePath, m.Version))
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

func UnitForModuleRoot(m *internal.LegacyModuleInfo, licenses []*licenses.Metadata) *internal.Unit {
	u := &internal.Unit{
		UnitMeta:        *UnitMeta(m.ModulePath, m.ModulePath, m.Version, "", m.IsRedistributable),
		LicenseContents: Licenses,
	}
	if m.LegacyReadmeFilePath != "" {
		u.Readme = &internal.Readme{
			Filepath: m.LegacyReadmeFilePath,
			Contents: m.LegacyReadmeContents,
		}
	}
	return u
}

func UnitForPackage(pkg *internal.LegacyPackage, modulePath, version string) *internal.Unit {
	return &internal.Unit{
		UnitMeta:        *UnitMeta(pkg.Path, modulePath, version, pkg.Name, pkg.IsRedistributable),
		Imports:         pkg.Imports,
		LicenseContents: Licenses,
		Documentation: &internal.Documentation{
			Synopsis: pkg.Synopsis,
			HTML:     pkg.DocumentationHTML,
			GOOS:     pkg.GOOS,
			GOARCH:   pkg.GOARCH,
		},
	}
}
