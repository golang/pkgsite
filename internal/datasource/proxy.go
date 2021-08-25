// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package datasource

import (
	"context"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/fetch"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
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
	return ds.ds.GetLatestInfo(ctx, unitPath, modulePath, latestUnitMeta)
}
