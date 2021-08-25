// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package datasource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/source"
)

// LocalDataSource implements an in-memory internal.DataSource used to display documentation
// locally. It is not backed by a database or a proxy instance.
type LocalDataSource struct {
	sourceClient *source.Client
	ds           *dataSource
}

// New creates and returns a new local datasource that bypasses license
// checks by default.
func NewLocal(getters []fetch.ModuleGetter, sc *source.Client) *LocalDataSource {
	return &LocalDataSource{
		sourceClient: sc,
		ds:           newDataSource(getters, sc, true, nil),
	}
}

// NewGOPATHModuleGetter returns a module getter that uses the GOPATH
// environment variable to find the module with the given import path.
func NewGOPATHModuleGetter(importPath string) (_ fetch.ModuleGetter, err error) {
	defer derrors.Wrap(&err, "NewGOPATHModuleGetter(%q)", importPath)

	dir := getFullPath(importPath)
	if dir == "" {
		return nil, fmt.Errorf("path %s doesn't exist: %w", importPath, derrors.NotFound)
	}
	return fetch.NewDirectoryModuleGetter(importPath, dir)
}

// getFullPath takes an import path, tests it relative to each GOPATH, and returns
// a full path to the module. If the given import path doesn't exist in any GOPATH,
// an empty string is returned.
func getFullPath(modulePath string) string {
	gopaths := filepath.SplitList(os.Getenv("GOPATH"))
	for _, gopath := range gopaths {
		path := filepath.Join(gopath, "src", modulePath)
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func (ds *LocalDataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	return ds.ds.GetUnitMeta(ctx, path, requestedModulePath, requestedVersion)
}

// GetUnit returns information about a unit. Both the module path and package
// path must be known.
func (ds *LocalDataSource) GetUnit(ctx context.Context, pathInfo *internal.UnitMeta, fields internal.FieldSet, bc internal.BuildContext) (_ *internal.Unit, err error) {
	return ds.ds.GetUnit(ctx, pathInfo, fields, bc)
}

// GetLatestInfo is not implemented.
func (ds *LocalDataSource) GetLatestInfo(ctx context.Context, unitPath, modulePath string, latestUnitMeta *internal.UnitMeta) (internal.LatestInfo, error) {
	return internal.LatestInfo{}, nil
}

// GetNestedModules is not implemented.
func (ds *LocalDataSource) GetNestedModules(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	return nil, nil
}

// GetModuleReadme is not implemented.
func (*LocalDataSource) GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (*internal.Readme, error) {
	return nil, nil
}
