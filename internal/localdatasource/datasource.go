// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package localdatasource implements an in-memory internal.DataSource used to load
// and display documentation for local modules that are not available via a proxy.
// Similar to proxydatasource, search and other tabs are not supported in this mode.
package localdatasource

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/source"
)

// DataSource implements an in-memory internal.DataSource used to display documentation
// locally. DataSource is not backed by a database or a proxy instance.
type DataSource struct {
	sourceClient *source.Client

	mu            sync.Mutex
	loadedModules map[string]*internal.Module
}

// New creates and returns a new local datasource that bypasses license
// checks by default.
func New() *DataSource {
	return &DataSource{
		loadedModules: make(map[string]*internal.Module),
	}
}

// Load loads a module from the given local path. Loading is required before
// being able to display the module.
func (ds *DataSource) Load(ctx context.Context, localPath string) (err error) {
	defer derrors.Wrap(&err, "Load(%q)", localPath)
	return ds.fetch(ctx, "", localPath)
}

// LoadFromGOPATH loads a module from GOPATH using the given import path. The full
// path of the module should be GOPATH/src/importPath. If several GOPATHs exist, the
// module is loaded from the first one that contains the import path. Loading is required
// before being able to display the module.
func (ds *DataSource) LoadFromGOPATH(ctx context.Context, importPath string) (err error) {
	defer derrors.Wrap(&err, "LoadFromGOPATH(%q)", importPath)

	path := getFullPath(importPath)
	if path == "" {
		return fmt.Errorf("path %s doesn't exist: %w", importPath, derrors.NotFound)
	}

	return ds.fetch(ctx, importPath, path)
}

// fetch fetches a module using FetchLocalModule and adds it to the datasource.
// If the fetching fails, an error is returned.
func (ds *DataSource) fetch(ctx context.Context, modulePath, localPath string) error {
	fr := fetch.FetchLocalModule(ctx, modulePath, localPath, ds.sourceClient)
	if fr.Error != nil {
		return fr.Error
	}

	fr.Module.IsRedistributable = true
	for _, unit := range fr.Module.Units {
		unit.IsRedistributable = true
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.loadedModules[fr.ModulePath] = fr.Module
	return nil
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

// GetUnit returns information about a unit. Both the module path and package
// path must both be known.
func (ds *DataSource) GetUnit(ctx context.Context, pathInfo *internal.UnitMeta, fields internal.FieldSet) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(%q, %q)", pathInfo.Path, pathInfo.ModulePath)

	modulepath := pathInfo.ModulePath
	path := pathInfo.Path

	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.loadedModules[modulepath] == nil {
		return nil, fmt.Errorf("%s not loaded: %w", modulepath, derrors.NotFound)
	}

	module := ds.loadedModules[modulepath]
	for _, unit := range module.Units {
		if unit.Path == path {
			return unit, nil
		}
	}

	return nil, fmt.Errorf("%s not found: %w", path, derrors.NotFound)
}

// GetUnitMeta returns information about a path.
func (ds *DataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "GetUnitMeta(%q, %q, %q)", path, requestedModulePath, requestedVersion)

	if requestedModulePath == internal.UnknownModulePath {
		requestedModulePath, err = ds.findModule(path)
		if err != nil {
			return nil, err
		}
	}

	ds.mu.Lock()
	module := ds.loadedModules[requestedModulePath]
	ds.mu.Unlock()

	um := &internal.UnitMeta{
		Path:       path,
		ModulePath: requestedModulePath,
		Version:    fetch.LocalVersion,
		CommitTime: fetch.LocalCommitTime,
	}

	for _, u := range module.Units {
		if u.Path == path {
			um.Name = u.Name
			um.IsRedistributable = u.IsRedistributable
		}
	}

	return um, nil
}

// findModule finds the longest module path in loadedModules containing the given
// package path. It iteratively checks parent directories to find an import path.
// Returns an error if no module is found.
func (ds *DataSource) findModule(pkgPath string) (_ string, err error) {
	defer derrors.Wrap(&err, "findModule(%q)", pkgPath)

	pkgPath = strings.TrimLeft(pkgPath, "/")

	ds.mu.Lock()
	defer ds.mu.Unlock()
	for modulePath := pkgPath; modulePath != "" && modulePath != "."; modulePath = path.Dir(modulePath) {
		if ds.loadedModules[modulePath] != nil {
			return modulePath, nil
		}
	}

	return "", fmt.Errorf("%s not loaded: %w", pkgPath, derrors.NotFound)
}

// GetLatestInfo is not implemented.
func (ds *DataSource) GetLatestInfo(ctx context.Context, unitPath, modulePath string) (internal.LatestInfo, error) {
	return internal.LatestInfo{}, nil
}

// GetNestedModules is not implemented.
func (ds *DataSource) GetNestedModules(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	return nil, nil
}

// GetModuleReadme is not implemented.
func (*DataSource) GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (*internal.Readme, error) {
	return nil, nil
}
