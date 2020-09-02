// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"time"

	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
)

// UnitMeta represents metadata about a unit.
type UnitMeta struct {
	// Unit level information
	//
	Path              string
	Name              string
	IsRedistributable bool
	Licenses          []*licenses.Metadata

	// Module level information
	//
	Version    string
	ModulePath string
	CommitTime time.Time
	SourceInfo *source.Info
}

// IsPackage reports whether the path represents a package path.
func (um *UnitMeta) IsPackage() bool {
	return um.Name != ""
}

// IsModule reports whether the path represents a module path.
func (um *UnitMeta) IsModule() bool {
	return um.ModulePath == um.Path
}

// Unit represents the contents of some path in the Go package/module
// namespace. It might be a module, a package, both a module and a package, or
// none of the above: a directory within a module that has no .go files, but
// contains other units, licenses and/or READMEs."
type Unit struct {
	UnitMeta
	Readme          *Readme
	Documentation   *Documentation
	Subdirectories  []*PackageMeta
	Imports         []string
	LicenseContents []*licenses.License
}

// Documentation is the rendered documentation for a given package
// for a specific GOOS and GOARCH.
type Documentation struct {
	// The values of the GOOS and GOARCH environment variables used to parse the
	// package.
	GOOS     string
	GOARCH   string
	Synopsis string
	HTML     safehtml.HTML
}

// Readme is a README at the specified filepath.
type Readme struct {
	Filepath string
	Contents string
}

// A FieldSet is a bit set of struct fields. It is used to avoid reading large
// struct fields from the data store. FieldSet is also the type of the
// individual bit values. (Think of them as singleton sets.)
//
// MinimalFields (the zero value) is the empty set. AllFields is the set containing
// every field.
//
// FieldSet bits are unique across the entire project, because some types are
// concatenations (via embedding) of others. For example, a
// LegacyVersionedPackage contains the fields of both a LegacyModuleInfo and a
// LegacyPackage, so we can't use the
// same bits for both LegacyModuleInfo's LegacyReadmeContents field and
// LegacyPackage's DocumentationHTML field.
type FieldSet int64

// MinimalFields is the empty FieldSet.
const MinimalFields FieldSet = 0

// AllFields is the FieldSet that contains all fields.
const AllFields FieldSet = -1

// StringFieldMissing is the value for string fields that are not present
// in a struct. We use it to distinguish a (possibly valid) empty string
// from a field that was never populated.
const StringFieldMissing = "!MISSING"

// FieldSet bits for fields that can be conditionally read from the data store.
const (
	WithReadme FieldSet = 1 << iota
	WithDocumentation
	WithImports
	WithLicenses
	WithSubdirectories
)
