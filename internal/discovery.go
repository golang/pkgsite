// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"time"
)

// LicenseInfo holds license metadata.
type LicenseInfo struct {
	Type     string
	FilePath string
}

// A License is a classified license file path and its contents.
type License struct {
	LicenseInfo
	Contents []byte
}

// VersionInfo holds metadata associated with a version.
type VersionInfo struct {
	SeriesPath     string
	ModulePath     string
	Version        string
	CommitTime     time.Time
	ReadmeFilePath string
	ReadmeContents []byte
	VersionType    VersionType
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
	Suffix            string         // if my.module/v2/A/B is the path, A/B is the package suffix
	Licenses          []*LicenseInfo // path to applicable version licenses
	Imports           []*Import
	DocumentationHTML []byte
}

// IsRedistributable reports whether the package may be redistributed.
func (p *Package) IsRedistributable() bool {
	return licensesAreRedistributable(p.Licenses)
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

// A VersionSource is the source of a record in the version logs.
type VersionSource string

const (
	// Version fetched by the Proxy Index Cron
	VersionSourceProxyIndex = VersionSource("proxy-index")

	// Version requested by user from Frontend
	VersionSourceFrontend = VersionSource("frontend")
)

func (vs VersionSource) String() string {
	return string(vs)
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

// A VersionLog is a record of a version that was
// (1) fetched from the module proxy index,
// (2) requested by user from the frontend
// The Path and Version may not necessarily be valid module versions (for example, if a
// user requests a module or version that does not exist).
type VersionLog struct {
	// A JSON struct tag is needed because the index uses the field "Path"
	// instead of "ModulePath".
	ModulePath string `json:"Path"`

	// Use the modproxy timestamp for the CreatedAt field, as this field is used
	// for polling the index.
	CreatedAt time.Time `json:"Timestamp"`

	Version string
	Source  VersionSource
	Error   string
}
