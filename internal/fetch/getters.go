// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"context"
	"io/fs"

	"golang.org/x/pkgsite/internal/proxy"
)

type proxyModuleGetter struct {
	prox *proxy.Client
}

func NewProxyModuleGetter(p *proxy.Client) ModuleGetter {
	return &proxyModuleGetter{p}
}

// Info returns basic information about the module.
func (g *proxyModuleGetter) Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error) {
	return g.prox.Info(ctx, path, version)
}

// Mod returns the contents of the module's go.mod file.
func (g *proxyModuleGetter) Mod(ctx context.Context, path, version string) ([]byte, error) {
	return g.prox.Mod(ctx, path, version)
}

// FS returns an FS for the module's contents. The FS should match the format
// of a module zip file.
func (g *proxyModuleGetter) FS(ctx context.Context, path, version string) (fs.FS, error) {
	return g.prox.Zip(ctx, path, version)
}

// ZipSize returns the approximate size of the zip file in bytes.
// It is used only for load-shedding.
func (g *proxyModuleGetter) ZipSize(ctx context.Context, path, version string) (int64, error) {
	return g.prox.ZipSize(ctx, path, version)
}
