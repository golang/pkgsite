// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetchdatasource provides an internal.DataSource implementation
// that fetches modules (rather than reading them from a database).
// Search and other tabs are not supported.
package fetchdatasource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/lru"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/version"
)

// FetchDataSource implements the internal.DataSource interface, by trying a list of
// fetch.ModuleGetters to fetch modules and caching the results.
type FetchDataSource struct {
	opts  Options
	cache *lru.Cache[internal.Modver, cacheEntry]
}

// Options are parameters for creating a new FetchDataSource.
type Options struct {
	// List of getters to try, in order.
	Getters []fetch.ModuleGetter
	// If set, this will be used for latest-version information. To fetch modules from the proxy,
	// include a ProxyModuleGetter in Getters.
	ProxyClientForLatest *proxy.Client
	BypassLicenseCheck   bool
}

// New creates a new FetchDataSource from the options.
func (o Options) New() *FetchDataSource {
	cache := lru.New[internal.Modver, cacheEntry](maxCachedModules)

	opts := o
	// Copy getters slice so caller doesn't modify us.
	opts.Getters = make([]fetch.ModuleGetter, len(opts.Getters))
	copy(opts.Getters, o.Getters)
	return &FetchDataSource{
		opts:  opts,
		cache: cache,
	}
}

// cacheEntry holds a fetched module or an error, if the fetch failed.
type cacheEntry struct {
	g      fetch.ModuleGetter
	module *internal.Module
	err    error
}

const maxCachedModules = 100

// cacheGet returns information from the cache if it is present, and (nil, nil) otherwise.
func (ds *FetchDataSource) cacheGet(path, version string) (fetch.ModuleGetter, *internal.Module, error) {
	// Look for an exact match first, then use LocalVersion, as for a
	// directory-based or GOPATH-mode module.
	for _, v := range []string{version, fetch.LocalVersion} {
		if e, ok := ds.cache.Get(internal.Modver{Path: path, Version: v}); ok {
			return e.g, e.module, e.err
		}
	}
	return nil, nil, nil
}

// cachePut puts information into the cache.
func (ds *FetchDataSource) cachePut(g fetch.ModuleGetter, path, version string, m *internal.Module, err error) {
	ds.cache.Put(internal.Modver{Path: path, Version: version}, cacheEntry{g, m, err})
}

// getModule gets the module at the given path and version. It first checks the
// cache, and if it isn't there it then tries to fetch it.
func (ds *FetchDataSource) getModule(ctx context.Context, modulePath, vers string) (_ *internal.Module, err error) {
	defer derrors.Wrap(&err, "FetchDataSource.getModule(%q, %q)", modulePath, vers)

	g, mod, err := ds.cacheGet(modulePath, vers)
	if err != nil {
		return nil, err
	}
	if mod != nil {
		// For getters supporting invalidation, check whether cached contents have
		// changed.
		v, ok := g.(fetch.VolatileModuleGetter)
		if !ok {
			return mod, nil
		}
		hasChanged, err := v.HasChanged(ctx, mod.ModuleInfo)
		if err != nil {
			return nil, err
		}
		if !hasChanged {
			return mod, nil
		}
	}

	// There can be a benign race here, where two goroutines both fetch the same
	// module. At worst some work will be duplicated, but if that turns out to
	// be a problem we could use golang.org/x/sync/singleflight.
	m, g, err := ds.fetch(ctx, modulePath, vers)
	if m != nil && ds.opts.ProxyClientForLatest != nil {
		// Use the go.mod file at the raw latest version to fill in deprecation
		// and retraction information. Ignore any problems getting the
		// information, because we may be trying to do this for a local module
		// that the proxy doesn't know about.
		if lmv, err := fetch.LatestModuleVersions(ctx, modulePath, ds.opts.ProxyClientForLatest, nil); err == nil {
			lmv.PopulateModuleInfo(&m.ModuleInfo)
		}
	}
	// Populate unit subdirectories. When we use a database, this only happens when we read
	// a unit from the DB.
	if m != nil {
		for _, u := range m.Units {
			ds.populateUnitSubdirectories(u, m)
		}
	}

	// Cache both successes and failures, but not cancellations.
	if !errors.Is(err, context.Canceled) {
		ds.cachePut(g, modulePath, vers, m, err)
		// Cache the resolved version of "latest" too. A useful optimization
		// because the frontend redirects "latest", resulting in another fetch.
		if m != nil && vers == version.Latest {
			ds.cachePut(g, modulePath, m.Version, m, err)
		}
	}
	return m, err
}

// fetch fetches a module using the configured ModuleGetters.
// It tries each getter in turn until it finds one that has the module.
func (ds *FetchDataSource) fetch(ctx context.Context, modulePath, version string) (_ *internal.Module, g fetch.ModuleGetter, err error) {
	log.Infof(ctx, "FetchDataSource: fetching %s@%s", modulePath, version)
	start := time.Now()
	defer func() {
		log.Infof(ctx, "FetchDataSource: fetched %s@%s using %T in %s with error %v", modulePath, version, g, time.Since(start), err)
	}()
	for _, g := range ds.opts.Getters {
		fr := fetch.FetchModule(ctx, modulePath, version, g)
		if fr.Error == nil {
			m := fr.Module
			if ds.opts.BypassLicenseCheck {
				m.IsRedistributable = true
				for _, unit := range m.Units {
					unit.IsRedistributable = true
				}
			} else {
				m.RemoveNonRedistributableData()
			}
			return m, g, nil
		}
		if !errors.Is(fr.Error, derrors.NotFound) {
			return nil, g, fr.Error
		}
	}
	return nil, nil, fmt.Errorf("%s@%s: %w", modulePath, version, derrors.NotFound)
}

func (ds *FetchDataSource) populateUnitSubdirectories(u *internal.Unit, m *internal.Module) {
	p := u.Path + "/"
	for _, u2 := range m.Units {
		if strings.HasPrefix(u2.Path, p) || u.Path == "std" {
			var syn string
			if len(u2.Documentation) > 0 {
				syn = u2.Documentation[0].Synopsis
			}
			u.Subdirectories = append(u.Subdirectories, &internal.PackageMeta{
				Path:              u2.Path,
				Name:              u2.Name,
				Synopsis:          syn,
				IsRedistributable: u2.IsRedistributable,
				Licenses:          u2.Licenses,
			})
		}
	}
}

// findModule finds the module with longest module path containing the given
// package path. It returns an error if no module is found.
func (ds *FetchDataSource) findModule(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.Module, err error) {
	defer derrors.Wrap(&err, "FetchDataSource.findModule(%q, %q, %q)", pkgPath, modulePath, version)

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
func (ds *FetchDataSource) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "FetchDataSource.GetUnitMeta(%q, %q, %q)", path, requestedModulePath, requestedVersion)

	module, err := ds.findModule(ctx, path, requestedModulePath, requestedVersion)
	if err != nil {
		return nil, err
	}
	um := &internal.UnitMeta{
		Path:       path,
		ModuleInfo: module.ModuleInfo,
	}
	u := findUnit(module, path)
	if u == nil {
		return nil, derrors.NotFound
	}
	um.Name = u.Name
	um.IsRedistributable = u.IsRedistributable
	return um, nil
}

// GetUnit returns information about a unit. Both the module path and package
// path must be known.
func (ds *FetchDataSource) GetUnit(ctx context.Context, um *internal.UnitMeta, fields internal.FieldSet, bc internal.BuildContext) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "FetchDataSource.GetUnit(%q, %q)", um.Path, um.ModulePath)

	m, err := ds.getModule(ctx, um.ModulePath, um.Version)
	if err != nil {
		return nil, err
	}
	u := findUnit(m, um.Path)
	if u == nil {
		return nil, fmt.Errorf("import path %s not found in module %s: %w", um.Path, um.ModulePath, derrors.NotFound)
	}
	// Return only the Documentation matching the given BuildContext, if any.
	// Since we cache the module and its units, we have to copy this unit before we modify it.
	// It can be a shallow copy, since we're only modifying the Unit.Documentation field.
	u2 := *u
	if d := matchingDoc(u.Documentation, bc); d != nil {
		u2.Documentation = []*internal.Documentation{d}
	} else {
		u2.Documentation = nil
	}
	return &u2, nil
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

// matchingDoc returns the Documentation that matches the given build context
// and comes earliest in build-context order. It returns nil if there is none.
func matchingDoc(docs []*internal.Documentation, bc internal.BuildContext) *internal.Documentation {
	var (
		dMin  *internal.Documentation
		bcMin = internal.BuildContext{GOOS: "unk", GOARCH: "unk"} // sorts last
	)
	for _, d := range docs {
		dbc := d.BuildContext()
		if bc.Match(dbc) && internal.CompareBuildContexts(dbc, bcMin) < 0 {
			dMin = d
			bcMin = dbc
		}
	}
	return dMin
}

// GetLatestInfo returns latest information for unitPath and modulePath.
func (ds *FetchDataSource) GetLatestInfo(ctx context.Context, unitPath, modulePath string, latestUnitMeta *internal.UnitMeta) (latest internal.LatestInfo, err error) {
	defer derrors.Wrap(&err, "FetchDataSource.GetLatestInfo(ctx, %q, %q)", unitPath, modulePath)

	if ds.opts.ProxyClientForLatest == nil {
		return internal.LatestInfo{}, nil
	}

	if latestUnitMeta == nil {
		latestUnitMeta, err = ds.GetUnitMeta(ctx, unitPath, modulePath, version.Latest)
		if err != nil {
			return latest, err
		}
	}
	latest.MinorVersion = latestUnitMeta.Version
	latest.MinorModulePath = latestUnitMeta.ModulePath

	latest.MajorModulePath, latest.MajorUnitPath, err = ds.getLatestMajorVersion(ctx, unitPath, modulePath)
	if err != nil {
		return latest, err
	}
	// Do not try to discover whether the unit is in the latest minor version; assume it is.
	latest.UnitExistsAtMinor = true
	return latest, nil
}

// getLatestMajorVersion returns the latest module path and the full package path
// of the latest version found in the proxy by iterating through vN versions.
// This function does not attempt to find whether the full path exists
// in the new major version.
func (ds *FetchDataSource) getLatestMajorVersion(ctx context.Context, fullPath, modulePath string) (_ string, _ string, err error) {
	// We are checking if the full path is valid so that we can forward the error if not.
	seriesPath := internal.SeriesPathForModule(modulePath)
	info, err := ds.opts.ProxyClientForLatest.Info(ctx, seriesPath, version.Latest)
	if err != nil {
		return "", "", err
	}

	// Converting version numbers to integers may cause an overflow, as version
	// numbers need not fit into machine integers.
	// While using Atoi is wrong, for it to fail, the version number must reach a
	// value higher than at least 2^31, which is unlikely.
	startVersion, err := strconv.Atoi(strings.TrimPrefix(semver.Major(info.Version), "v"))
	if err != nil {
		return "", "", err
	}
	startVersion++

	// We start checking versions from "/v2" or higher, since v1 and v0 versions
	// don't have a major version at the end of the modulepath.
	if startVersion < 2 {
		startVersion = 2
	}

	for v := startVersion; ; v++ {
		query := fmt.Sprintf("%s/v%d", seriesPath, v)

		_, err := ds.opts.ProxyClientForLatest.Info(ctx, query, version.Latest)
		if errors.Is(err, derrors.NotFound) {
			if v == 2 {
				return modulePath, fullPath, nil
			}
			latestModulePath := fmt.Sprintf("%s/v%d", seriesPath, v-1)
			return latestModulePath, latestModulePath, nil
		}
		if err != nil {
			return "", "", err
		}
	}
}

// GetNestedModules is not implemented.
func (ds *FetchDataSource) GetNestedModules(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	return nil, nil
}

// GetModuleReadme is not implemented.
func (*FetchDataSource) GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (*internal.Readme, error) {
	return nil, nil
}

// SupportsSearch reports whether any of the configured Getters are searchable.
func (ds *FetchDataSource) SearchSupport() internal.SearchSupport {
	for _, g := range ds.opts.Getters {
		if _, ok := g.(fetch.SearchableModuleGetter); ok {
			// Getters only support basic search.
			return internal.BasicSearch
		}
	}
	return internal.NoSearch
}

// Search delegates search to any configured getters that support the
// SearchableModuleGetter interface, merging their results.
func (ds *FetchDataSource) Search(ctx context.Context, q string, opts internal.SearchOptions) (_ []*internal.SearchResult, err error) {
	var results []*internal.SearchResult
	// Since results are potentially merged from multiple sources, we can't know
	// a priori how many results will be used from any particular getter.
	//
	// Offset+MaxResults is an upper bound.
	limit := opts.Offset + opts.MaxResults
	for _, g := range ds.opts.Getters {
		if s, ok := g.(fetch.SearchableModuleGetter); ok {
			rs, err := s.Search(ctx, q, limit)
			if err != nil {
				return nil, err
			}
			results = append(results, rs...)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if opts.Offset > 0 {
		if len(results) < opts.Offset {
			return nil, nil
		}
		results = results[opts.Offset:]
	}
	if opts.MaxResults > 0 && len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results, nil
}
