// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetch provides a way to fetch modules from a proxy.
package fetch

import (
	"archive/zip"
	"fmt"
	"path"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// extractReadmesFromZip returns the file path and contents of all files from r
// that are README files.
func extractReadmesFromZip(modulePath, resolvedVersion string, r *zip.Reader) (_ []*internal.Readme, err error) {
	defer derrors.Wrap(&err, "extractReadmesFromZip(ctx, %q, %q, r)", modulePath, resolvedVersion)
	var readmes []*internal.Readme
	for _, zipFile := range r.File {
		if isReadme(zipFile.Name) {
			if zipFile.UncompressedSize64 > MaxFileSize {
				return nil, fmt.Errorf("file size %d exceeds max limit %d", zipFile.UncompressedSize64, MaxFileSize)
			}
			c, err := readZipFile(zipFile, MaxFileSize)
			if err != nil {
				return nil, err
			}
			readmes = append(readmes, &internal.Readme{
				Filepath: strings.TrimPrefix(zipFile.Name, moduleVersionDir(modulePath, resolvedVersion)+"/"),
				Contents: string(c),
			})

		}
	}
	return readmes, nil
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
