// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"errors"
	"io/ioutil"
	"path/filepath"
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
