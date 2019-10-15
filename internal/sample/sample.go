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
	"golang.org/x/discovery/internal/version"
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
	VersionType       = version.TypeRelease
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
		GOOS:              "linux",
		GOARCH:            "amd64",
	}
}

func VersionInfo() *internal.VersionInfo {
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

func VersionedPackage() *internal.VersionedPackage {
	return &internal.VersionedPackage{
		VersionInfo: *VersionInfo(),
		Package:     *Package(),
	}
}

func Version() *internal.Version {
	return &internal.Version{
		VersionInfo: *VersionInfo(),
		Packages:    []*internal.Package{Package()},
		Licenses:    Licenses,
	}
}

// SetSuffixes sets packages corresponding to the given path suffixes.  Paths
// are constructed using the existing module path of the Version.
func SetSuffixes(v *internal.Version, suffixes ...string) {
	series := internal.SeriesPathForModule(v.ModulePath)
	v.Packages = nil
	for _, suffix := range suffixes {
		p := Package()
		p.Path = path.Join(v.ModulePath, suffix)
		p.V1Path = path.Join(series, suffix)
		v.Packages = append(v.Packages, p)
	}
}
