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
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
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

	// ZipSize returns the approximate size of the zip file in bytes.
	// It is used only for load-shedding.
	ZipSize(ctx context.Context, path, version string) (int64, error)
}

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

// ContentDir returns an FS for the module's contents. The FS should match the format
// of a module zip file.
func (g *proxyModuleGetter) ContentDir(ctx context.Context, path, version string) (fs.FS, error) {
	zr, err := g.prox.Zip(ctx, path, version)
	if err != nil {
		return nil, err
	}
	return fs.Sub(zr, path+"@"+version)
}

// ZipSize returns the approximate size of the zip file in bytes.
// It is used only for load-shedding.
func (g *proxyModuleGetter) ZipSize(ctx context.Context, path, version string) (int64, error) {
	return g.prox.ZipSize(ctx, path, version)
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
	dir        string
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
	return &directoryModuleGetter{
		dir:        dir,
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
	zr, err := createZipReader(g.dir, path, LocalVersion)
	if err != nil {
		return nil, err
	}
	return fs.Sub(zr, path+"@"+LocalVersion)
}

// ZipSize returns the approximate size of the zip file in bytes.
func (g *directoryModuleGetter) ZipSize(ctx context.Context, path, version string) (int64, error) {
	return 0, errors.New("directoryModuleGetter.ZipSize unimplemented")
}

// createZipReader creates a zip file from a directory given a local path and
// returns a zip.Reader to be passed to processZipFile. The purpose of the
// function is to transform a local go module into a zip file to be processed by
// existing functions.
func createZipReader(localPath, modulePath, version string) (*zip.Reader, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	err := filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		readFrom, err := os.Open(path)
		if err != nil {
			return err
		}
		defer readFrom.Close()

		writeTo, err := w.Create(filepath.Join(moduleVersionDir(modulePath, version), strings.TrimPrefix(path, localPath)))
		if err != nil {
			return err
		}

		_, err = io.Copy(writeTo, readFrom)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	reader := bytes.NewReader(buf.Bytes())
	return zip.NewReader(reader, reader.Size())
}

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

// ContentDir returns an fs.FS for the module's contents.
func (g *fsModuleGetter) ContentDir(ctx context.Context, path, version string) (_ fs.FS, err error) {
	defer derrors.Wrap(&err, "fsModuleGetter.ContentDir(%q, %q)", path, version)

	data, err := g.readFile(path, version, "zip")
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	return fs.Sub(zr, path+"@"+version)
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
