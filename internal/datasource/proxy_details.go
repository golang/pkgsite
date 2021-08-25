// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package datasource

import (
	"context"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// GetUnit returns information about a directory at a path.
func (ds *ProxyDataSource) GetUnit(ctx context.Context, um *internal.UnitMeta, field internal.FieldSet, bc internal.BuildContext) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(%q, %q, %q)", um.Path, um.ModulePath, um.Version)
	return ds.ds.GetUnit(ctx, um, field, bc)
}

func (ds *ProxyDataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	return ds.ds.GetUnitMeta(ctx, path, requestedModulePath, requestedVersion)
}

// GetExperiments is unimplemented.
func (*ProxyDataSource) GetExperiments(ctx context.Context) ([]*internal.Experiment, error) {
	return nil, nil
}

// GetNestedModules will return an empty slice since it is not implemented in proxy mode.
func (ds *ProxyDataSource) GetNestedModules(ctx context.Context, modulePath string) (_ []*internal.ModuleInfo, err error) {
	return nil, nil
}

// GetModuleReadme is unimplemented.
func (ds *ProxyDataSource) GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (*internal.Readme, error) {
	return nil, nil
}
