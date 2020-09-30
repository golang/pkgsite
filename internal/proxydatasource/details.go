// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxydatasource implements an internal.DataSource backed solely by a
// proxy instance.
package proxydatasource

import (
	"context"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
)

// GetUnit returns information about a directory at a path.
func (ds *DataSource) GetUnit(ctx context.Context, um *internal.UnitMeta, field internal.FieldSet) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(%q, %q, %q)", um.Path, um.ModulePath, um.Version)
	return ds.getUnit(ctx, um.Path, um.ModulePath, um.Version)
}

// GetModuleInfo returns the ModuleInfo as fetched from the proxy for module
// version specified by modulePath and version.
func (ds *DataSource) GetModuleInfo(ctx context.Context, modulePath, version string) (_ *internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetModuleInfo(%q, %q)", modulePath, version)
	m, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return &m.ModuleInfo, nil
}

// GetUnitMeta returns information about the given path.
func (ds *DataSource) GetUnitMeta(ctx context.Context, path, inModulePath, inVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "GetUnitMeta(%q, %q, %q)", path, inModulePath, inVersion)

	var info *proxy.VersionInfo
	if inModulePath == internal.UnknownModulePath {
		inModulePath, info, err = ds.findModule(ctx, path, inVersion)
		if err != nil {
			return nil, err
		}
		inVersion = info.Version
	}
	m, err := ds.getModule(ctx, inModulePath, inVersion)
	if err != nil {
		return nil, err
	}
	um := &internal.UnitMeta{
		Path:       path,
		ModulePath: inModulePath,
		Version:    inVersion,
	}
	for _, d := range m.Units {
		if d.Path == path {
			um.Name = d.Name
			um.IsRedistributable = d.IsRedistributable
			break
		}
	}
	return um, nil
}

// GetExperiments is unimplemented.
func (*DataSource) GetExperiments(ctx context.Context) ([]*internal.Experiment, error) {
	return nil, nil
}

// GetNestedModules will return an empty slice since it is not implemented in proxy mode.
func (ds *DataSource) GetNestedModules(ctx context.Context, modulePath string) (_ []*internal.ModuleInfo, err error) {
	return nil, nil
}
