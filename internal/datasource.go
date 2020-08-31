// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"

	"golang.org/x/pkgsite/internal/licenses"
)

// DataSource is the interface used by the frontend to interact with module data.
type DataSource interface {
	// See the internal/postgres package for further documentation of these
	// methods, particularly as they pertain to the main postgres implementation.

	// GetLatestMajorVersion returns the latest major version of a series path.
	GetLatestMajorVersion(ctx context.Context, seriesPath string) (_ string, err error)
	// GetUnitMeta returns information about a path.
	GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *UnitMeta, err error)
	// GetUnit returns information about a directory, which may also be a module and/or package.
	// The module and version must both be known.
	GetUnit(ctx context.Context, pathInfo *UnitMeta, fields FieldSet) (_ *Unit, err error)

	// TODO(golang/go#39629): Deprecate these methods by moving the logic
	// behind GetUnit.
	//
	// GetLicenses returns licenses at the given path for given modulePath and version.
	GetLicenses(ctx context.Context, fullPath, modulePath, resolvedVersion string) ([]*licenses.License, error)

	// TODO(golang/go#39629): Deprecate these methods.
	//
	// LegacyGetDirectory returns packages whose import path is in a (possibly
	// nested) subdirectory of the given directory path. When multiple
	// package paths satisfy this query, it should prefer the module with
	// the longest path.
	LegacyGetDirectory(ctx context.Context, dirPath, modulePath, version string, fields FieldSet) (_ *LegacyDirectory, err error)
	// LegacyGetImports returns a slice of import paths imported by the package
	// specified by path and version.
	LegacyGetImports(ctx context.Context, pkgPath, modulePath, version string) ([]string, error)
	// LegacyGetModuleInfo returns the LegacyModuleInfo corresponding to modulePath and
	// version.
	LegacyGetModuleInfo(ctx context.Context, modulePath, version string) (*LegacyModuleInfo, error)
	// LegacyGetModuleLicenses returns all top-level Licenses for the given modulePath
	// and version. (i.e., Licenses contained in the module root directory)
	LegacyGetModuleLicenses(ctx context.Context, modulePath, version string) ([]*licenses.License, error)
	// LegacyGetPackage returns the LegacyVersionedPackage corresponding to the given package
	// pkgPath, modulePath, and version. When multiple package paths satisfy this query, it
	// should prefer the module with the longest path.
	LegacyGetPackage(ctx context.Context, pkgPath, modulePath, version string) (*LegacyVersionedPackage, error)
	// LegacyGetPackagesInModule returns LegacyPackages contained in the module version
	// specified by modulePath and version.
	LegacyGetPackagesInModule(ctx context.Context, modulePath, version string) ([]*LegacyPackage, error)
	// LegacyGetPackageLicenses returns all Licenses that apply to pkgPath, within the
	// module version specified by modulePath and version.
	LegacyGetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) ([]*licenses.License, error)
	// LegacyGetPsuedoVersionsForModule returns ModuleInfo for all known
	// pseudo-versions for the module corresponding to modulePath.
	LegacyGetPsuedoVersionsForModule(ctx context.Context, modulePath string) ([]*ModuleInfo, error)
	// LegacyGetPsuedoVersionsForModule returns ModuleInfo for all known
	// pseudo-versions for any module containing a package with the given import
	// path.
	LegacyGetPsuedoVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*ModuleInfo, error)
	// LegacyGetTaggedVersionsForModule returns ModuleInfo for all known tagged
	// versions for the module corresponding to modulePath.
	LegacyGetTaggedVersionsForModule(ctx context.Context, modulePath string) ([]*ModuleInfo, error)
	// LegacyGetTaggedVersionsForModule returns ModuleInfo for all known tagged
	// versions for any module containing a package with the given import path.
	LegacyGetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*ModuleInfo, error)
}
