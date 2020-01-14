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
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/version"
)

// These sample values can be used to construct test cases.
var (
	ModulePath      = "github.com/valid_module_name"
	RepositoryURL   = "https://github.com/valid_module_name"
	VersionString   = "v1.0.0"
	CommitTime      = NowTruncated()
	LicenseMetadata = []*license.Metadata{
		{
			Types:    []string{"MIT"},
			FilePath: "LICENSE",
			Coverage: licensecheck.Coverage{
				Percent: 100,
				Match:   []licensecheck.Match{{Name: "MIT", Type: licensecheck.MIT, Percent: 100}},
			},
		},
	}
	Licenses = []*license.License{
		{Metadata: LicenseMetadata[0], Contents: `Lorem Ipsum`},
	}
	DocumentationHTML = "This is the documentation HTML"
	PackageName       = "foo"
	PackagePath       = "github.com/valid_module_name/foo"
	V1Path            = "github.com/valid_module_name/foo"
	Imports           = []string{"path/to/bar", "fmt"}
	Synopsis          = "This is a package synopsis"
	ReadmeFilePath    = "README.md"
	ReadmeContents    = "readme"
	VersionType       = version.TypeRelease
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

func Package() *internal.Package {
	return &internal.Package{
		Name:              PackageName,
		Synopsis:          Synopsis,
		Path:              PackagePath,
		Licenses:          LicenseMetadata,
		DocumentationHTML: DocumentationHTML,
		V1Path:            V1Path,
		Imports:           Imports,
		GOOS:              GOOS,
		GOARCH:            GOARCH,
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
		SourceInfo:     source.NewGitHubInfo(RepositoryURL, "", ""),
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
