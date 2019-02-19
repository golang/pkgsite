// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
)

var errReadmeNotFound = errors.New("fetch: zip file does not contain a README")

// isReadme checks if file is the README. It is case insensitive.
func isReadme(file string) bool {
	base := filepath.Base(file)
	f := strings.TrimSuffix(base, filepath.Ext(base))
	return strings.ToUpper(f) == "README"
}

// containsReadme checks if rc contains a README file.
func containsReadme(rc *zip.ReadCloser) bool {
	for _, zipFile := range rc.File {
		if isReadme(zipFile.Name) {
			return true
		}
	}
	return false
}

// readReadme reads the contents of the first file from rc that passes the
// isReadme check. It returns an error if such a file does not exist.
func readReadme(rc *zip.ReadCloser) ([]byte, error) {
	for _, zipFile := range rc.File {
		if isReadme(zipFile.Name) {
			f, err := zipFile.Open()
			if err != nil {
				return nil, err
			}
			defer f.Close()

			return ioutil.ReadAll(f)
		}
	}
	return nil, errReadmeNotFound
}

// seriesNameForModule reports the series name for the given module. The series
// name is the shared base path of a group of major-version variants. For
// example, my/module, my/module/v2, my/module/v3 are a single series, with the
// series name my/module.
func seriesNameForModule(name string) (string, error) {
	if name == "" {
		return "", errors.New("module name cannot be empty")
	}

	name = strings.TrimSuffix(name, "/")
	parts := strings.Split(name, "/")
	if len(parts) <= 1 {
		return name, nil
	}

	suffix := parts[len(parts)-1]
	if string(suffix[0]) == "v" {
		// Attempt to convert the portion of suffix following "v" to an
		// integer. If that portion cannot be converted to an integer, or the
		// version = 0 or 1, return the full module name. For example:
		// my/module/v2 has series name my/module
		// my/module/v1 has series name my/module/v1
		// my/module/v2x has series name my/module/v2x
		version, err := strconv.Atoi(suffix[1:len(suffix)])
		if err != nil || version < 2 {
			return name, nil
		}
		return strings.Join(parts[0:len(parts)-1], "/"), nil
	}
	return name, nil
}
