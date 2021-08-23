// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package localdatasource implements an in-memory internal.DataSource used to load
// and display documentation for local modules that are not available via a proxy.
// Similar to proxydatasource, search and other tabs are not supported in this mode.
package localdatasource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/source"
)

// DataSource implements an in-memory internal.DataSource used to display documentation
// locally. DataSource is not backed by a database or a proxy instance.
type DataSource struct {
	sourceClient *source.Client

	mu            sync.Mutex
	getters       []fetch.ModuleGetter
	loadedModules map[string]*internal.Module
}

// New creates and returns a new local datasource that bypasses license
// checks by default.
func New(sc *source.Client) *DataSource {
	return &DataSource{
		sourceClient:  sc,
		loadedModules: make(map[string]*internal.Module),
	}
}

// AddModuleGetter adds a module getter to the DataSource. To look up a module,
// the getters are tried in the order they were added until the desired module
// is found.
func (ds *DataSource) AddModuleGetter(g fetch.ModuleGetter) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.getters = append(ds.getters, g)
}

// getModule gets the module at the given path and version. It first checks the
// cache, and if it isn't there it then tries to fetch it.
func (ds *DataSource) getModule(ctx context.Context, path, version string) (*internal.Module, error) {
	if m := ds.getFromCache(path, version); m != nil {
		return m, nil
	}
	m, err := ds.fetch(ctx, path, version)
	if err != nil {
		return nil, err
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.loadedModules[m.ModulePath+"@"+m.Version] = m
	return m, nil
}

// getFromCache returns a module from the cache if it is present, and nil otherwise.
func (ds *DataSource) getFromCache(path, version string) *internal.Module {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	// Look for an exact match first.
	if m := ds.loadedModules[path+"@"+version]; m != nil {
		return m
	}
	// Look for the module path with LocalVersion, as for a directory-based or GOPATH-mode module.
	return ds.loadedModules[path+"@"+fetch.LocalVersion]
}

// fetch fetches a module using the configured ModuleGetters.
// It tries each getter in turn until it finds one that has the module.
func (ds *DataSource) fetch(ctx context.Context, modulePath, version string) (_ *internal.Module, err error) {
	log.Infof(ctx, "local DataSource: fetching %s@%s", modulePath, version)
	start := time.Now()
	defer func() {
		log.Infof(ctx, "local DataSource: fetched %s@%s in %s with error %v", modulePath, version, time.Since(start), err)
	}()
	for _, g := range ds.getters {
		fr := fetch.FetchModule(ctx, modulePath, version, g, ds.sourceClient)
		if fr.Error == nil {
			adjust(fr.Module)
			return fr.Module, nil
		}
		if !errors.Is(fr.Error, derrors.NotFound) {
			return nil, fr.Error
		}
	}
	return nil, fmt.Errorf("%s@%s: %w", modulePath, version, derrors.NotFound)
}

func adjust(m *internal.Module) {
	m.IsRedistributable = true
	for _, unit := range m.Units {
		unit.IsRedistributable = true
	}
	for _, unit := range m.Units {
		for _, d := range unit.Documentation {
			unit.BuildContexts = append(unit.BuildContexts, internal.BuildContext{
				GOOS:   d.GOOS,
				GOARCH: d.GOARCH,
			})
		}
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

// GetUnit returns information about a unit. Both the module path and package
// path must be known.
func (ds *DataSource) GetUnit(ctx context.Context, pathInfo *internal.UnitMeta, fields internal.FieldSet, bc internal.BuildContext) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(%q, %q)", pathInfo.Path, pathInfo.ModulePath)

	module, err := ds.getModule(ctx, pathInfo.ModulePath, pathInfo.Version)
	if err != nil {
		return nil, err
	}
	for _, unit := range module.Units {
		if unit.Path == pathInfo.Path {
			return unit, nil
		}
	}

	return nil, fmt.Errorf("import path %s not found in module %s: %w", pathInfo.Path, pathInfo.ModulePath, derrors.NotFound)
}

// GetUnitMeta returns information about a path.
func (ds *DataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "GetUnitMeta(%q, %q, %q)", path, requestedModulePath, requestedVersion)

	module, err := ds.findModule(ctx, path, requestedModulePath, requestedVersion)
	if err != nil {
		return nil, err
	}
	um := &internal.UnitMeta{
		Path:       path,
		ModuleInfo: module.ModuleInfo,
	}

	for _, u := range module.Units {
		if u.Path == path {
			um.Name = u.Name
			um.IsRedistributable = u.IsRedistributable
		}
	}

	return um, nil
}

// findModule finds the module with longest module path containing the given
// package path. It returns an error if no module is found.
func (ds *DataSource) findModule(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.Module, err error) {
	defer derrors.Wrap(&err, "findModule(%q, %q, %q)", pkgPath, modulePath, version)

	if modulePath != internal.UnknownModulePath {
		return ds.getModule(ctx, modulePath, version)
	}
	pkgPath = strings.TrimLeft(pkgPath, "/")
	for _, modulePath := range internal.CandidateModulePaths(pkgPath) {
		m, err := ds.getModule(ctx, modulePath, version)
		if err == nil {
			return m, nil
		}
		if !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("could not find module for import path %s: %w", pkgPath, derrors.NotFound)
}

// GetLatestInfo is not implemented.
func (ds *DataSource) GetLatestInfo(ctx context.Context, unitPath, modulePath string, latestUnitMeta *internal.UnitMeta) (internal.LatestInfo, error) {
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
