// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fetch provides a way to fetch modules from a proxy.
package fetch

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// extractReadmes returns the file path and contents of all files from r
// that are README files.
func extractReadmes(modulePath, resolvedVersion string, contentDir fs.FS) (_ []*internal.Readme, err error) {
	defer derrors.Wrap(&err, "extractReadmes(ctx, %q, %q, r)", modulePath, resolvedVersion)

	// The key is the README directory. Since we only store one README file per
	// directory, we use this below to prioritize READMEs in markdown.
	readmes := map[string]*internal.Readme{}
	err = fs.WalkDir(contentDir, ".", func(pathname string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isReadme(pathname) {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() > MaxFileSize {
				return fmt.Errorf("file size %d exceeds max limit %d", info.Size(), MaxFileSize)
			}
			c, err := readFSFile(contentDir, pathname, MaxFileSize)
			if err != nil {
				return err
			}

			key := path.Dir(pathname)
			if r, ok := readmes[key]; ok {
				// Prefer READMEs written in markdown, since we style these on
				// the frontend.
				ext := path.Ext(r.Filepath)
				if ext == ".md" || ext == ".markdown" {
					return nil
				}
			}
			readmes[key] = &internal.Readme{
				Filepath: pathname,
				Contents: string(c),
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) { // we can get NotExist on an empty FS {
		return nil, err
	}
	var rs []*internal.Readme
	for _, r := range readmes {
		rs = append(rs, r)
	}
	return rs, nil
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
