// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
)

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

// Zip returns a reader for the module's zip file.
func (g *directoryModuleGetter) Zip(ctx context.Context, path, version string) (*zip.Reader, error) {
	if err := g.checkPath(path); err != nil {
		return nil, err
	}
	return createZipReader(g.dir, path, LocalVersion)
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
