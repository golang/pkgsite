// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "context"

// DataSource is the interface used by the frontend to interact with module data.
type DataSource interface {
	// See the internal/postgres package for further documentation of these
	// methods, particularly as they pertain to the main postgres implementation.

	// GetLatestMajorVersion returns the latest module path and the full package path
	// of the latest version found, given the fullPath and the modulePath.
	// For example, in the module path "github.com/casbin/casbin", there
	// is another module path with a greater major version "github.com/casbin/casbin/v3".
	// This function will return "github.com/casbin/casbin/v3" or the input module path
	// if no later module path was found. It also returns the full package path at the
	// latest module version if it exists. If not, it returns the module path.
	GetLatestMajorVersion(ctx context.Context, fullPath, modulePath string) (_ string, _ string, err error)
	// GetNestedModules returns the latest major version of all nested modules
	// given a modulePath path prefix.
	GetNestedModules(ctx context.Context, modulePath string) ([]*ModuleInfo, error)
	// GetUnit returns information about a directory, which may also be a
	// module and/or package. The module and version must both be known.
	GetUnit(ctx context.Context, pathInfo *UnitMeta, fields FieldSet) (_ *Unit, err error)
	// GetUnitMeta returns information about a path.
	GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *UnitMeta, err error)
	// GetModuleReadme gets the readme for the module.
	GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (*Readme, error)
}
