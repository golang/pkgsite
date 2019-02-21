// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/tools/go/packages"
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
			return readZipFile(zipFile)
		}
	}
	return nil, errReadmeNotFound
}

// readZipFile returns the uncompressed contents of f or an error if the
// uncompressed size of f exceeds 1MB.
func readZipFile(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > 1e6 {
		return nil, fmt.Errorf("file size %d exceeds 1MB, skipping", f.UncompressedSize64)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return ioutil.ReadAll(rc)
}

// extractPackagesFromZip returns a slice of packages from the module zip r.
func extractPackagesFromZip(module string, r *zip.ReadCloser) ([]*internal.Package, error) {
	// Create a temporary directory to write the contents of the module zip.
	tempPrefix := "discovery_"
	dir, err := ioutil.TempDir("", tempPrefix)
	if err != nil {
		return nil, fmt.Errorf("ioutil.TempDir(%q, %q): %v", "", tempPrefix, err)
	}
	defer os.RemoveAll(dir)

	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, module) && !strings.HasPrefix(module, f.Name) {
			return nil, fmt.Errorf("expected files to have shared prefix %q, found %q", module, f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(
				fmt.Sprintf("%s/%s", dir, f.Name), os.ModePerm); err != nil {
				return nil, fmt.Errorf("ioutil.TempDir(%q, %q): %v", dir, f.Name, err)
			}
		} else {
			file, err := os.Create(fmt.Sprintf("%s/%s", dir, f.Name))
			if err != nil {
				return nil, fmt.Errorf("ioutil.TempFile(%q, %q): %v", dir, f.Name, err)
			}
			defer file.Close()

			b, err := readZipFile(f)
			if err != nil {
				return nil, fmt.Errorf("readZipFile(%q): %v", f.Name, err)
			}

			if _, err := file.Write(b); err != nil {
				return nil, fmt.Errorf("file.Write: %v", err)
			}
		}
	}

	config := &packages.Config{
		Mode: packages.LoadSyntax,
		Dir:  fmt.Sprintf("%s/%s", dir, module),
	}
	pattern := fmt.Sprintf("%s/...", module)
	pkgs, err := packages.Load(config, pattern)
	if err != nil {
		return nil, fmt.Errorf("packages.Load(%+v, %q): %v", config, pattern, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("packages.Load(%+v, %q) returned 0 packages", config, pattern)
	}

	packages := []*internal.Package{}
	for _, p := range pkgs {
		packages = append(packages, &internal.Package{
			Name: p.Name,
			Path: p.PkgPath,
		})
	}
	return packages, nil
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
