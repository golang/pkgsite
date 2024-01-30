// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetch provides a way to fetch modules from a proxy.
package fetch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// extractReadmes returns the file path and contents of all files from r
// that are README files.
func extractReadmes(modulePath, resolvedVersion string, contentDir fs.FS) (_ []*internal.Readme, err error) {
	defer derrors.Wrap(&err, "extractReadmes(ctx, %q, %q, r)", modulePath, resolvedVersion)

	var readmes []*internal.Readme
	err = fs.WalkDir(contentDir, ".", func(pathname string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		readme, err := extractReadme(modulePath, path.Join(modulePath, pathname), resolvedVersion, contentDir)
		if err != nil {
			return err
		}
		if readme == nil {
			// no readme for the directory
			return nil
		}
		readmes = append(readmes, readme)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) { // we can get NotExist on an empty FS {
		return nil, err
	}
	return readmes, nil
}

// rel returns the relative path from the modulePath to the pkgPath
// returning "." if they're the same.
func rel(pkgPath, modulePath string) string {
	suff := internal.Suffix(pkgPath, modulePath)
	if suff == "" {
		return "."
	}
	return suff
}

// extractReadme returns the file path and contents the unit's README,
// if there is one. dir is the directory path prefixed with the modulePath.
func extractReadme(modulePath, dir, resolvedVersion string, contentDir fs.FS) (_ *internal.Readme, err error) {
	defer derrors.Wrap(&err, "extractReadme(ctx, %q, %q %q, r)", modulePath, dir, resolvedVersion)

	innerPath := rel(dir, modulePath)
	if strings.HasPrefix(innerPath, "_") {
		// TODO(matloob): do we want to check each element of the path?
		// The original code didn't.
		return nil, nil
	}

	f, err := contentDir.Open(innerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	rdf, ok := f.(fs.ReadDirFile)
	if !ok {
		return nil, fmt.Errorf("could not open directory for %v", dir)
	}
	entries, err := rdf.ReadDir(0)
	if err != nil {
		return nil, err
	}
	var readme *internal.Readme
	for _, e := range entries {
		pathname := path.Join(innerPath, e.Name())
		if !e.IsDir() && isReadme(pathname) {
			info, err := e.Info()
			if err != nil {
				return nil, err
			}
			if info.Size() > MaxFileSize {
				return nil, fmt.Errorf("file size %d exceeds max limit %d: %w", info.Size(), MaxFileSize, derrors.ModuleTooLarge)
			}
			c, err := readFSFile(contentDir, pathname, MaxFileSize)
			if err != nil {
				return nil, err
			}

			if readme != nil {
				// Prefer READMEs written in markdown, since we style these on
				// the frontend.
				ext := path.Ext(readme.Filepath)
				if ext == ".md" || ext == ".markdown" {
					continue
				}
			}
			readme = &internal.Readme{
				Filepath: pathname,
				Contents: string(c),
			}
		}
	}
	return readme, nil
}

var excludedReadmeExts = map[string]bool{".go": true, ".vendor": true}

// isReadme reports whether file is README or if the base name of file, with or
// without the extension, is equal to expectedFile. README.go files will return
// false. It is case insensitive. It operates on '/'-separated paths.
func isReadme(file string) bool {
	const expectedFile = "README"
	base := path.Base(file)
	ext := path.Ext(base)
	return !excludedReadmeExts[ext] && strings.EqualFold(strings.TrimSuffix(base, ext), expectedFile)
}
