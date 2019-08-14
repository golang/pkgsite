// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sample provides functionality for generating sample values of
// the types contained in the internal package.
package sample

import (
	"path"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/license"
)

// These sample values can be used to construct test cases.
var (
	ModulePath      = "github.com/valid_module_name"
	RepositoryURL   = "github.com/valid_module_name"
	VersionString   = "v1.0.0"
	VCSType         = "git"
	CommitTime      = NowTruncated()
	LicenseMetadata = []*license.Metadata{
		{Types: []string{"MIT"}, FilePath: "LICENSE"},
	}
	Licenses = []*license.License{
		{Metadata: LicenseMetadata[0], Contents: []byte(`Lorem Ipsum`)},
	}
	DocumentationHTML = []byte("This is the documentation HTML")
	PackageName       = "foo"
	PackagePath       = "github.com/valid_module_name/foo"
	V1Path            = "github.com/valid_module_name/foo"
	Imports           = []string{"path/to/bar", "fmt"}
	Synopsis          = "This is a package synopsis"
	ReadmeFilePath    = "README.md"
	ReadmeContents    = []byte("readme")
	VersionType       = internal.VersionTypeRelease
	VersionInfo       = func() *internal.VersionInfo {
		return &internal.VersionInfo{
			ModulePath:     ModulePath,
			Version:        VersionString,
			ReadmeFilePath: ReadmeFilePath,
			ReadmeContents: ReadmeContents,
			CommitTime:     CommitTime,
			VersionType:    VersionType,
			VCSType:        VCSType,
			RepositoryURL:  RepositoryURL,
			HomepageURL:    ModulePath,
		}
	}
	Version = VersionSampler(func() *internal.Version {
		return &internal.Version{
			VersionInfo: *VersionInfo(),
			Packages:    []*internal.Package{Package()},
		}
	}).Sample
	VersionedPackage = VersionedPackageSampler(func() *internal.VersionedPackage {
		return &internal.VersionedPackage{
			VersionInfo: *VersionInfo(),
			Package:     *Package(),
		}
	}).Sample
)

// NowTruncated returns time.Now() truncated to Microsecond precision.
//
// This makes it easier to work with timestamps in PostgreSQL, which have
// Microsecond precision:
//   https://www.postgresql.org/docs/9.1/datatype-datetime.html
func NowTruncated() time.Time {
	return time.Now().Truncate(time.Microsecond)
}

func Package() *internal.Package {
	return &internal.Package{
		Name:              PackageName,
		Synopsis:          Synopsis,
		Path:              PackagePath,
		Licenses:          LicenseMetadata,
		DocumentationHTML: DocumentationHTML,
		V1Path:            V1Path,
		Imports:           Imports,
	}
}

// Samplers and Mutators are used to generate composite types. A Sampler
// provides a Sample method that creates a new instance of the type, after
// applying zero or more Mutators. This pattern facilitates generation of test
// data inline within a table-driven test struct.
//
// Mutators are intended to be composable, though they may be order-dependent.
type (
	VersionedPackageSampler func() *internal.VersionedPackage
	VersionedPackageMutator func(*internal.VersionedPackage)
	VersionSampler          func() *internal.Version
	VersionMutator          func(*internal.Version)
)

// Sample returns the templated VersionedPackage, after applying mutators.
func (s VersionedPackageSampler) Sample(mutators ...VersionedPackageMutator) *internal.VersionedPackage {
	p := s()
	for _, mut := range mutators {
		mut(p)
	}
	return p
}

// Sample returns the templated Version, after applying mutators.
func (s VersionSampler) Sample(mutators ...VersionMutator) *internal.Version {
	v := s()
	for _, mut := range mutators {
		mut(v)
	}
	return v
}

// WithModulePath sets Version.ModulePath
func WithModulePath(modulePath string) VersionMutator {
	return func(v *internal.Version) { v.ModulePath = modulePath }
}

// WithVersion sets the Version.Version.
func WithVersion(version string) VersionMutator {
	return func(v *internal.Version) { v.Version = version }
}

// WithVersionType sets Version.VersionType.
func WithVersionType(versionType internal.VersionType) VersionMutator {
	return func(v *internal.Version) { v.VersionType = versionType }
}

// WithPackages sets the given packages.
func WithPackages(packages ...*internal.Package) VersionMutator {
	return func(v *internal.Version) { v.Packages = packages }
}

// WithSuffixes sets packages corresponding to the given path suffixes.  Paths
// are constructed using the existing module path of the Version.
func WithSuffixes(suffixes ...string) VersionMutator {
	return func(v *internal.Version) {
		series := internal.SeriesPathForModule(v.ModulePath)
		v.Packages = nil
		for _, suffix := range suffixes {
			p := Package()
			p.Path = path.Join(v.ModulePath, suffix)
			p.V1Path = path.Join(series, suffix)
			v.Packages = append(v.Packages, p)
		}
	}
}
