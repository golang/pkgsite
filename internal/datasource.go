// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "context"

// DataSource is the interface used by the frontend to interact with module data.
type DataSource interface {
	// See the internal/postgres package for further documentation of these
	// methods, particularly as they pertain to the main postgres implementation.

	// GetLatestMajorVersion returns the latest major version of a series path.
	GetLatestMajorVersion(ctx context.Context, seriesPath string) (_ string, err error)
	// GetNestedModules returns the latest major version of all nested modules
	// given a modulePath path prefix.
	GetNestedModules(ctx context.Context, modulePath string) ([]*ModuleInfo, error)
	// GetUnit returns information about a directory, which may also be a
	// module and/or package. The module and version must both be known.
	GetUnit(ctx context.Context, pathInfo *UnitMeta, fields FieldSet) (_ *Unit, err error)
	// GetUnitMeta returns information about a path.
	GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *UnitMeta, err error)
}
