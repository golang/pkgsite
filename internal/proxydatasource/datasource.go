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
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
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
	}
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
}

type versionKey struct {
	modulePath, version string
}

// versionEntry holds the result of a call to worker.FetchModule.
type versionEntry struct {
	module *internal.Module
	err    error
}

// LegacyGetDirectory returns packages contained in the given subdirectory of a module version.
func (ds *DataSource) LegacyGetDirectory(ctx context.Context, dirPath, modulePath, version string, _ internal.FieldSet) (_ *internal.LegacyDirectory, err error) {
	defer derrors.Wrap(&err, "LegacyGetDirectory(%q, %q, %q)", dirPath, modulePath, version)

	var info *proxy.VersionInfo
	if modulePath == internal.UnknownModulePath {
		modulePath, info, err = ds.findModule(ctx, dirPath, version)
		if err != nil {
			return nil, err
		}
		version = info.Version
	}
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return &internal.LegacyDirectory{
		LegacyModuleInfo: internal.LegacyModuleInfo{ModuleInfo: v.ModuleInfo},
		Path:             dirPath,
		Packages:         v.LegacyPackages,
	}, nil
}

// GetDirectoryNew returns information about a directory at a path.
func (ds *DataSource) GetDirectoryNew(ctx context.Context, dirPath, modulePath, version string) (_ *internal.VersionedDirectory, err error) {
	m, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return &internal.VersionedDirectory{
		ModuleInfo: m.ModuleInfo,
		DirectoryNew: internal.DirectoryNew{
			Path:   dirPath,
			V1Path: internal.V1Path(modulePath, strings.TrimPrefix(dirPath, modulePath+"/")),
		},
	}, nil
}

// GetImports returns package imports as extracted from the module zip.
func (ds *DataSource) GetImports(ctx context.Context, pkgPath, modulePath, version string) (_ []string, err error) {
	defer derrors.Wrap(&err, "GetImports(%q, %q, %q)", pkgPath, modulePath, version)
	vp, err := ds.LegacyGetPackage(ctx, pkgPath, modulePath, version)
	if err != nil {
		return nil, err
	}
	return vp.Imports, nil
}

// LegacyGetModuleLicenses returns root-level licenses detected within the module zip
// for modulePath and version.
func (ds *DataSource) LegacyGetModuleLicenses(ctx context.Context, modulePath, version string) (_ []*licenses.License, err error) {
	defer derrors.Wrap(&err, "LegacyGetModuleLicenses(%q, %q)", modulePath, version)
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	var filtered []*licenses.License
	for _, lic := range v.Licenses {
		if !strings.Contains(lic.FilePath, "/") {
			filtered = append(filtered, lic)
		}
	}
	return filtered, nil
}

// LegacyGetPackage returns a LegacyVersionedPackage for the given pkgPath and version. If
// such a package exists in the cache, it will be returned without querying the
// proxy. Otherwise, the proxy is queried to find the longest module path at
// that version containing the package.
func (ds *DataSource) LegacyGetPackage(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.LegacyVersionedPackage, err error) {
	defer derrors.Wrap(&err, "LegacyGetPackage(%q, %q)", pkgPath, version)

	var m *internal.Module
	if modulePath != internal.UnknownModulePath {
		m, err = ds.getModule(ctx, modulePath, version)
	} else {
		m, err = ds.getPackageVersion(ctx, pkgPath, version)
	}
	if err != nil {
		return nil, err
	}
	return packageFromVersion(pkgPath, m)
}

// LegacyGetPackageLicenses returns the Licenses that apply to pkgPath within the
// module version specified by modulePath and version.
func (ds *DataSource) LegacyGetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) (_ []*licenses.License, err error) {
	defer derrors.Wrap(&err, "LegacyGetPackageLicenses(%q, %q, %q)", pkgPath, modulePath, version)
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	for _, p := range v.LegacyPackages {
		if p.Path == pkgPath {
			var lics []*licenses.License
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
	return nil, fmt.Errorf("package %s is missing from module %s: %w", pkgPath, modulePath, derrors.NotFound)
}

// LegacyGetPackagesInModule returns LegacyPackages contained in the module zip corresponding to modulePath and version.
func (ds *DataSource) LegacyGetPackagesInModule(ctx context.Context, modulePath, version string) (_ []*internal.LegacyPackage, err error) {
	defer derrors.Wrap(&err, "LegacyGetPackagesInModule(%q, %q)", modulePath, version)
	v, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return v.LegacyPackages, nil
}

// GetPseudoVersionsForModule returns versions from the the proxy /list
// endpoint, if they are pseudoversions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetPseudoVersionsForModule(ctx context.Context, modulePath string) (_ []*internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetPseudoVersionsForModule(%q)", modulePath)
	return ds.listModuleVersions(ctx, modulePath, true)
}

// GetPseudoVersionsForPackageSeries finds the longest module path containing
// pkgPath, and returns its versions from the proxy /list endpoint, if they are
// pseudoversions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetPseudoVersionsForPackageSeries(ctx context.Context, pkgPath string) (_ []*internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetPseudoVersionsForPackageSeries(%q)", pkgPath)
	return ds.listPackageVersions(ctx, pkgPath, true)
}

// GetTaggedVersionsForModule returns versions from the the proxy /list
// endpoint, if they are tagged versions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetTaggedVersionsForModule(ctx context.Context, modulePath string) (_ []*internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetTaggedVersionsForModule(%q)", modulePath)
	return ds.listModuleVersions(ctx, modulePath, false)
}

// GetTaggedVersionsForPackageSeries finds the longest module path containing
// pkgPath, and returns its versions from the proxy /list endpoint, if they are
// tagged versions. Otherwise, it returns an empty slice.
func (ds *DataSource) GetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) (_ []*internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetTaggedVersionsForPackageSeries(%q)", pkgPath)
	return ds.listPackageVersions(ctx, pkgPath, false)
}

// LegacyGetModuleInfo returns the LegacyModuleInfo as fetched from the proxy for module
// version specified by modulePath and version.
func (ds *DataSource) LegacyGetModuleInfo(ctx context.Context, modulePath, version string) (_ *internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetModuleInfo(%q, %q)", modulePath, version)
	m, err := ds.getModule(ctx, modulePath, version)
	if err != nil {
		return nil, err
	}
	return &m.LegacyModuleInfo, nil
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
func (ds *DataSource) listPackageVersions(ctx context.Context, pkgPath string, pseudo bool) (_ []*internal.LegacyModuleInfo, err error) {
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
func (ds *DataSource) listModuleVersions(ctx context.Context, modulePath string, pseudo bool) (_ []*internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "listModuleVersions(%q, %t)", modulePath, pseudo)
	versions, err := ds.proxyClient.ListVersions(ctx, modulePath)
	if err != nil {
		return nil, err
	}
	var vis []*internal.LegacyModuleInfo
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
			vis = append(vis, &v.module.LegacyModuleInfo)
		} else {
			// In this case we can't produce s LegacyModuleInfo without fully processing
			// the module zip, so we instead append a stub. We could further query
			// for this version's /info endpoint to get commit time, but that is
			// deferred as a potential future enhancement.
			vis = append(vis, &internal.LegacyModuleInfo{
				ModuleInfo: internal.ModuleInfo{
					ModulePath: modulePath,
					Version:    vers,
				},
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

// GetExperiments is unimplemented.
func (*DataSource) GetExperiments(ctx context.Context) ([]*internal.Experiment, error) {
	return nil, nil
}

// GetPathInfo returns information about the given path.
func (ds *DataSource) GetPathInfo(ctx context.Context, path, inModulePath, inVersion string) (outModulePath, outVersion string, isPackage bool, err error) {
	defer derrors.Wrap(&err, "GetPathInfo(%q, %q, %q)", path, inModulePath, inVersion)

	var info *proxy.VersionInfo
	if inModulePath == internal.UnknownModulePath {
		inModulePath, info, err = ds.findModule(ctx, path, inVersion)
		if err != nil {
			return "", "", false, err
		}
		inVersion = info.Version
	}
	m, err := ds.getModule(ctx, inModulePath, inVersion)
	if err != nil {
		return "", "", false, err
	}
	isPackage = false
	for _, p := range m.LegacyPackages {
		if p.Path == path {
			isPackage = true
			break
		}
	}
	return m.ModulePath, m.Version, isPackage, nil
}
