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
	return newProxyDataSource(proxyClient, source.NewClient(1*time.Minute), false)
}

func NewForTesting(proxyClient *proxy.Client, bypassLicenseCheck bool) *ProxyDataSource {
	return newProxyDataSource(proxyClient, source.NewClientForTesting(), bypassLicenseCheck)
}

func newProxyDataSource(proxyClient *proxy.Client, sourceClient *source.Client, bypassLicenseCheck bool) *ProxyDataSource {
	ds := newDataSource([]fetch.ModuleGetter{fetch.NewProxyModuleGetter(proxyClient)}, sourceClient, bypassLicenseCheck, proxyClient)
	return &ProxyDataSource{
		ds: ds,
	}
}

// NewBypassingLicenseCheck returns a new direct proxy datasource that bypasses
// license checks. That means all data will be returned for non-redistributable
// modules, packages and directories.
func NewBypassingLicenseCheck(c *proxy.Client) *ProxyDataSource {
	return newProxyDataSource(c, source.NewClient(1*time.Minute), true)
}

// ProxyDataSource implements the frontend.DataSource interface, by querying a
// module proxy directly and caching the results in memory.
type ProxyDataSource struct {
	ds *dataSource
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
	info, err := ds.ds.prox.Info(ctx, seriesPath, version.Latest)
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

		_, err := ds.ds.prox.Info(ctx, query, version.Latest)
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
