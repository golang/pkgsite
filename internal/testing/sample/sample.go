// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sample provides functionality for generating sample values of
// the types contained in the internal package.
package sample

import (
	"fmt"
	"math"
	"path"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/licensecheck"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// These sample values can be used to construct test cases.
var (
	ModulePath      = "github.com/valid_module_name"
	RepositoryURL   = "https://github.com/valid_module_name"
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
	DocumentationHTML = "This is the documentation HTML"
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
	pkgPath := suffix
	if modulePath != stdlib.ModulePath {
		pkgPath = path.Join(modulePath, suffix)
	}
	return &internal.LegacyPackage{
		Name:              path.Base(pkgPath),
		Path:              pkgPath,
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

func LegacyModuleInfo(modulePath, versionString string) *internal.LegacyModuleInfo {
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
func ModuleInfoReleaseType(modulePath, versionString string) *internal.LegacyModuleInfo {
	return &internal.LegacyModuleInfo{
		ModulePath:           modulePath,
		Version:              versionString,
		LegacyReadmeFilePath: ReadmeFilePath,
		LegacyReadmeContents: ReadmeContents,
		CommitTime:           CommitTime,
		VersionType:          version.TypeRelease,
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

// Module creates a Module with the given path and version.
// The list of suffixes is used to create LegacyPackages within the module.
func Module(modulePath, version string, suffixes ...string) *internal.Module {
	mi := LegacyModuleInfo(modulePath, version)
	m := &internal.Module{
		LegacyModuleInfo: *mi,
		LegacyPackages:   nil,
		Licenses:         Licenses,
	}
	emptySuffix := false
	for _, s := range suffixes {
		if s == "" {
			emptySuffix = true
			break
		}
	}
	if emptySuffix {
		AddPackage(m, LegacyPackage(modulePath, ""))
	} else {
		m.Directories = []*internal.DirectoryNew{DirectoryNewForModuleRoot(mi, LicenseMetadata)}
	}
	for _, s := range suffixes {
		if s != "" {
			AddPackage(m, LegacyPackage(modulePath, s))
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
	AddDirectory(m, DirectoryNewForPackage(p))
	minLen := len(m.ModulePath)
	if m.ModulePath == stdlib.ModulePath {
		minLen = 1
	}
	for pth := p.Path; len(pth) > minLen; pth = path.Dir(pth) {
		found := false
		for _, d := range m.Directories {
			if d.Path == pth {
				found = true
				break
			}
		}
		if !found {
			AddDirectory(m, DirectoryNewEmpty(pth))
		}
	}
	return m
}

func AddDirectory(m *internal.Module, d *internal.DirectoryNew) {
	for _, e := range m.Directories {
		if e.Path == d.Path {
			panic(fmt.Sprintf("module already has path %q", e.Path))
		}
	}
	m.Directories = append(m.Directories, d)
}

func DirectoryNewEmpty(path string) *internal.DirectoryNew {
	return &internal.DirectoryNew{
		Path:              path,
		IsRedistributable: true,
		Licenses:          LicenseMetadata,
		V1Path:            path,
	}
}

func DirectoryNewForModuleRoot(m *internal.LegacyModuleInfo, licenses []*licenses.Metadata) *internal.DirectoryNew {
	d := &internal.DirectoryNew{
		Path:              m.ModulePath,
		IsRedistributable: m.IsRedistributable,
		Licenses:          licenses,
		V1Path:            internal.SeriesPathForModule(m.ModulePath),
	}
	if m.LegacyReadmeFilePath != "" {
		d.Readme = &internal.Readme{
			Filepath: m.LegacyReadmeFilePath,
			Contents: m.LegacyReadmeContents,
		}
	}
	return d
}

func DirectoryNewForPackage(pkg *internal.LegacyPackage) *internal.DirectoryNew {
	return &internal.DirectoryNew{
		Path:              pkg.Path,
		IsRedistributable: pkg.IsRedistributable,
		Licenses:          pkg.Licenses,
		V1Path:            pkg.V1Path,
		Package: &internal.PackageNew{
			Name:    pkg.Name,
			Path:    pkg.Path,
			Imports: pkg.Imports,
			Documentation: &internal.Documentation{
				Synopsis: pkg.Synopsis,
				HTML:     pkg.DocumentationHTML,
				GOOS:     pkg.GOOS,
				GOARCH:   pkg.GOARCH,
			},
		},
	}
}
