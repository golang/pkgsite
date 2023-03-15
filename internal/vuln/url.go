// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code adapted from
// https://github.com/golang/go/blob/2ebe77a2fda1ee9ff6fd9a3e08933ad1ebaea039/src/cmd/go/internal/web/url.go
// TODO(go.dev/issue/32456): if accepted, use the new API.

package vuln

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
)

var errNotAbsolute = errors.New("path is not absolute")

// URLToFilePath converts a file-scheme url to a file path.
func URLToFilePath(u *url.URL) (string, error) {
	if u.Scheme != "file" {
		return "", errors.New("non-file URL")
	}

	checkAbs := func(path string) (string, error) {
		if !filepath.IsAbs(path) {
			return "", errNotAbsolute
		}
		return path, nil
	}

	if u.Path == "" {
		if u.Host != "" || u.Opaque == "" {
			return "", errors.New("file URL missing path")
		}
		return checkAbs(filepath.FromSlash(u.Opaque))
	}

	path, err := convertFileURLPath(u.Host, u.Path)
	if err != nil {
		return path, err
	}
	return checkAbs(path)
}

func convertFileURLPath(host, path string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("windows not supported")
	}
	switch host {
	case "", "localhost":
	default:
		return "", errors.New("file URL specifies non-local host")
	}
	return filepath.FromSlash(path), nil
}
