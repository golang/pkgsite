// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxydatasource implements a frontend.DataSource backed solely by a
// proxy instance.
package proxydatasource

import (
	"context"
	"path"
	"sort"
	"strings"
	"sync"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/etl"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/xerrors"
)

// New returns a new direct proxy datasource.
func New(proxyClient *proxy.Client) *DataSource {
	return &DataSource{
		proxyClient:          proxyClient,
		versionCache:         make(map[versionKey]*versionEntry),
		modulePathToVersions: make(map[string][]string),
		packagePathToModules: make(map[string][]string),
	}
}

// DataSource implements the frontend.DataSource interface, by querying a
// module proxy directly and caching the results in memory.
type DataSource struct {
	proxyClient *proxy.Client

	// Use an extremely coarse lock for now - mu guards all maps below. The
	// assumption is that this will only be used for local development.
	mu           sync.RWMutex
	versionCache map[versionKey]*versionEntry
	// map of modulePath -> versions, with versions sorted in semver order
	modulePathToVersions map[string][]string
	// map of package path -> modules paths containing it, with module paths
	// sorted by descending length
	packagePathToModules map[string][]string
}

type versionKey struct {
	modulePath, version string
}

// versionEntry holds the result of a call to etl.FetchVersion.
type versionEntry struct {
	version *internal.Version
	err     error
}

// GetDirectory returns packages contained in the given subdirectory of a module version.
func (ds *DataSource) GetDirectory(ctx context.Context, dirPath, version string) (_ *internal.Directory, err error) {
	defer derrors.Wrap(&err, "GetDirectory(%q, %q)", dirPath, version)
	modulePath, info, err := ds.findModule(ctx, dirPath, version)
	if err != nil {
		return nil, err
	}
	v, err := ds.getVersion(ctx, modulePath, info.Version)
	if err != nil {
		return nil, err
	}
	var vps []*internal.VersionedPackage
	for _, p := range v.Packages {
		if strings.HasPrefix(p.Path+"/", dirPath+"/") {
			vps = append(vps, &internal.VersionedPackage{
				Package:     *p,
				VersionInfo: v.VersionInfo,
			})
		}
	}
	return &internal.Directory{
		Path:       dirPath,
		ModulePath: modulePath,
		Version:    info.Version,
		Packages:   vps,
	}, nil
}

// GetImportedBy is unimplemented.
func (ds *DataSource) GetImportedBy(ctx context.Context, path, version string, limit int) (_ []string, err error) {
	return []string{}, nil
}

// GetImports returns package imports as extracted from the module zip.
func (ds *DataSource) GetImports(ctx context.Context, pkgPath, version string) (_ []string, err error) {
	defer derrors.Wrap(&err, "GetImports(%q, %q)", pkgPath, version)
	vp, err := ds.GetPackage(ctx, pkgPath, version)
	if err != nil {
		return nil, err
	}
	return vp.Imports, nil
}

// GetModuleLicenses returns root-level licenses detected within the module zip
// for modulePath and version.
func (ds *DataSource) GetModuleLicenses(ctx context.Context, modulePath, version string) (_ []*license.License, err error) {
	defer derrors.Wrap(&err, "GetModuleLicenses(%q, %q)", modulePath, version)
	v, err := ds.getVersion(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	var filtered []*license.License
	for _, lic := range v.Licenses {
		if !strings.Contains(lic.FilePath, "/") {
			filtered = append(filtered, lic)
		}
	}
	return filtered, nil
}

// GetPackage returns a VersionedPackage for the given pkgPath and version. If
// such a package exists in the cache, it will be returned without querying the
// proxy. Otherwise, the proxy is queried to find the longest module path at
// that version containing the package.
func (ds *DataSource) GetPackage(ctx context.Context, pkgPath, version string) (_ *internal.VersionedPackage, err error) {
	defer derrors.Wrap(&err, "GetPackage(%q, %q)", pkgPath, version)
	v, err := ds.getPackageVersion(ctx, pkgPath, version)
	if err != nil {
		return nil, err
	}
	return packageFromVersion(pkgPath, v)
}

// GetPackageLicenses returns the Licenses that apply to pkgPath within the
// module version specified by modulePath and version.
func (ds *DataSource) GetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) (_ []*license.License, err error) {
	defer derrors.Wrap(&err, "GetPackageLicenses(%q, %q, %q)", pkgPath, modulePath, version)
	v, err := ds.getVersion(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	for _, p := range v.Packages {
		if p.Path == pkgPath {
			var lics []*license.License
			for _, lmd := range p.Licenses {
				// lmd is just license metadata, the version has the actual licenses.
				for _, lic := range v.Licenses {
					if lic.FilePath == lmd.FilePath {
						lics = append(lics, lic)
						break
					}
				}
			}
			return lics, nil
		}
	}
	return nil, xerrors.Errorf("package %s is missing from module %s: %w", pkgPath, modulePath, derrors.NotFound)
}

// GetPackagesInVersion returns Packages contained in the module zip corresponding to modulePath and version.
func (ds *DataSource) GetPackagesInVersion(ctx context.Context, modulePath, version string) (_ []*internal.Package, err error) {
	defer derrors.Wrap(&err, "GetPackagesInVersion(%q, %q)", modulePath, version)
	v, err := ds.getVersion(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return v.Packages, nil
}

// GetPseudoVersionsForModule returns versions from the the proxy /list
// endpoint, if they are pseudoversions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetPseudoVersionsForModule(ctx context.Context, modulePath string) (_ []*internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "GetPseudoVersionsForModule(%q)", modulePath)
	return ds.listModuleVersions(ctx, modulePath, true)
}

// GetPseudoVersionsForPackageSeries finds the longest module path containing
// pkgPath, and returns its versions from the proxy /list endpoint, if they are
// pseudoversions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetPseudoVersionsForPackageSeries(ctx context.Context, pkgPath string) (_ []*internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "GetPseudoVersionsForPackageSeries(%q)", pkgPath)
	return ds.listPackageVersions(ctx, pkgPath, true)
}

// GetTaggedVersionsForModule returns versions from the the proxy /list
// endpoint, if they are tagged versions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetTaggedVersionsForModule(ctx context.Context, modulePath string) (_ []*internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "GetTaggedVersionsForModule(%q)", modulePath)
	return ds.listModuleVersions(ctx, modulePath, false)
}

// GetTaggedVersionsForPackageSeries finds the longest module path containing
// pkgPath, and returns its versions from the proxy /list endpoint, if they are
// tagged versions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) (_ []*internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "GetTaggedVersionsForPackageSeries(%q)", pkgPath)
	return ds.listPackageVersions(ctx, pkgPath, false)
}

// GetVersionInfo returns the VersionInfo as fetched from the proxy for module
// version specified by modulePath and version.
func (ds *DataSource) GetVersionInfo(ctx context.Context, modulePath, version string) (_ *internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "GetVersionInfo(%q, %q)", modulePath, version)
	v, err := ds.getVersion(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return &v.VersionInfo, nil
}

// LegacySearch is unimplemented.
func (ds *DataSource) LegacySearch(ctx context.Context, query string, limit, offset int) ([]*postgres.SearchResult, error) {
	return []*postgres.SearchResult{}, nil
}

// Search is unimplemented.
func (ds *DataSource) Search(ctx context.Context, query string, limit, offset int) ([]*postgres.SearchResult, error) {
	return []*postgres.SearchResult{}, nil
}

// FastSearch is unimplemented.
func (ds *DataSource) FastSearch(ctx context.Context, query string, limit, offset int) ([]*postgres.SearchResult, error) {
	return []*postgres.SearchResult{}, nil
}

// getVersion retrieves a version from the cache, or failing that queries and
// processes the version from the proxy.
func (ds *DataSource) getVersion(ctx context.Context, modulePath, version string) (_ *internal.Version, err error) {
	defer derrors.Wrap(&err, "getVersion(%q, %q)", modulePath, version)

	key := versionKey{modulePath, version}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if e, ok := ds.versionCache[key]; ok {
		return e.version, e.err
	}

	v, _, err := etl.FetchVersion(ctx, modulePath, version, ds.proxyClient)
	ds.versionCache[key] = &versionEntry{version: v, err: err}
	if err != nil {
		return nil, err
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
	for _, pkg := range v.Packages {
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
	return v, nil
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
		if xerrors.Is(err, derrors.NotFound) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		return modulePath, info, nil
	}
	return "", nil, xerrors.Errorf("unable to find module: %w", derrors.NotFound)
}

// listPackageVersions finds the longest module corresponding to pkgPath, and
// calls the proxy /list endpoint to list its versions. If pseudo is true, it
// filters to pseudo versions.  If pseudo is false, it filters to tagged
// versions.
func (ds *DataSource) listPackageVersions(ctx context.Context, pkgPath string, pseudo bool) (_ []*internal.VersionInfo, err error) {
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
func (ds *DataSource) listModuleVersions(ctx context.Context, modulePath string, pseudo bool) (_ []*internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "listModuleVersions(%q, %t)", modulePath, pseudo)
	versions, err := ds.proxyClient.ListVersions(ctx, modulePath)
	if err != nil {
		return nil, err
	}
	var vis []*internal.VersionInfo
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	for _, vers := range versions {
		// In practice, the /list endpoint should only return either pseudo
		// versions or tagged versions, but we filter here for maximum
		// compatibility.
		if internal.IsPseudoVersion(vers) != pseudo {
			continue
		}
		if v, ok := ds.versionCache[versionKey{modulePath, vers}]; ok {
			vis = append(vis, &v.version.VersionInfo)
		} else {
			// In this case we can't produce s VersionInfo without fully processing
			// the module zip, so we instead append a stub. We could further query
			// for this version's /info endpoint to get commit time, but that is
			// deferred as a potential future enhancement.
			vis = append(vis, &internal.VersionInfo{
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
func (ds *DataSource) getPackageVersion(ctx context.Context, pkgPath, version string) (_ *internal.Version, err error) {
	defer derrors.Wrap(&err, "getPackageVersion(%q, %q)", pkgPath, version)
	// First, try to retrieve this version from the cache, using our reverse
	// indexes.
	if modulePath, ok := ds.findModulePathForPackage(pkgPath, version); ok {
		// This should hit the cache.
		return ds.getVersion(ctx, modulePath, version)
	}
	modulePath, info, err := ds.findModule(ctx, pkgPath, version)
	if err != nil {
		return nil, err
	}
	return ds.getVersion(ctx, modulePath, info.Version)
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

// packageFromVersion extracts the VersionedPackage for pkgPath from the
// Version payload.
func packageFromVersion(pkgPath string, v *internal.Version) (_ *internal.VersionedPackage, err error) {
	defer derrors.Wrap(&err, "packageFromVersion(%q, ...)", pkgPath)
	for _, p := range v.Packages {
		if p.Path == pkgPath {
			return &internal.VersionedPackage{
				Package:     *p,
				VersionInfo: v.VersionInfo,
			}, nil
		}
	}
	return nil, xerrors.Errorf("package missing from module %s: %w", v.ModulePath, derrors.NotFound)
}

func (*DataSource) IsExcluded(context.Context, string) (bool, error) {
	return false, nil
}
