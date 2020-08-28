// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxydatasource implements an internal.DataSource backed solely by a
// proxy instance.
package proxydatasource

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

var _ internal.DataSource = (*DataSource)(nil)

// New returns a new direct proxy datasource.
func New(proxyClient *proxy.Client) *DataSource {
	return &DataSource{
		proxyClient:          proxyClient,
		sourceClient:         source.NewClient(1 * time.Minute),
		versionCache:         make(map[versionKey]*versionEntry),
		modulePathToVersions: make(map[string][]string),
		packagePathToModules: make(map[string][]string),
		bypassLicenseCheck:   false,
	}
}

// NewBypassingLicenseCheck returns a new direct proxy datasource that bypasses
// license checks. That means all data will be returned for non-redistributable
// modules, packages and directories.
func NewBypassingLicenseCheck(c *proxy.Client) *DataSource {
	ds := New(c)
	ds.bypassLicenseCheck = true
	return ds
}

// DataSource implements the frontend.DataSource interface, by querying a
// module proxy directly and caching the results in memory.
type DataSource struct {
	proxyClient  *proxy.Client
	sourceClient *source.Client

	// Use an extremely coarse lock for now - mu guards all maps below. The
	// assumption is that this will only be used for local development.
	mu           sync.RWMutex
	versionCache map[versionKey]*versionEntry
	// map of modulePath -> versions, with versions sorted in semver order
	modulePathToVersions map[string][]string
	// map of package path -> modules paths containing it, with module paths
	// sorted by descending length
	packagePathToModules map[string][]string
	bypassLicenseCheck   bool
}

type versionKey struct {
	modulePath, version string
}

// versionEntry holds the result of a call to worker.FetchModule.
type versionEntry struct {
	module *internal.Module
	err    error
}

// getModule retrieves a version from the cache, or failing that queries and
// processes the version from the proxy.
func (ds *DataSource) getModule(ctx context.Context, modulePath, version string) (_ *internal.Module, err error) {
	defer derrors.Wrap(&err, "getModule(%q, %q)", modulePath, version)

	key := versionKey{modulePath, version}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if e, ok := ds.versionCache[key]; ok {
		return e.module, e.err
	}
	res := fetch.FetchModule(ctx, modulePath, version, ds.proxyClient, ds.sourceClient)
	m := res.Module
	if m != nil && !ds.bypassLicenseCheck {
		m.RemoveNonRedistributableData()
	}
	ds.versionCache[key] = &versionEntry{module: m, err: err}
	if res.Error != nil {
		return nil, res.Error
	}

	// Since we hold the lock and missed the cache, we can assume that we have
	// never seen this module version. Therefore the following insert-and-sort
	// preserves uniqueness of versions in the module version list.
	newVersions := append(ds.modulePathToVersions[modulePath], version)
	sort.Slice(newVersions, func(i, j int) bool {
		return semver.Compare(newVersions[i], newVersions[j]) < 0
	})
	ds.modulePathToVersions[modulePath] = newVersions

	// Unlike the above, we don't know at this point whether or not we've seen
	// this module path for this particular package before. Therefore, we need to
	// be a bit more careful and check that it is new. To do this, we can
	// leverage the invariant that module paths in packagePathToModules are kept
	// sorted in descending order of length.
	for _, pkg := range m.LegacyPackages {
		var (
			i   int
			mp  string
			mps = ds.packagePathToModules[pkg.Path]
		)
		for i, mp = range mps {
			if len(mp) <= len(modulePath) {
				break
			}
		}
		if mp != modulePath {
			ds.packagePathToModules[pkg.Path] = append(mps[:i], append([]string{modulePath}, mps[i:]...)...)
		}
	}
	return m, nil
}

// findModule finds the longest module path containing the given package path,
// using the given finder func and iteratively testing parent directories of
// the import path. It performs no testing as to whether the specified module
// version that was found actually contains a package corresponding to pkgPath.
func (ds *DataSource) findModule(ctx context.Context, pkgPath string, version string) (_ string, _ *proxy.VersionInfo, err error) {
	defer derrors.Wrap(&err, "findModule(%q, ...)", pkgPath)
	pkgPath = strings.TrimLeft(pkgPath, "/")
	for modulePath := pkgPath; modulePath != "" && modulePath != "."; modulePath = path.Dir(modulePath) {
		info, err := ds.proxyClient.GetInfo(ctx, modulePath, version)
		if errors.Is(err, derrors.NotFound) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		return modulePath, info, nil
	}
	return "", nil, fmt.Errorf("unable to find module: %w", derrors.NotFound)
}

// listPackageVersions finds the longest module corresponding to pkgPath, and
// calls the proxy /list endpoint to list its versions. If pseudo is true, it
// filters to pseudo versions.  If pseudo is false, it filters to tagged
// versions.
func (ds *DataSource) listPackageVersions(ctx context.Context, pkgPath string, pseudo bool) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "listPackageVersions(%q, %t)", pkgPath, pseudo)
	ds.mu.RLock()
	mods := ds.packagePathToModules[pkgPath]
	ds.mu.RUnlock()
	var modulePath string
	if len(mods) > 0 {
		// Since mods is kept sorted, the first element is the longest module.
		modulePath = mods[0]
	} else {
		modulePath, _, err = ds.findModule(ctx, pkgPath, internal.LatestVersion)
		if err != nil {
			return nil, err
		}
	}
	return ds.listModuleVersions(ctx, modulePath, pseudo)
}

// listModuleVersions finds the longest module corresponding to pkgPath, and
// calls the proxy /list endpoint to list its versions. If pseudo is true, it
// filters to pseudo versions.  If pseudo is false, it filters to tagged
// versions.
func (ds *DataSource) listModuleVersions(ctx context.Context, modulePath string, pseudo bool) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "listModuleVersions(%q, %t)", modulePath, pseudo)
	var versions []string
	if modulePath == stdlib.ModulePath {
		versions, err = stdlib.Versions()
	} else {
		versions, err = ds.proxyClient.ListVersions(ctx, modulePath)
	}
	if err != nil {
		return nil, err
	}
	var vis []*internal.ModuleInfo
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	for _, vers := range versions {
		// In practice, the /list endpoint should only return either pseudo
		// versions or tagged versions, but we filter here for maximum
		// compatibility.
		if version.IsPseudo(vers) != pseudo {
			continue
		}
		if v, ok := ds.versionCache[versionKey{modulePath, vers}]; ok {
			vis = append(vis, &v.module.ModuleInfo)
		} else {
			// In this case we can't produce ModuleInfo without fully processing
			// the module zip, so we instead append a stub. We could further query
			// for this version's /info endpoint to get commit time, but that is
			// deferred as a potential future enhancement.
			vis = append(vis, &internal.ModuleInfo{
				ModulePath: modulePath,
				Version:    vers,
			})
		}
	}
	sort.Slice(vis, func(i, j int) bool {
		return semver.Compare(vis[i].Version, vis[j].Version) > 0
	})
	return vis, nil
}

// getPackageVersion finds a module at version that contains a package with
// import path pkgPath. To do this, it first checks the cache for any module
// satisfying this requirement, querying the proxy if none is found.
func (ds *DataSource) getPackageVersion(ctx context.Context, pkgPath, version string) (_ *internal.Module, err error) {
	defer derrors.Wrap(&err, "getPackageVersion(%q, %q)", pkgPath, version)
	// First, try to retrieve this version from the cache, using our reverse
	// indexes.
	if modulePath, ok := ds.findModulePathForPackage(pkgPath, version); ok {
		// This should hit the cache.
		return ds.getModule(ctx, modulePath, version)
	}
	modulePath, info, err := ds.findModule(ctx, pkgPath, version)
	if err != nil {
		return nil, err
	}
	return ds.getModule(ctx, modulePath, info.Version)
}

// findModulePathForPackage looks for an existing instance of a module at
// version that contains a package with path pkgPath. The return bool reports
// whether a valid module path was found.
func (ds *DataSource) findModulePathForPackage(pkgPath, version string) (string, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	for _, mp := range ds.packagePathToModules[pkgPath] {
		for _, vers := range ds.modulePathToVersions[mp] {
			if vers == version {
				return mp, true
			}
		}
	}
	return "", false
}

// packageFromVersion extracts the LegacyVersionedPackage for pkgPath from the
// Version payload.
func packageFromVersion(pkgPath string, m *internal.Module) (_ *internal.LegacyVersionedPackage, err error) {
	defer derrors.Wrap(&err, "packageFromVersion(%q, ...)", pkgPath)
	for _, p := range m.LegacyPackages {
		if p.Path == pkgPath {
			return &internal.LegacyVersionedPackage{
				LegacyPackage:    *p,
				LegacyModuleInfo: m.LegacyModuleInfo,
			}, nil
		}
	}
	return nil, fmt.Errorf("package missing from module %s: %w", m.ModulePath, derrors.NotFound)
}

// getUnit returns information about a unit.
func (ds *DataSource) getUnit(ctx context.Context, fullPath, modulePath, version string) (_ *internal.Unit, err error) {
	var m *internal.Module
	m, err = ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	for _, d := range m.Units {
		if d.Path == fullPath {
			return d, nil
		}
	}
	return nil, fmt.Errorf("%q missing from module %s: %w", fullPath, m.ModulePath, derrors.NotFound)
}

// GetLatestMajorVersion finds the latest major version of a modulePath that
// is found in the proxy by iterating through vN versions.
func (ds *DataSource) GetLatestMajorVersion(ctx context.Context, seriesPath string) (_ string, err error) {
	// We are checking if the series path is valid so that we can forward the error if not.
	_, err = ds.proxyClient.GetInfo(ctx, seriesPath, internal.LatestVersion)
	if err != nil {
		return "", err
	}
	const startVersion = 2
	// We start checking versions from "/v2", since v1 and v0 versions don't
	// have a major version at the end of the modulepath.
	for v := startVersion; ; v++ {
		query := fmt.Sprintf("%s/v%d", seriesPath, v)

		_, err := ds.proxyClient.GetInfo(ctx, query, internal.LatestVersion)
		if errors.Is(err, derrors.NotFound) {
			if v == 2 {
				return "", nil
			}
			return fmt.Sprintf("/v%d", v-1), nil
		}
		if err != nil {
			return "", err
		}
	}
}
