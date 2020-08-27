// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sample provides functionality for generating sample values of
// the types contained in the internal package.
package sample

import (
	"fmt"
	"math"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/licensecheck"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// These sample values can be used to construct test cases.
var (
	ModulePath      = "github.com/valid/module_name"
	RepositoryURL   = "https://github.com/valid/module_name"
	VersionString   = "v1.0.0"
	CommitTime      = NowTruncated()
	LicenseMetadata = []*licenses.Metadata{
		{
			Types:    []string{"MIT"},
			FilePath: "LICENSE",
			Coverage: licensecheck.Coverage{
				Percent: 100,
				Match:   []licensecheck.Match{{Name: "MIT", Type: licensecheck.MIT, Percent: 100}},
			},
		},
	}
	Licenses = []*licenses.License{
		{Metadata: LicenseMetadata[0], Contents: []byte(`Lorem Ipsum`)},
	}
	NonRedistributableLicense = &licenses.License{
		Metadata: &licenses.Metadata{
			FilePath: "NONREDIST_LICENSE",
			Types:    []string{"UNKNOWN"},
		},
		Contents: []byte(`unknown`),
	}
	DocumentationHTML = template.MustParseAndExecuteToHTML("This is the documentation HTML")
	PackageName       = "foo"
	Suffix            = "foo"
	PackagePath       = path.Join(ModulePath, Suffix)
	V1Path            = PackagePath
	Imports           = []string{"path/to/bar", "fmt"}
	Synopsis          = "This is a package synopsis"
	ReadmeFilePath    = "README.md"
	ReadmeContents    = "readme"
	GOOS              = "linux"
	GOARCH            = "amd64"
)

// LicenseCmpOpts are options to use when comparing licenses with the cmp package.
var LicenseCmpOpts = []cmp.Option{
	cmp.Comparer(coveragePercentEqual),
	cmpopts.IgnoreFields(licensecheck.Match{}, "Start", "End"),
}

// coveragePercentEqual considers two floats the same if they are within 4
// percentage points, and both are on the same side of 90% (our threshold).
func coveragePercentEqual(a, b float64) bool {
	if (a >= 90) != (b >= 90) {
		return false
	}
	return math.Abs(a-b) <= 4
}

// NowTruncated returns time.Now() truncated to Microsecond precision.
//
// This makes it easier to work with timestamps in PostgreSQL, which have
// Microsecond precision:
//   https://www.postgresql.org/docs/9.1/datatype-datetime.html
func NowTruncated() time.Time {
	return time.Now().Truncate(time.Microsecond)
}

// LegacyPackage constructs a package with the given module path and suffix.
//
// If modulePath is the standard library, the package path is the
// suffix, which must not be empty. Otherwise, the package path
// is the concatenation of modulePath and suffix.
//
// The package name is last component of the package path.
func LegacyPackage(modulePath, suffix string) *internal.LegacyPackage {
	return &internal.LegacyPackage{
		Name:              path.Base(fullPath(modulePath, suffix)),
		Path:              fullPath(modulePath, suffix),
		V1Path:            internal.V1Path(modulePath, suffix),
		Synopsis:          Synopsis,
		IsRedistributable: true,
		Licenses:          LicenseMetadata,
		DocumentationHTML: DocumentationHTML,
		Imports:           Imports,
		GOOS:              GOOS,
		GOARCH:            GOARCH,
	}
}

func DirectoryMeta(modulePath, suffix string) *internal.DirectoryMeta {
	return &internal.DirectoryMeta{
		ModuleInfo:        *ModuleInfo(modulePath, VersionString),
		Path:              fullPath(modulePath, suffix),
		Name:              path.Base(fullPath(modulePath, suffix)),
		V1Path:            internal.V1Path(modulePath, suffix),
		IsRedistributable: true,
		Licenses:          LicenseMetadata,
	}
}

func PackageMeta(modulePath, suffix string) *internal.PackageMeta {
	return &internal.PackageMeta{
		Path:              fullPath(modulePath, suffix),
		IsRedistributable: true,
		Name:              path.Base(fullPath(modulePath, suffix)),
		Synopsis:          Synopsis,
		Licenses:          LicenseMetadata,
	}
}

func LegacyModuleInfo(modulePath, versionString string) *internal.LegacyModuleInfo {
	vtype, err := version.ParseType(versionString)
	if err != nil {
		panic(err)
	}
	mi := ModuleInfoReleaseType(modulePath, versionString)
	mi.VersionType = vtype
	return &internal.LegacyModuleInfo{
		ModuleInfo:           *mi,
		LegacyReadmeFilePath: ReadmeFilePath,
		LegacyReadmeContents: ReadmeContents,
	}
}

func ModuleInfo(modulePath, versionString string) *internal.ModuleInfo {
	vtype, err := version.ParseType(versionString)
	if err != nil {
		panic(err)
	}
	mi := ModuleInfoReleaseType(modulePath, versionString)
	mi.VersionType = vtype
	return mi
}

// We shouldn't need this, but some code (notably frontend/directory_test.go) creates
// ModuleInfos with "latest" for version, which should not be valid.
func ModuleInfoReleaseType(modulePath, versionString string) *internal.ModuleInfo {
	return &internal.ModuleInfo{
		ModulePath:  modulePath,
		Version:     versionString,
		CommitTime:  CommitTime,
		VersionType: version.TypeRelease,
		// Assume the module path is a GitHub-like repo name.
		SourceInfo:        source.NewGitHubInfo("https://"+modulePath, "", versionString),
		IsRedistributable: true,
		HasGoMod:          true,
	}
}

func DefaultModule() *internal.Module {
	return AddPackage(
		Module(ModulePath, VersionString),
		LegacyPackage(ModulePath, Suffix))
}

func DefaultVersionMap() *internal.VersionMap {
	return &internal.VersionMap{
		ModulePath:       ModulePath,
		RequestedVersion: VersionString,
		ResolvedVersion:  VersionString,
		Status:           http.StatusOK,
		GoModPath:        "",
		Error:            "",
	}
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
			m.Units[0].Package = UnitForPackage(lp).Package
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
	AddUnit(m, UnitForPackage(p))
	minLen := len(m.ModulePath)
	if m.ModulePath == stdlib.ModulePath {
		minLen = 1
	}
	for pth := p.Path; len(pth) > minLen; pth = path.Dir(pth) {
		found := false
		for _, d := range m.Units {
			if d.Path == pth {
				found = true
				break
			}
		}
		if !found {
			AddUnit(m, UnitEmpty(pth))
		}
	}
	return m
}

func AddUnit(m *internal.Module, d *internal.Unit) {
	for _, e := range m.Units {
		if e.Path == d.Path {
			panic(fmt.Sprintf("module already has path %q", e.Path))
		}
	}
	m.Units = append(m.Units, d)
}

func AddLicense(m *internal.Module, lic *licenses.License) {
	m.Licenses = append(m.Licenses, lic)
	dir := path.Dir(lic.FilePath)
	if dir == "." {
		dir = ""
	}
	for _, d := range m.Units {
		if strings.TrimPrefix(d.Path, m.ModulePath+"/") == dir {
			d.Licenses = append(d.Licenses, lic.Metadata)
		}
	}
}

func UnitEmpty(path string) *internal.Unit {
	return &internal.Unit{
		DirectoryMeta: internal.DirectoryMeta{
			Path:              path,
			IsRedistributable: true,
			Licenses:          LicenseMetadata,
			V1Path:            path,
		},
	}
}

func UnitForModuleRoot(m *internal.LegacyModuleInfo, licenses []*licenses.Metadata) *internal.Unit {
	d := &internal.Unit{
		DirectoryMeta: internal.DirectoryMeta{
			Path:              m.ModulePath,
			IsRedistributable: m.IsRedistributable,
			Licenses:          licenses,
			V1Path:            internal.SeriesPathForModule(m.ModulePath),
		},
	}
	if m.LegacyReadmeFilePath != "" {
		d.Readme = &internal.Readme{
			Filepath: m.LegacyReadmeFilePath,
			Contents: m.LegacyReadmeContents,
		}
	}
	return d
}

func UnitForPackage(pkg *internal.LegacyPackage) *internal.Unit {
	return &internal.Unit{
		DirectoryMeta: internal.DirectoryMeta{
			Path:              pkg.Path,
			IsRedistributable: pkg.IsRedistributable,
			Licenses:          pkg.Licenses,
			V1Path:            pkg.V1Path,
		},
		Imports: pkg.Imports,
		Package: &internal.Package{
			Name: pkg.Name,
			Path: pkg.Path,
			Documentation: &internal.Documentation{
				Synopsis: pkg.Synopsis,
				HTML:     pkg.DocumentationHTML,
				GOOS:     pkg.GOOS,
				GOARCH:   pkg.GOARCH,
			},
		},
	}
}

func fullPath(modulePath, suffix string) string {
	if modulePath != stdlib.ModulePath {
		return path.Join(modulePath, suffix)
	}
	return suffix
}
