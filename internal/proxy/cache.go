// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"archive/zip"
	"sync"

	"golang.org/x/pkgsite/internal"
)

// cache caches proxy info, mod and zip calls.
type cache struct {
	mu sync.Mutex

	infoCache map[internal.Modver]*VersionInfo
	modCache  map[internal.Modver][]byte

	// One-element zip cache, to avoid a double download.
	// See TestFetchAndUpdateStateCacheZip in internal/worker/fetch_test.go.
	zipKey    internal.Modver
	zipReader *zip.Reader
}

func (c *cache) getInfo(modulePath, version string) *VersionInfo {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.infoCache[internal.Modver{Path: modulePath, Version: version}]
}

func (c *cache) putInfo(modulePath, version string, v *VersionInfo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.infoCache == nil {
		c.infoCache = map[internal.Modver]*VersionInfo{}
	}
	c.infoCache[internal.Modver{Path: modulePath, Version: version}] = v
}

func (c *cache) getMod(modulePath, version string) []byte {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.modCache[internal.Modver{Path: modulePath, Version: version}]
}

func (c *cache) putMod(modulePath, version string, b []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.modCache == nil {
		c.modCache = map[internal.Modver][]byte{}
	}
	c.modCache[internal.Modver{Path: modulePath, Version: version}] = b
}

func (c *cache) getZip(modulePath, version string) *zip.Reader {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.zipKey == (internal.Modver{Path: modulePath, Version: version}) {
		return c.zipReader
	}
	return nil
}

func (c *cache) putZip(modulePath, version string, r *zip.Reader) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.zipKey = internal.Modver{Path: modulePath, Version: version}
	c.zipReader = r
}
