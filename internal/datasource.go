// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package internal

import (
	"context"
	"golang.org/x/discovery/internal/license"
)

// DataSource is the interface used by the frontend to interact with module data.
type DataSource interface {
	// See the internal/postgres package for further documentation of these
	// methods, particularly as they pertain to the main postgres implementation.

	// GetDirectory returns packages whose import path is in a (possibly
	// nested) subdirectory of the given directory path. When multiple
	// package paths satisfy this query, it should prefer the module with
	// the longest path.
	GetDirectory(ctx context.Context, dirPath, modulePath, version string) (_ *Directory, err error)
	// GetImportedBy returns a slice of import paths corresponding to packages
	// that import the given package path (at any version).
	GetImportedBy(ctx context.Context, pkgPath, version string, limit int) ([]string, error)
	// GetImports returns a slice of import paths imported by the package
	// specified by path and version.
	GetImports(ctx context.Context, pkgPath, modulePath, version string) ([]string, error)
	// GetModuleLicenses returns all top-level Licenses for the given modulePath
	// and version. (i.e., Licenses contained in the module root directory)
	GetModuleLicenses(ctx context.Context, modulePath, version string) ([]*license.License, error)
	// GetPackage returns the VersionedPackage corresponding to the given package
	// pkgPath, modulePath, and version. When multiple package paths satisfy this query, it
	// should prefer the module with the longest path.
	GetPackage(ctx context.Context, pkgPath, modulePath, version string) (*VersionedPackage, error)
	// GetPackageLicenses returns all Licenses that apply to pkgPath, within the
	// module version specified by modulePath and version.
	GetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) ([]*license.License, error)
	// GetPackagesInVersion returns Packages contained in the module version
	// specified by modulePath and version.
	GetPackagesInVersion(ctx context.Context, modulePath, version string) ([]*Package, error)
	// GetPseudoVersionsForModule returns VersionInfo for all known
	// pseudo-versions for the module corresponding to modulePath.
	GetPseudoVersionsForModule(ctx context.Context, modulePath string) ([]*VersionInfo, error)
	// GetPseudoVersionsForModule returns VersionInfo for all known
	// pseudo-versions for any module containing a package with the given import
	// path.
	GetPseudoVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*VersionInfo, error)
	// GetTaggedVersionsForModule returns VersionInfo for all known tagged
	// versions for the module corresponding to modulePath.
	GetTaggedVersionsForModule(ctx context.Context, modulePath string) ([]*VersionInfo, error)
	// GetTaggedVersionsForModule returns VersionInfo for all known tagged
	// versions for any module containing a package with the given import path.
	GetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*VersionInfo, error)
	// GetVersionInfo returns the VersionInfo corresponding to modulePath and
	// version.
	GetVersionInfo(ctx context.Context, modulePath, version string) (*VersionInfo, error)
	// IsExcluded reports whether the path is excluded from processinng.
	IsExcluded(ctx context.Context, path string) (bool, error)

	// Temporarily, we support many types of search, for diagnostic purposes. In
	// the future this will be pruned to just one (FastSearch).

	// FastSearch performs a hedged search of both popular and all packages.
	FastSearch(ctx context.Context, query string, limit, offset int) ([]*SearchResult, error)

	// Alternative search types, for testing.
	// TODO(b/141182438): remove all of these.
	Search(ctx context.Context, query string, limit, offset int) ([]*SearchResult, error)
	DeepSearch(ctx context.Context, query string, limit, offset int) ([]*SearchResult, error)
	PartialFastSearch(ctx context.Context, query string, limit, offset int) ([]*SearchResult, error)
	PopularSearch(ctx context.Context, query string, limit, offset int) ([]*SearchResult, error)
}
