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
	"time"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/source"
)

// dataSource implements the internal.DataSource interface, by trying a list of
// fetch.ModuleGetters to fetch modules and caching the results.
type dataSource struct {
	getters            []fetch.ModuleGetter
	sourceClient       *source.Client
	bypassLicenseCheck bool
	cache              *lru.Cache
}

func newDataSource(getters []fetch.ModuleGetter, sc *source.Client, bypassLicenseCheck bool) *dataSource {
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
