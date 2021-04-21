// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "context"

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
}

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
