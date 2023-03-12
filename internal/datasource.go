// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"time"
)

// SearchOptions provide information used by db.Search.
type SearchOptions struct {
	// Maximum number of results to return (page size).
	MaxResults int

	// Offset for DB query.
	Offset int

	// Maximum number to use for total result count.
	MaxResultCount int

	// If true, perform a symbol search.
	SearchSymbols bool

	// SymbolFilter is the word in a search query with a # prefix.
	SymbolFilter string
}

// SearchResult represents a single search result from SearchDocuments.
type SearchResult struct {
	Name        string
	PackagePath string
	ModulePath  string
	Version     string
	Synopsis    string
	Licenses    []string

	CommitTime time.Time

	// Score is used to sort items in an array of SearchResult.
	Score float64

	// NumImportedBy is the number of packages that import PackagePath.
	NumImportedBy uint64

	// SameModule is a list of SearchResults from the same module as this one,
	// with lower scores.
	SameModule []*SearchResult

	// OtherMajor is a map from module paths with the same series path but at
	// different major versions of this module, to major version.
	// The major version for a non-vN module path (either 0 or 1) is computed
	// based on the version in search documents.
	OtherMajor map[string]int

	// NumResults is the total number of packages that were returned for this
	// search.
	NumResults uint64

	// Symbol information returned by a search request.
	// Only populated for symbol search mode.
	SymbolName     string
	SymbolKind     SymbolKind
	SymbolSynopsis string
	SymbolGOOS     string
	SymbolGOARCH   string

	// Offset is the 0-based number of this row in the DB query results, which
	// is the value to use in a SQL OFFSET clause to have this row be the first
	// one returned.
	Offset int
}

// DataSource is the interface used by the frontend to interact with module data.
type DataSource interface {
	// See the internal/postgres package for further documentation of these
	// methods, particularly as they pertain to the main postgres implementation.

	// GetNestedModules returns the latest major version of all nested modules
	// given a modulePath path prefix.
	GetNestedModules(ctx context.Context, modulePath string) ([]*ModuleInfo, error)
	// GetUnit returns information about a directory, which may also be a
	// module and/or package. The module and version must both be known.
	// The BuildContext selects the documentation to read.
	GetUnit(ctx context.Context, pathInfo *UnitMeta, fields FieldSet, bc BuildContext) (_ *Unit, err error)
	// GetUnitMeta returns information about a path.
	GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *UnitMeta, err error)
	// GetModuleReadme gets the readme for the module.
	GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (*Readme, error)
	// GetLatestInfo gets information about the latest versions of a unit and module.
	// See LatestInfo for documentation.
	GetLatestInfo(ctx context.Context, unitPath, modulePath string, latestUnitMeta *UnitMeta) (LatestInfo, error)

	// SearchSupport reports the search types supported by this datasource.
	SearchSupport() SearchSupport
	// Search searches for packages matching the given query.
	Search(ctx context.Context, q string, opts SearchOptions) (_ []*SearchResult, err error)
}

type SearchSupport int

const (
	NoSearch    SearchSupport = iota
	BasicSearch               // package search only
	FullSearch                // all search modes supported
)

// LatestInfo holds information about the latest versions and paths.
// The information is relative to a unit in a module.
type LatestInfo struct {
	// MinorVersion is the latest minor version for the unit, regardless of
	// module.
	MinorVersion string

	// MinorModulePath is the module path for MinorVersion.
	MinorModulePath string

	// UnitExistsAtMinor is whether the unit exists at the latest minor version
	// of the module
	UnitExistsAtMinor bool

	// MajorModulePath is the path of the latest module path in the series.
	// For example, in the module path "github.com/casbin/casbin", there
	// is another module path with a greater major version
	// "github.com/casbin/casbin/v3". This field will be
	// "github.com/casbin/casbin/v3" or the input module path if no later module
	// path was found.
	MajorModulePath string

	// MajorUnitPath is the path of the unit in the latest major version of the
	// module, if it exists. For example, if the module is M, the unit is M/U,
	// and the latest major version is 3, then is field is "M/v3/U". If the module version
	// at MajorModulePath does not contain this unit, then it is the module path."
	MajorUnitPath string
}
