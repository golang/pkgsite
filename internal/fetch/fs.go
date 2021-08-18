// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
)

// An fsModuleGetter gets modules from a directory in the filesystem
// that is organized like the proxy, with paths that correspond to proxy
// URLs. An example of such a directory is $(go env GOMODCACHE)/cache/download.
type fsModuleGetter struct {
	dir string
}

// NewFSModuleGetter return a ModuleGetter that reads modules from a filesystem
// directory organized like the proxy.
func NewFSModuleGetter(dir string) ModuleGetter {
	return &fsModuleGetter{dir: dir}
}

// Info returns basic information about the module.
func (g *fsModuleGetter) Info(ctx context.Context, path, version string) (_ *proxy.VersionInfo, err error) {
	defer derrors.Wrap(&err, "fsModuleGetter.Info(%q, %q)", path, version)

	data, err := g.readFile(path, version, "info")
	if err != nil {
		return nil, err
	}
	var info proxy.VersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Mod returns the contents of the module's go.mod file.
func (g *fsModuleGetter) Mod(ctx context.Context, path, version string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "fsModuleGetter.Mod(%q, %q)", path, version)

	return g.readFile(path, version, "mod")
}

// Zip returns a reader for the module's zip file.
func (g *fsModuleGetter) Zip(ctx context.Context, path, version string) (_ *zip.Reader, err error) {
	defer derrors.Wrap(&err, "fsModuleGetter.Zip(%q, %q)", path, version)

	data, err := g.readFile(path, version, "zip")
	if err != nil {
		return nil, err
	}
	return zip.NewReader(bytes.NewReader(data), int64(len(data)))
}

// ZipSize returns the approximate size of the zip file in bytes.
func (g *fsModuleGetter) ZipSize(ctx context.Context, path, version string) (int64, error) {
	return 0, errors.New("fsModuleGetter.ZipSize unimplemented")
}

func (g *fsModuleGetter) readFile(path, version, suffix string) (_ []byte, err error) {
	epath, err := g.escapedPath(path, version, suffix)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(epath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

func (g *fsModuleGetter) escapedPath(modulePath, version, suffix string) (string, error) {
	ep, err := module.EscapePath(modulePath)
	if err != nil {
		return "", fmt.Errorf("path: %v: %w", err, derrors.InvalidArgument)
	}
	ev, err := module.EscapeVersion(version)
	if err != nil {
		return "", fmt.Errorf("version: %v: %w", err, derrors.InvalidArgument)
	}
	return filepath.Join(g.dir, ep, "@v", fmt.Sprintf("%s.%s", ev, suffix)), nil
}
