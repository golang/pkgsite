// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"fmt"
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
	"golang.org/x/pkgsite/internal/version"
)

var _ internal.DataSource = (*ProxyDataSource)(nil)

// New returns a new direct proxy datasource.
func NewProxy(proxyClient *proxy.Client) *ProxyDataSource {
	return newProxyDataSource(proxyClient, source.NewClient(1*time.Minute))
}

func NewForTesting(proxyClient *proxy.Client) *ProxyDataSource {
	return newProxyDataSource(proxyClient, source.NewClientForTesting())
}

func newProxyDataSource(proxyClient *proxy.Client, sourceClient *source.Client) *ProxyDataSource {
	ds := newDataSource([]fetch.ModuleGetter{fetch.NewProxyModuleGetter(proxyClient)}, sourceClient)
	return &ProxyDataSource{
		ds:                 ds,
		proxyClient:        proxyClient,
		bypassLicenseCheck: false,
	}
}

// NewBypassingLicenseCheck returns a new direct proxy datasource that bypasses
// license checks. That means all data will be returned for non-redistributable
// modules, packages and directories.
func NewBypassingLicenseCheck(c *proxy.Client) *ProxyDataSource {
	ds := NewProxy(c)
	ds.bypassLicenseCheck = true
	return ds
}

// ProxyDataSource implements the frontend.DataSource interface, by querying a
// module proxy directly and caching the results in memory.
type ProxyDataSource struct {
	proxyClient *proxy.Client

	mu                 sync.Mutex
	ds                 *dataSource
	bypassLicenseCheck bool
}

// getModule retrieves a version from the cache, or failing that queries and
// processes the version from the proxy.
func (ds *ProxyDataSource) getModule(ctx context.Context, modulePath, version string, _ internal.BuildContext) (_ *internal.Module, err error) {
	defer derrors.Wrap(&err, "getModule(%q, %q)", modulePath, version)

	ds.mu.Lock()
	defer ds.mu.Unlock()

	mod, err := ds.ds.cacheGet(modulePath, version)
	if mod != nil || err != nil {
		return mod, err
	}
	res := fetch.FetchModule(ctx, modulePath, version, ds.ds.getters[0], ds.ds.sourceClient)
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
		//
		// Use the go.mod file at the raw latest version to fill in deprecation
		// and retraction information.
		lmv, err := fetch.LatestModuleVersions(ctx, modulePath, ds.proxyClient, nil)
		if err != nil {
			res.Error = err
		} else {
			lmv.PopulateModuleInfo(&m.ModuleInfo)
		}
	}

	if res.Error != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			ds.ds.cachePut(modulePath, version, m, res.Error)
		}
		return nil, res.Error
	}
	ds.ds.cachePut(modulePath, version, m, err)

	return m, nil
}

// findModule finds the longest module path containing the given package path,
// using the given finder func and iteratively testing parent directories of
// the import path. It performs no testing as to whether the specified module
// version that was found actually contains a package corresponding to pkgPath.
func (ds *ProxyDataSource) findModule(ctx context.Context, pkgPath string, version string) (_ string, _ *proxy.VersionInfo, err error) {
	defer derrors.Wrap(&err, "findModule(%q, ...)", pkgPath)
	pkgPath = strings.TrimLeft(pkgPath, "/")
	for _, modulePath := range internal.CandidateModulePaths(pkgPath) {
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
func (ds *ProxyDataSource) getUnit(ctx context.Context, fullPath, modulePath, version string, bc internal.BuildContext) (_ *internal.Unit, err error) {
	var m *internal.Module
	m, err = ds.getModule(ctx, modulePath, version, bc)
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
func (ds *ProxyDataSource) GetLatestInfo(ctx context.Context, unitPath, modulePath string, latestUnitMeta *internal.UnitMeta) (latest internal.LatestInfo, err error) {
	defer derrors.Wrap(&err, "GetLatestInfo(ctx, %q, %q)", unitPath, modulePath)

	if latestUnitMeta == nil {
		latestUnitMeta, err = ds.GetUnitMeta(ctx, unitPath, internal.UnknownModulePath, version.Latest)
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
func (ds *ProxyDataSource) getLatestMajorVersion(ctx context.Context, fullPath, modulePath string) (_ string, _ string, err error) {
	// We are checking if the full path is valid so that we can forward the error if not.
	seriesPath := internal.SeriesPathForModule(modulePath)
	info, err := ds.proxyClient.Info(ctx, seriesPath, version.Latest)
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

		_, err := ds.proxyClient.Info(ctx, query, version.Latest)
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
