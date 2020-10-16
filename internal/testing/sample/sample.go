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
	Documentation     = &internal.Documentation{
		Synopsis: Synopsis,
		HTML:     DocumentationHTML,
		GOOS:     GOOS,
		GOARCH:   GOARCH,
	}
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

// UnitForPackage constructs a unit with the given module path and suffix.
//
// If modulePath is the standard library, the package path is the
// suffix, which must not be empty. Otherwise, the package path
// is the concatenation of modulePath and suffix.
//
// The package name is last component of the package path.
func UnitForPackage(modulePath, suffix string) *internal.Unit {
	p := constructFullPath(modulePath, suffix)
	return &internal.Unit{
		UnitMeta: internal.UnitMeta{
			Name:              path.Base(p),
			Path:              p,
			IsRedistributable: true,
			Licenses:          LicenseMetadata,
		},
		Documentation: &internal.Documentation{
			Synopsis: Synopsis,
			HTML:     DocumentationHTML,
			GOOS:     GOOS,
			GOARCH:   GOARCH,
		},
		Imports: Imports,
	}
}

func AddPackage(m *internal.Module, fullPath string) *internal.Module {
	if m.ModulePath != stdlib.ModulePath && !strings.HasPrefix(fullPath, m.ModulePath) {
		panic(fmt.Sprintf("package path %q not a prefix of module path %q",
			fullPath, m.ModulePath))
	}
	AddUnit(m, &internal.Unit{
		UnitMeta:        *UnitMeta(fullPath, m.ModulePath, m.Version, path.Base(fullPath), true),
		Imports:         Imports,
		LicenseContents: Licenses,
		Documentation: &internal.Documentation{
			Synopsis: Synopsis,
			HTML:     DocumentationHTML,
			GOOS:     GOOS,
			GOARCH:   GOARCH,
		},
	})
	minLen := len(m.ModulePath)
	if m.ModulePath == stdlib.ModulePath {
		minLen = 1
	}
	for pth := fullPath; len(pth) > minLen; pth = path.Dir(pth) {
		found := false
		for _, u := range m.Units {
			if u.Path == pth {
				found = true
				break
			}
		}
		if !found {
			AddUnit(m, UnitEmpty(pth, m.ModulePath, m.Version))
		}
	}
	return m
}

func PackageMeta(fullPath string) *internal.PackageMeta {
	return &internal.PackageMeta{
		Path:              fullPath,
		IsRedistributable: true,
		Name:              path.Base(fullPath),
		Synopsis:          Synopsis,
		Licenses:          LicenseMetadata,
	}
}

func ModuleInfo(modulePath, versionString string) *internal.ModuleInfo {
	return &internal.ModuleInfo{
		ModulePath: modulePath,
		Version:    versionString,
		CommitTime: CommitTime,
		// Assume the module path is a GitHub-like repo name.
		SourceInfo:        source.NewGitHubInfo("https://"+modulePath, "", versionString),
		IsRedistributable: true,
		HasGoMod:          true,
	}
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

func AddUnit(m *internal.Module, u *internal.Unit) {
	for _, e := range m.Units {
		if e.Path == u.Path {
			panic(fmt.Sprintf("module already has path %q", e.Path))
		}
	}
	m.Units = append(m.Units, u)
}

func AddLicense(m *internal.Module, lic *licenses.License) {
	m.Licenses = append(m.Licenses, lic)
	dir := path.Dir(lic.FilePath)
	if dir == "." {
		dir = ""
	}
	for _, u := range m.Units {
		if strings.TrimPrefix(u.Path, m.ModulePath+"/") == dir {
			u.Licenses = append(u.Licenses, lic.Metadata)
		}
	}
}

func UnitEmpty(path, modulePath, version string) *internal.Unit {
	return &internal.Unit{
		UnitMeta: *UnitMeta(path, modulePath, version, "", true),
	}
}

func UnitMeta(path, modulePath, version, name string, isRedistributable bool) *internal.UnitMeta {
	return &internal.UnitMeta{
		ModulePath:        modulePath,
		Version:           version,
		Path:              path,
		Name:              name,
		IsRedistributable: isRedistributable,
		Licenses:          LicenseMetadata,
		SourceInfo:        source.NewGitHubInfo("https://"+modulePath, "", version),
	}
}

func constructFullPath(modulePath, suffix string) string {
	if modulePath != stdlib.ModulePath {
		return path.Join(modulePath, suffix)
	}
	return suffix
}
