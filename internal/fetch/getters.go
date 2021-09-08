// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

// The ModuleGetter interface and its implementations.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/version"
)

// ModuleGetter gets module data.
type ModuleGetter interface {
	// Info returns basic information about the module.
	Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error)

	// Mod returns the contents of the module's go.mod file.
	Mod(ctx context.Context, path, version string) ([]byte, error)

	// ContentDir returns an FS for the module's contents. The FS should match the
	// format of a module zip file's content directory. That is the
	// "<module>@<resolvedVersion>" directory that all module zips are expected
	// to have according to the zip archive layout specification at
	// https://golang.org/ref/mod#zip-files.
	ContentDir(ctx context.Context, path, version string) (fs.FS, error)

	// SourceInfo returns information about where to find a module's repo and
	// source files.
	SourceInfo(ctx context.Context, path, version string) (*source.Info, error)

	// SourceFS returns the path to serve the files of the modules loaded by
	// this ModuleGetter, and an FS that can be used to read the files. The
	// returned values are intended to be passed to
	// internal/frontend.Server.InstallFiles.
	SourceFS() (string, fs.FS)
}

type proxyModuleGetter struct {
	prox *proxy.Client
	src  *source.Client
}

func NewProxyModuleGetter(p *proxy.Client, s *source.Client) ModuleGetter {
	return &proxyModuleGetter{p, s}
}

// Info returns basic information about the module.
func (g *proxyModuleGetter) Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error) {
	return g.prox.Info(ctx, path, version)
}

// Mod returns the contents of the module's go.mod file.
func (g *proxyModuleGetter) Mod(ctx context.Context, path, version string) ([]byte, error) {
	return g.prox.Mod(ctx, path, version)
}

// ContentDir returns an FS for the module's contents. The FS should match the format
// of a module zip file.
func (g *proxyModuleGetter) ContentDir(ctx context.Context, path, version string) (fs.FS, error) {
	zr, err := g.prox.Zip(ctx, path, version)
	if err != nil {
		return nil, err
	}
	return fs.Sub(zr, path+"@"+version)
}

// SourceInfo gets information about a module's repo and source files by calling source.ModuleInfo.
func (g *proxyModuleGetter) SourceInfo(ctx context.Context, path, version string) (*source.Info, error) {
	return source.ModuleInfo(ctx, g.src, path, version)
}

// SourceFS is unimplemented for modules served from the proxy, because we
// link directly to the module's repo.
func (g *proxyModuleGetter) SourceFS() (string, fs.FS) {
	return "", nil
}

// Version and commit time are pre specified when fetching a local module, as these
// fields are normally obtained from a proxy.
var (
	LocalVersion    = "v0.0.0"
	LocalCommitTime = time.Time{}
)

// A directoryModuleGetter is a ModuleGetter whose source is a directory in the file system that contains
// a module's files.
type directoryModuleGetter struct {
	modulePath string
	dir        string // absolute path to direction
}

// NewDirectoryModuleGetter returns a ModuleGetter for reading a module from a directory.
func NewDirectoryModuleGetter(modulePath, dir string) (*directoryModuleGetter, error) {

	if modulePath == "" {
		goModBytes, err := ioutil.ReadFile(filepath.Join(dir, "go.mod"))
		if err != nil {
			return nil, fmt.Errorf("cannot obtain module path for %q (%v): %w", dir, err, derrors.BadModule)
		}
		modulePath = modfile.ModulePath(goModBytes)
		if modulePath == "" {
			return nil, fmt.Errorf("go.mod in %q has no module path: %w", dir, derrors.BadModule)
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &directoryModuleGetter{
		dir:        abs,
		modulePath: modulePath,
	}, nil
}

func (g *directoryModuleGetter) checkPath(path string) error {
	if path != g.modulePath {
		return fmt.Errorf("given module path %q does not match %q for directory %q: %w",
			path, g.modulePath, g.dir, derrors.NotFound)
	}
	return nil
}

// Info returns basic information about the module.
func (g *directoryModuleGetter) Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error) {
	if err := g.checkPath(path); err != nil {
		return nil, err
	}
	return &proxy.VersionInfo{
		Version: LocalVersion,
		Time:    LocalCommitTime,
	}, nil
}

// Mod returns the contents of the module's go.mod file.
// If the file does not exist, it returns a synthesized one.
func (g *directoryModuleGetter) Mod(ctx context.Context, path, version string) ([]byte, error) {
	if err := g.checkPath(path); err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(filepath.Join(g.dir, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return []byte(fmt.Sprintf("module %s\n", g.modulePath)), nil
	}
	return data, err
}

// ContentDir returns an fs.FS for the module's contents.
func (g *directoryModuleGetter) ContentDir(ctx context.Context, path, version string) (fs.FS, error) {
	if err := g.checkPath(path); err != nil {
		return nil, err
	}
	return os.DirFS(g.dir), nil
}

// SourceInfo returns a source.Info that will link to the files in the
// directory. The files will be under /files/directory/modulePath, with no
// version.
func (g *directoryModuleGetter) SourceInfo(ctx context.Context, _, _ string) (*source.Info, error) {
	return source.FilesInfo(g.fileServingPath()), nil
}

// SourceFS returns the absolute path to the directory along with a
// filesystem FS for serving the directory.
func (g *directoryModuleGetter) SourceFS() (string, fs.FS) {
	return g.fileServingPath(), os.DirFS(g.dir)
}

func (g *directoryModuleGetter) fileServingPath() string {
	return path.Join(filepath.ToSlash(g.dir), g.modulePath)
}

// An fsProxyModuleGetter gets modules from a directory in the filesystem that
// is organized like the module cache, with a cache/download directory that has
// paths that correspond to proxy URLs. An example of such a directory is
// $(go env GOMODCACHE).
type fsProxyModuleGetter struct {
	dir string
}

// NewFSModuleGetter return a ModuleGetter that reads modules from a filesystem
// directory organized like the proxy.
func NewFSProxyModuleGetter(dir string) (_ *fsProxyModuleGetter, err error) {
	derrors.Wrap(&err, "NewFSProxyModuleGetter(%q)", dir)

	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &fsProxyModuleGetter{dir: abs}, nil
}

// Info returns basic information about the module.
func (g *fsProxyModuleGetter) Info(ctx context.Context, path, vers string) (_ *proxy.VersionInfo, err error) {
	defer derrors.Wrap(&err, "fsProxyModuleGetter.Info(%q, %q)", path, vers)

	if vers == version.Latest {
		vers, err = g.latestVersion(path)
		if err != nil {
			return nil, err
		}
	}

	// Check for a .zip file. Some directories in the download cache have .info and .mod files but no .zip.
	f, err := g.openFile(path, vers, "zip")
	if err != nil {
		return nil, err
	}
	f.Close()
	data, err := g.readFile(path, vers, "info")
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
func (g *fsProxyModuleGetter) Mod(ctx context.Context, path, vers string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "fsProxyModuleGetter.Mod(%q, %q)", path, vers)

	if vers == version.Latest {
		vers, err = g.latestVersion(path)
		if err != nil {
			return nil, err
		}
	}

	// Check that .zip is readable first.
	f, err := g.openFile(path, vers, "zip")
	if err != nil {
		return nil, err
	}
	f.Close()
	return g.readFile(path, vers, "mod")
}

// ContentDir returns an fs.FS for the module's contents.
func (g *fsProxyModuleGetter) ContentDir(ctx context.Context, path, vers string) (_ fs.FS, err error) {
	defer derrors.Wrap(&err, "fsProxyModuleGetter.ContentDir(%q, %q)", path, vers)

	if vers == version.Latest {
		vers, err = g.latestVersion(path)
		if err != nil {
			return nil, err
		}
	}
	data, err := g.readFile(path, vers, "zip")
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	return fs.Sub(zr, path+"@"+vers)
}

// SourceInfo returns a source.Info that will create /files links to modules in
// the cache.
func (g *fsProxyModuleGetter) SourceInfo(ctx context.Context, mpath, version string) (*source.Info, error) {
	return source.FilesInfo(path.Join(g.dir, mpath+"@"+version)), nil
}

// SourceFS returns the absolute path to the cache, and an FS that retrieves
// files from it.
func (g *fsProxyModuleGetter) SourceFS() (string, fs.FS) {
	return filepath.ToSlash(g.dir), os.DirFS(g.dir)
}

// latestVersion gets the latest version that is in the directory.
func (g *fsProxyModuleGetter) latestVersion(modulePath string) (_ string, err error) {
	defer derrors.Wrap(&err, "fsProxyModuleGetter.latestVersion(%q)", modulePath)

	dir, err := g.moduleDir(modulePath)
	if err != nil {
		return "", err
	}
	zips, err := filepath.Glob(filepath.Join(dir, "*.zip"))
	if err != nil {
		return "", err
	}
	if len(zips) == 0 {
		return "", fmt.Errorf("no zips in %q for module %q: %w", g.dir, modulePath, derrors.NotFound)
	}
	var versions []string
	for _, z := range zips {
		versions = append(versions, strings.TrimSuffix(filepath.Base(z), ".zip"))
	}
	return version.LatestOf(versions), nil
}

func (g *fsProxyModuleGetter) readFile(path, version, suffix string) (_ []byte, err error) {
	defer derrors.Wrap(&err, "fsProxyModuleGetter.readFile(%q, %q, %q)", path, version, suffix)

	f, err := g.openFile(path, version, suffix)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

func (g *fsProxyModuleGetter) openFile(path, version, suffix string) (_ *os.File, err error) {
	epath, err := g.escapedPath(path, version, suffix)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(epath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("%w: %v", derrors.NotFound, err)
		}
		return nil, err
	}
	return f, nil
}

func (g *fsProxyModuleGetter) escapedPath(modulePath, version, suffix string) (string, error) {
	dir, err := g.moduleDir(modulePath)
	if err != nil {
		return "", err
	}
	ev, err := module.EscapeVersion(version)
	if err != nil {
		return "", fmt.Errorf("version: %v: %w", err, derrors.InvalidArgument)
	}
	return filepath.Join(dir, fmt.Sprintf("%s.%s", ev, suffix)), nil
}

func (g *fsProxyModuleGetter) moduleDir(modulePath string) (string, error) {
	ep, err := module.EscapePath(modulePath)
	if err != nil {
		return "", fmt.Errorf("path: %v: %w", err, derrors.InvalidArgument)
	}
	return filepath.Join(g.dir, "cache", "download", filepath.FromSlash(ep), "@v"), nil
}
