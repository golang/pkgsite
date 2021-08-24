// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package datasource provides internal.DataSource implementations backed solely
// by a proxy instance, and backed by the local filesystem.
// Search and other tabs are not supported by these implementations.
package datasource

import (
	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/source"
)

// dataSource implements the internal.DataSource interface, by trying a list of
// fetch.ModuleGetters to fetch modules and caching the results.
type dataSource struct {
	getters      []fetch.ModuleGetter
	sourceClient *source.Client
	cache        *lru.Cache
}

// cacheEntry holds a fetched module or an error, if the fetch failed.
type cacheEntry struct {
	module *internal.Module
	err    error
}

const maxCachedModules = 100

func newDataSource(getters []fetch.ModuleGetter, sc *source.Client) *dataSource {
	cache, err := lru.New(maxCachedModules)
	if err != nil {
		// Can only happen if size is bad.
		panic(err)
	}
	return &dataSource{
		getters:      getters,
		sourceClient: sc,
		cache:        cache,
	}
}

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
