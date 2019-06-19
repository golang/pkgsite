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
}

// SeriesPath returns the series path for the module.
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
}

// A Package is a group of one or more Go source files with the same package
// header. Packages are part of a module.
type Package struct {
	Path              string
	Name              string
	Synopsis          string
	Licenses          []*license.Metadata // path to applicable version licenses
	Imports           []*Import
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

// An Import represents a package that is imported by another package.
type Import struct {
	Name string
	Path string
}

// VersionType defines the version types a module can have.
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
	// NextProcessedAfter is the next time a fetch for thsi version should be
	// attempted.
	NextProcessedAfter time.Time
}
