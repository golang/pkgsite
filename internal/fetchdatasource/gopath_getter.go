// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetchdatasource

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/fetch"
)

// NewGOPATHModuleGetter returns a module getter that uses the GOPATH
// environment variable to find the module with the given import path.
func NewGOPATHModuleGetter(importPath string) (_ fetch.ModuleGetter, err error) {
	defer derrors.Wrap(&err, "NewGOPATHModuleGetter(%q)", importPath)

	dir := getFullPath(importPath)
	if dir == "" {
		return nil, fmt.Errorf("path %s doesn't exist: %w", importPath, derrors.NotFound)
	}
	return fetch.NewDirectoryModuleGetter(importPath, dir)
}

// getFullPath takes an import path, tests it relative to each GOPATH, and returns
// a full path to the module. If the given import path doesn't exist in any GOPATH,
// an empty string is returned.
func getFullPath(modulePath string) string {
	gopaths := filepath.SplitList(os.Getenv("GOPATH"))
	for _, gopath := range gopaths {
		path := filepath.Join(gopath, "src", modulePath)
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}
