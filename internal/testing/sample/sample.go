// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sample provides functionality for generating sample values of
// the types contained in the internal package.
package sample

import (
	"math"
	"path"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/licensecheck"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
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
	PackagePath       = "github.com/valid_module_name/foo"
	V1Path            = "github.com/valid_module_name/foo"
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

func DefaultPackage() *internal.Package {
	return Package(PackageName, PackagePath, V1Path)
}

func Package(name, path, v1path string) *internal.Package {
	return &internal.Package{
		Name:              name,
		Path:              path,
		V1Path:            v1path,
		Synopsis:          Synopsis,
		IsRedistributable: true,
		Licenses:          LicenseMetadata,
		DocumentationHTML: DocumentationHTML,
		Imports:           Imports,
		GOOS:              GOOS,
		GOARCH:            GOARCH,
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
		ModulePath:        modulePath,
		Version:           versionString,
		ReadmeFilePath:    ReadmeFilePath,
		ReadmeContents:    ReadmeContents,
		CommitTime:        CommitTime,
		VersionType:       version.TypeRelease,
		SourceInfo:        source.NewGitHubInfo(RepositoryURL, "", ""),
		IsRedistributable: true,
		HasGoMod:          true,
	}
}

func DefaultModule() *internal.Module {
	return AddPackage(Module(ModulePath, VersionString), DefaultPackage())
}

func Module(modulePath, version string) *internal.Module {
	mi := ModuleInfo(modulePath, version)
	return &internal.Module{
		ModuleInfo: *mi,
		Packages:   nil,
		Licenses:   Licenses,
		Directories: []*internal.DirectoryNew{
			DirectoryNewForModuleRoot(mi, LicenseMetadata),
		},
	}
}

func AddPackage(m *internal.Module, p *internal.Package) *internal.Module {
	m.Packages = append(m.Packages, p)
	m.Directories = append(m.Directories, DirectoryNewForPackage(p))
	return m
}

func DirectoryNewEmpty(path string) *internal.DirectoryNew {
	return &internal.DirectoryNew{
		Path:              path,
		IsRedistributable: true,
		Licenses:          LicenseMetadata,
		V1Path:            path,
	}
}

func DirectoryNewForModuleRoot(m *internal.ModuleInfo, licenses []*licenses.Metadata) *internal.DirectoryNew {
	d := &internal.DirectoryNew{
		Path:              m.ModulePath,
		IsRedistributable: m.IsRedistributable,
		Licenses:          licenses,
		V1Path:            internal.SeriesPathForModule(m.ModulePath),
	}
	if m.ReadmeFilePath != "" {
		d.Readme = &internal.Readme{
			Filepath: m.ReadmeFilePath,
			Contents: m.ReadmeContents,
		}
	}
	return d
}

func DirectoryNewForPackage(pkg *internal.Package) *internal.DirectoryNew {
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

// SetSuffixes sets packages corresponding to the given path suffixes.  Paths
// are constructed using the existing module path of the Version.
func SetSuffixes(m *internal.Module, suffixes ...string) {
	series := internal.SeriesPathForModule(m.ModulePath)
	m.Packages = nil
	for _, suffix := range suffixes {
		p := DefaultPackage()
		p.Path = path.Join(m.ModulePath, suffix)
		p.V1Path = path.Join(series, suffix)
		m.Packages = append(m.Packages, p)
	}
}
