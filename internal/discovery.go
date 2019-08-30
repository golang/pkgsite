// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"time"

	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/thirdparty/module"
)

// VersionInfo holds metadata associated with a version.
type VersionInfo struct {
	ModulePath     string
	Version        string
	CommitTime     time.Time
	ReadmeFilePath string
	ReadmeContents []byte
	VersionType    VersionType
	VCSType        string
	RepositoryURL  string
	HomepageURL    string
}

// SeriesPath returns the series path for the module.
//
// A series is a group of modules that share the same base path and are assumed
// to be major-version variants.
//
// The series path is the module path without the version. For most modules,
// this will be the module path for all module versions with major version 0 or
// 1. For gopkg.in modules, the series path does not correspond to any module
// version.
//
// Examples:
// The module paths "a/b" and "a/b/v2"  both have series path "a/b".
// The module paths "gopkg.in/yaml.v1" and "gopkg.in/yaml.v2" both have series path "gopkg.in/yaml".
func (v *VersionInfo) SeriesPath() string {
	return SeriesPathForModule(v.ModulePath)
}

// SeriesPathForModule returns the series path for the provided modulePath.
func SeriesPathForModule(modulePath string) string {
	seriesPath, _, _ := module.SplitPathVersion(modulePath)
	return seriesPath
}

// A Version is a specific, reproducible build of a module.
type Version struct {
	VersionInfo
	Packages []*Package
	// Licenses holds all licenses within this module version, including those
	// that may be contained in nested subdirectories.
	Licenses []*license.License
}

// A Package is a group of one or more Go source files with the same package
// header. Packages are part of a module.
type Package struct {
	Path              string
	Name              string
	Synopsis          string
	Licenses          []*license.Metadata // path to applicable version licenses
	Imports           []string
	DocumentationHTML []byte

	// V1Path is the package path of a package with major version 1 in a given series.
	V1Path string
}

// IsRedistributable reports whether the package may be redistributed.
func (p *Package) IsRedistributable() bool {
	return license.AreRedistributable(p.Licenses)
}

// VersionedPackage is a Package along with its corresponding version
// information.
type VersionedPackage struct {
	Package
	VersionInfo
}

// Directory represents a folder in a module version, and all of the packages
// inside that folder.
type Directory struct {
	Path     string
	Version  string
	Packages []*VersionedPackage
}

// VersionType defines the version types a module can have.
// This must be kept in sync with the 'version_type' database enum.
type VersionType string

const (
	// VersionTypeRelease is a normal release.
	VersionTypeRelease = VersionType("release")

	// VersionTypePrerelease is a version with a prerelease.
	VersionTypePrerelease = VersionType("prerelease")

	// VersionTypePseudo appears to have a prerelease of the
	// form <commit date>-<commit hash>.
	VersionTypePseudo = VersionType("pseudo")
)

func (vt VersionType) String() string {
	return string(vt)
}

// IndexVersion holds the version information returned by the module index.
type IndexVersion struct {
	Path      string
	Version   string
	Timestamp time.Time
}

// VersionState holds the ETL version state.
type VersionState struct {
	ModulePath string
	Version    string

	// IndexTimestamp is the timestamp received from the Index for this version,
	// which should correspond to the time this version was committed to the
	// Index.
	IndexTimestamp time.Time
	// CreatedAt is the time this version was originally inserted into the
	// version state table.
	CreatedAt time.Time

	// Status is the most recent HTTP status code received from the Fetch service
	// for this version, or nil if no request to the fetch service has been made.
	Status *int
	// Error is the most recent HTTP response body received from the Fetch
	// service, for a response with an unsuccessful status code. It is used for
	// debugging only, and has no semantic significance.
	Error *string
	// TryCount is the number of times a fetch of this version has been
	// attempted.
	TryCount int
	// LastProcessedAt is the last time this version was updated with a result
	// from the fetch service.
	LastProcessedAt *time.Time
	// NextProcessedAfter is the next time a fetch for this version should be
	// attempted.
	NextProcessedAfter time.Time

	// AppVersion is the value of the GAE_VERSION environment variable, which is
	// set by app engine. It is a timestamp in the format 20190709t112655 that
	// is close to, but not the same as, the deployment time. For example, the
	// deployment time for the above timestamp might be Jul 9, 2019, 11:29:59 AM.
	AppVersion string
}
