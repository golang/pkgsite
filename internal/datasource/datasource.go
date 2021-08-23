// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package datasource provides internal.DataSource implementations backed solely
// by a proxy instance, and backed by the local filesystem.
// Search and other tabs are not supported by these implementations.
package datasource

import (
	"sync"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/source"
)

// dataSource implements the internal.DataSource interface, by trying a list of
// fetch.ModuleGetters to fetch modules and caching the results.
type dataSource struct {
	sourceClient *source.Client

	mu    sync.Mutex
	cache map[internal.Modver]cacheEntry
}

// cacheEntry holds a fetched module or an error, if the fetch failed.
type cacheEntry struct {
	module *internal.Module
	err    error
}

func newDataSource(sc *source.Client) *dataSource {
	return &dataSource{
		sourceClient: sc,
		cache:        map[internal.Modver]cacheEntry{},
	}
}

// cacheGet returns information from the cache if it is present, and (nil, nil) otherwise.
func (ds *dataSource) cacheGet(path, version string) (*internal.Module, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	// Look for an exact match first.
	if e, ok := ds.cache[internal.Modver{Path: path, Version: version}]; ok {
		return e.module, e.err
	}
	// Look for the module path with LocalVersion, as for a directory-based or GOPATH-mode module.
	if e, ok := ds.cache[internal.Modver{Path: path, Version: fetch.LocalVersion}]; ok {
		return e.module, e.err
	}
	return nil, nil
}

// cachePut puts information into the cache.
func (ds *dataSource) cachePut(path, version string, m *internal.Module, err error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.cache[internal.Modver{Path: path, Version: version}] = cacheEntry{m, err}
}
