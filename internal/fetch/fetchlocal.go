// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/source"
)

// Version and commit time are pre specified when fetching a local module, as these
// fields are normally obtained from a proxy.
var (
	LocalVersion    = "v0.0.0"
	LocalCommitTime = time.Time{}
)

// FetchLocalModule fetches a module from a local directory and process its contents
// to return an internal.Module and other related information. modulePath is not necessary
// if the module has a go.mod file, but if both exist, then they must match.
// FetchResult.Error should be checked to verify that the fetch succeeded. Even if the
// error is non-nil the result may contain useful data.
func FetchLocalModule(ctx context.Context, modulePath, localPath string, sourceClient *source.Client) *FetchResult {
	fr := &FetchResult{
		ModulePath:       modulePath,
		RequestedVersion: LocalVersion,
		ResolvedVersion:  LocalVersion,
		Defer:            func() {},
	}

	var fi *FetchInfo
	defer func() {
		if fr.Error != nil {
			derrors.Wrap(&fr.Error, "FetchLocalModule(%q, %q)", modulePath, localPath)
			fr.Status = derrors.ToStatus(fr.Error)
		}
		if fr.Status == 0 {
			fr.Status = http.StatusOK
		}
		if fi != nil {
			finishFetchInfo(fi, fr.Status, fr.Error)
		}
	}()

	info, err := os.Stat(localPath)
	if err != nil {
		fr.Error = fmt.Errorf("%s: %w", err.Error(), derrors.NotFound)
		return fr
	}

	if !info.IsDir() {
		fr.Error = fmt.Errorf("%s not a directory: %w", localPath, derrors.NotFound)
		return fr
	}

	fi = &FetchInfo{
		ModulePath: fr.ModulePath,
		Version:    fr.ResolvedVersion,
		Start:      time.Now(),
	}
	startFetchInfo(fi)

	// Options for module path are either the modulePath parameter or go.mod file.
	// Accepted cases:
	//   - Both are given and are the same.
	//   - Only one is given. Note that: if modulePath is given and there's no go.mod
	//     file, then the package is assumed to be using GOPATH.
	// Errors:
	//   - Both are given and are different.
	//   - Neither is given.
	if goModBytes, err := ioutil.ReadFile(filepath.Join(localPath, "go.mod")); err != nil {
		fr.GoModPath = modulePath
		fr.HasGoMod = false
	} else {
		fr.HasGoMod = true
		fr.GoModPath = modfile.ModulePath(goModBytes)
		if fr.GoModPath != modulePath && modulePath != "" {
			fr.Error = fmt.Errorf("module path=%s, go.mod path=%s: %w", modulePath, fr.GoModPath, derrors.AlternativeModule)
			return fr
		}
	}

	if fr.GoModPath == "" {
		fr.Error = fmt.Errorf("no module path: %w", derrors.BadModule)
		return fr
	}
	fr.ModulePath = fr.GoModPath

	zipReader, err := createZipReader(localPath, fr.GoModPath, LocalVersion)
	if err != nil {
		fr.Error = fmt.Errorf("couldn't create a zip: %s, %w", err.Error(), derrors.BadModule)
		return fr
	}

	mod, pvs, err := processZipFile(ctx, fr.GoModPath, LocalVersion, LocalVersion, LocalCommitTime, zipReader, sourceClient)
	if err != nil {
		fr.Error = err
		return fr
	}
	mod.HasGoMod = fr.HasGoMod
	fr.Module = mod
	fr.PackageVersionStates = pvs
	fr.Module.SourceInfo = nil // version is not known, so even if info is found it most likely is wrong.
	for _, state := range fr.PackageVersionStates {
		if state.Status != http.StatusOK {
			fr.Status = derrors.ToStatus(derrors.HasIncompletePackages)
		}
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
