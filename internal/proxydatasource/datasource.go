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
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
)

var _ internal.DataSource = (*DataSource)(nil)

// New returns a new direct proxy datasource.
func New(proxyClient *proxy.Client) *DataSource {
	return newDataSource(proxyClient, source.NewClient(1*time.Minute))
}

func NewForTesting(proxyClient *proxy.Client) *DataSource {
	return newDataSource(proxyClient, source.NewClientForTesting())
}

func newDataSource(proxyClient *proxy.Client, sourceClient *source.Client) *DataSource {
	return &DataSource{
		proxyClient:          proxyClient,
		sourceClient:         sourceClient,
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
	res := fetch.FetchModule(ctx, modulePath, version, ds.proxyClient, ds.sourceClient, false)
	defer res.Defer()
	m := res.Module
	if m != nil {
		if ds.bypassLicenseCheck {
			m.IsRedistributable = true
			for _, pkg := range m.Packages() {
				pkg.IsRedistributable = true
			}
		} else {
			m.RemoveNonRedistributableData()
		}
	}

	if res.Error != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			ds.versionCache[key] = &versionEntry{module: m, err: res.Error}
		}
		return nil, res.Error
	}
	ds.versionCache[key] = &versionEntry{module: m, err: err}

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
	for _, pkg := range m.Packages() {
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
		info, err := ds.proxyClient.Info(ctx, modulePath, version)
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

// GetLatestInfo returns latest information for unitPath and modulePath.
func (ds *DataSource) GetLatestInfo(ctx context.Context, unitPath, modulePath string) (latest internal.LatestInfo, err error) {
	defer derrors.Wrap(&err, "GetLatestInfo(ctx, %q, %q)", unitPath, modulePath)

	um, err := ds.GetUnitMeta(ctx, unitPath, internal.UnknownModulePath, internal.LatestVersion)
	if err != nil {
		return latest, err
	}
	latest.MinorVersion = um.Version
	latest.MinorModulePath = um.ModulePath

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
func (ds *DataSource) getLatestMajorVersion(ctx context.Context, fullPath, modulePath string) (_ string, _ string, err error) {
	// We are checking if the full path is valid so that we can forward the error if not.
	seriesPath := internal.SeriesPathForModule(modulePath)
	info, err := ds.proxyClient.Info(ctx, seriesPath, internal.LatestVersion)
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

		_, err := ds.proxyClient.Info(ctx, query, internal.LatestVersion)
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
