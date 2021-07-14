// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

const (
	// LatestVersion signifies the latest available version in requests to the
	// proxy client.
	LatestVersion = "latest"

	// MainVersion represents the main branch.
	MainVersion = "main"

	// MasterVersion represents the master branch.
	MasterVersion = "master"

	// UnknownModulePath signifies that the module path for a given package
	// path is ambiguous or not known. This is because requests to the
	// frontend can come in the form of <import-path>[@<version>], and it is
	// not clear which part of the import-path is the module path.
	UnknownModulePath = "unknownModulePath"
)

// DefaultBranches are default branches that are supported by pkgsite.
var DefaultBranches = map[string]bool{
	MainVersion:   true,
	MasterVersion: true,
}

// ModuleInfo holds metadata associated with a module.
type ModuleInfo struct {
	ModulePath        string
	Version           string
	CommitTime        time.Time
	IsRedistributable bool
	// HasGoMod describes whether the module zip has a go.mod file.
	HasGoMod   bool
	SourceInfo *source.Info

	// Deprecated describes whether the module is deprecated.
	Deprecated bool
	// DeprecationComment is the comment describing the deprecation, if any.
	DeprecationComment string
	// Retracted describes whether the module version is retracted.
	Retracted bool
	// RetractionRationale is the reason for the retraction, if any.
	RetractionRationale string
}

// VersionMap holds metadata associated with module queries for a version.
type VersionMap struct {
	ModulePath       string
	RequestedVersion string
	ResolvedVersion  string
	GoModPath        string
	Status           int
	Error            string
	UpdatedAt        time.Time
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
// The module paths "gopkg.in/yaml.v1" and "gopkg.in/yaml.v2" both have series
// path "gopkg.in/yaml".
func (v *ModuleInfo) SeriesPath() string {
	return SeriesPathForModule(v.ModulePath)
}

// SeriesPathForModule returns the series path for the provided modulePath.
func SeriesPathForModule(modulePath string) string {
	seriesPath, _, _ := module.SplitPathVersion(modulePath)
	return seriesPath
}

// MajorVersionForModule returns the final "vN" from the module path, if any.
// It returns the empty string if the module path is malformed.
// Examples:
//   "m.com" => ""
//   "m.com/v2" => "v2"
//   "gpkg.in/m.v1 = "v1"
func MajorVersionForModule(modulePath string) string {
	_, v, _ := module.SplitPathVersion(modulePath)
	return strings.TrimLeft(v, "/.")
}

// SeriesPathAndMajorVersion splits modulePath into a series path and a
// numeric major version.
// If the path doesn't have a "vN" suffix, it returns 1.
// If the module path is invalid, it returns ("", 0).
func SeriesPathAndMajorVersion(modulePath string) (string, int) {
	seriesPath, v, ok := module.SplitPathVersion(modulePath)
	if !ok {
		return "", 0
	}
	if v == "" {
		return seriesPath, 1
	}
	// First two characters are either ".v" or "/v".
	n, err := strconv.Atoi(v[2:])
	if err != nil {
		return "", 0
	}
	return seriesPath, n
}

// Suffix returns the suffix of the fullPath. It assumes that basePath is a
// prefix of fullPath. If fullPath and basePath are the same, the empty string
// is returned.
func Suffix(fullPath, basePath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(fullPath, basePath), "/")
}

// V1Path returns the path for version 1 of the package whose import path
// is fullPath. If modulePath is the standard library, then V1Path returns
// fullPath.
func V1Path(fullPath, modulePath string) string {
	if modulePath == stdlib.ModulePath {
		return fullPath
	}
	return path.Join(SeriesPathForModule(modulePath), Suffix(fullPath, modulePath))
}

// A Module is a specific, reproducible build of a module.
type Module struct {
	ModuleInfo
	// Licenses holds all licenses within this module version, including those
	// that may be contained in nested subdirectories.
	Licenses []*licenses.License
	Units    []*Unit
}

// Packages returns all of the units for a module that are packages.
func (m *Module) Packages() []*Unit {
	var pkgs []*Unit
	for _, u := range m.Units {
		if u.IsPackage() {
			pkgs = append(pkgs, u)
		}
	}
	return pkgs
}

// IndexVersion holds the version information returned by the module index.
type IndexVersion struct {
	Path      string
	Version   string
	Timestamp time.Time
}

// ModuleVersionState holds a worker module version state.
type ModuleVersionState struct {
	ModulePath string
	Version    string

	// IndexTimestamp is the timestamp received from the Index for this version,
	// which should correspond to the time this version was committed to the
	// Index.
	IndexTimestamp time.Time
	// CreatedAt is the time this version was originally inserted into the
	// module version state table.
	CreatedAt time.Time

	// Status is the most recent HTTP status code received from the Fetch service
	// for this version, or nil if no request to the fetch service has been made.
	Status int
	// Error is the most recent HTTP response body received from the Fetch
	// service, for a response with an unsuccessful status code. It is used for
	// debugging only, and has no semantic significance.
	Error string
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

	// HasGoMod says whether the zip file has a go.mod file.
	HasGoMod bool

	// GoModPath is the path declared in the go.mod file fetched from the proxy.
	GoModPath string

	// NumPackages it the number of packages that were processed as part of the
	// module (regardless of whether the processing was successful).
	NumPackages *int
}

// PackageVersionState holds a worker package version state. It is associated
// with a given module version state.
type PackageVersionState struct {
	PackagePath string
	ModulePath  string
	Version     string
	Status      int
	Error       string
}
