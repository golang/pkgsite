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

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
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
	dir string // the directory containing the module's files
}

// NewDirectoryModuleGetter returns a ModuleGetter for reading a module from a directory.
func NewDirectoryModuleGetter(dir string) ModuleGetter {
	return &directoryModuleGetter{dir: dir}
}

// Info returns basic information about the module.
func (g *directoryModuleGetter) Info(ctx context.Context, path, version string) (*proxy.VersionInfo, error) {
	return &proxy.VersionInfo{
		Version: LocalVersion,
		Time:    LocalCommitTime,
	}, nil
}

// Mod returns the contents of the module's go.mod file.
// If the file does not exist, it returns a synthesized one.
func (g *directoryModuleGetter) Mod(ctx context.Context, path, version string) ([]byte, error) {
	data, err := ioutil.ReadFile(filepath.Join(g.dir, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		if path == "" {
			return nil, fmt.Errorf("no module path: %w", derrors.BadModule)
		}
		return []byte(fmt.Sprintf("module %s\n", path)), nil
	}
	return data, err
}

// Zip returns a reader for the module's zip file.
func (g *directoryModuleGetter) Zip(ctx context.Context, path, version string) (*zip.Reader, error) {
	return createZipReader(g.dir, path, LocalVersion)
}

// ZipSize returns the approximate size of the zip file in bytes.
func (g *directoryModuleGetter) ZipSize(ctx context.Context, path, version string) (int64, error) {
	return 0, errors.New("directoryModuleGetter.ZipSize unimplemented")
}

// FetchLocalModule fetches a module from a local directory and process its contents
// to return an internal.Module and other related information. modulePath is not necessary
// if the module has a go.mod file, but if both exist, then they must match.
// FetchResult.Error should be checked to verify that the fetch succeeded. Even if the
// error is non-nil the result may contain useful data.
func FetchLocalModule(ctx context.Context, modulePath, localPath string, sourceClient *source.Client) *FetchResult {
	g := NewDirectoryModuleGetter(localPath)
	fr := FetchModule(ctx, modulePath, LocalVersion, g, sourceClient)
	if fr.Error != nil {
		fr.Error = fmt.Errorf("FetchLocalModule(%q, %q): %w", modulePath, localPath, fr.Error)
	}
	return fr
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
