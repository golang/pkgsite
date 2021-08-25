// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package datasource provides internal.DataSource implementations backed solely
// by a proxy instance, and backed by the local filesystem.
// Search and other tabs are not supported by these implementations.
package datasource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
)

// dataSource implements the internal.DataSource interface, by trying a list of
// fetch.ModuleGetters to fetch modules and caching the results.
type dataSource struct {
	getters            []fetch.ModuleGetter
	sourceClient       *source.Client
	bypassLicenseCheck bool
	cache              *lru.Cache
	prox               *proxy.Client // used for latest-version info only

}

func newDataSource(getters []fetch.ModuleGetter, sc *source.Client, bypassLicenseCheck bool, prox *proxy.Client) *dataSource {
	cache, err := lru.New(maxCachedModules)
	if err != nil {
		// Can only happen if size is bad.
		panic(err)
	}
	return &dataSource{
		getters:            getters,
		sourceClient:       sc,
		bypassLicenseCheck: bypassLicenseCheck,
		cache:              cache,
		prox:               prox,
	}
}

// cacheEntry holds a fetched module or an error, if the fetch failed.
type cacheEntry struct {
	module *internal.Module
	err    error
}

const maxCachedModules = 100

// cacheGet returns information from the cache if it is present, and (nil, nil) otherwise.
func (ds *dataSource) cacheGet(path, version string) (*internal.Module, error) {
	// Look for an exact match first, then use LocalVersion, as for a
	// directory-based or GOPATH-mode module.
	for _, v := range []string{version, fetch.LocalVersion} {
		if e, ok := ds.cache.Get(internal.Modver{Path: path, Version: v}); ok {
			e := e.(cacheEntry)
			return e.module, e.err
		}
	}
	return nil, nil
}

// cachePut puts information into the cache.
func (ds *dataSource) cachePut(path, version string, m *internal.Module, err error) {
	ds.cache.Add(internal.Modver{Path: path, Version: version}, cacheEntry{m, err})
}

// getModule gets the module at the given path and version. It first checks the
// cache, and if it isn't there it then tries to fetch it.
func (ds *dataSource) getModule(ctx context.Context, modulePath, version string) (_ *internal.Module, err error) {
	defer derrors.Wrap(&err, "getModule(%q, %q)", modulePath, version)

	mod, err := ds.cacheGet(modulePath, version)
	if mod != nil || err != nil {
		return mod, err
	}

	// There can be a benign race here, where two goroutines both fetch the same
	// module. At worst some work will be duplicated, but if that turns out to
	// be a problem we could use golang.org/x/sync/singleflight.
	m, err := ds.fetch(ctx, modulePath, version)
	if m != nil && ds.prox != nil {
		// Use the go.mod file at the raw latest version to fill in deprecation
		// and retraction information.
		lmv, err2 := fetch.LatestModuleVersions(ctx, modulePath, ds.prox, nil)
		if err2 != nil {
			err = err2
		} else {
			lmv.PopulateModuleInfo(&m.ModuleInfo)
		}
	}

	// Don't cache cancellations.
	if !errors.Is(err, context.Canceled) {
		ds.cachePut(modulePath, version, m, err)
	}
	return m, err
}

// fetch fetches a module using the configured ModuleGetters.
// It tries each getter in turn until it finds one that has the module.
func (ds *dataSource) fetch(ctx context.Context, modulePath, version string) (_ *internal.Module, err error) {
	log.Infof(ctx, "DataSource: fetching %s@%s", modulePath, version)
	start := time.Now()
	defer func() {
		log.Infof(ctx, "DataSource: fetched %s@%s in %s with error %v", modulePath, version, time.Since(start), err)
	}()
	for _, g := range ds.getters {
		fr := fetch.FetchModule(ctx, modulePath, version, g, ds.sourceClient)
		defer fr.Defer()
		if fr.Error == nil {
			m := fr.Module
			if ds.bypassLicenseCheck {
				m.IsRedistributable = true
				for _, unit := range m.Units {
					unit.IsRedistributable = true
				}
			} else {
				m.RemoveNonRedistributableData()
			}
			return m, nil
		}
		if !errors.Is(fr.Error, derrors.NotFound) {
			return nil, fr.Error
		}
	}
	return nil, fmt.Errorf("%s@%s: %w", modulePath, version, derrors.NotFound)
}

// findModule finds the module with longest module path containing the given
// package path. It returns an error if no module is found.
func (ds *dataSource) findModule(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.Module, err error) {
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

// GetUnitMeta returns information about a path.
func (ds *dataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "GetUnitMeta(%q, %q, %q)", path, requestedModulePath, requestedVersion)

	module, err := ds.findModule(ctx, path, requestedModulePath, requestedVersion)
	if err != nil {
		return nil, err
	}
	um := &internal.UnitMeta{
		Path:       path,
		ModuleInfo: module.ModuleInfo,
	}
	if u := findUnit(module, path); u != nil {
		um.Name = u.Name
		um.IsRedistributable = u.IsRedistributable
	}
	return um, nil
}

// GetUnit returns information about a unit. Both the module path and package
// path must be known.
func (ds *dataSource) GetUnit(ctx context.Context, um *internal.UnitMeta, fields internal.FieldSet, bc internal.BuildContext) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(%q, %q)", um.Path, um.ModulePath)

	m, err := ds.getModule(ctx, um.ModulePath, um.Version)
	if err != nil {
		return nil, err
	}
	if u := findUnit(m, um.Path); u != nil {
		return u, nil
	}
	return nil, fmt.Errorf("import path %s not found in module %s: %w", um.Path, um.ModulePath, derrors.NotFound)
}

// findUnit returns the unit with the given path in m, or nil if none.
func findUnit(m *internal.Module, path string) *internal.Unit {
	for _, u := range m.Units {
		if u.Path == path {
			return u
		}
	}
	return nil
}
