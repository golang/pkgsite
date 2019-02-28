// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/tools/go/packages"
)

var (
	errModuleContainsNoPackages = errors.New("module contains 0 packages")
)

// ParseNameAndVersion returns the module and version specified by u. u is
// assumed to be a valid url following the structure http(s)://<fetchURL>/<module>@<version>.
func ParseNameAndVersion(u *url.URL) (string, string, error) {
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/@v/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid path: %q", u)
	}

	// TODO(julieqiu): Check module name is valid using
	// https://github.com/golang/go/blob/c97e576/src/cmd/go/internal/module/module.go#L123
	// Check version is valid using
	// https://github.com/golang/go/blob/c97e576/src/cmd/go/internal/modload/query.go#L183
	if parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid path: %q", u)
	}

	return parts[0], parts[1], nil
}

// FetchAndInsertVersion downloads the given module version from the module proxy, processes
// the contents, and writes the data to the database. The fetch service will:
// (1) Get the version commit time from the proxy
// (2) Download the version zip from the proxy
// (3) Process the contents (series name, readme, license, and packages)
// (4) Write the data to the discovery database
func FetchAndInsertVersion(name, version string, proxyClient *proxy.Client, db *postgres.DB) error {
	info, err := proxyClient.GetInfo(name, version)
	if err != nil {
		return fmt.Errorf("proxyClient.GetInfo(%q, %q): %v", name, version, err)
	}

	zipReader, err := proxyClient.GetZip(name, version)
	if err != nil {
		return fmt.Errorf("proxyClient.GetZip(%q, %q): %v", name, version, err)
	}

	var readme []byte
	readmeFile := "README"
	if containsFile(zipReader, readmeFile) {
		readme, err = extractFile(zipReader, readmeFile)
		if err != nil {
			return fmt.Errorf("extractFile(%v, %q): %v", zipReader, readmeFile, err)
		}
	}

	var license []byte
	licenseFile := "LICENSE"
	if containsFile(zipReader, licenseFile) {
		license, err = extractFile(zipReader, licenseFile)
		if err != nil {
			return fmt.Errorf("extractFile(%v, %q): %v", zipReader, licenseFile, err)
		}
	}

	seriesName, err := seriesNameForModule(name)
	if err != nil {
		return fmt.Errorf("seriesNameForModule(%q): %v", name, err)
	}

	packages, err := extractPackagesFromZip(name, version, zipReader)
	if err != nil && err != errModuleContainsNoPackages {
		return fmt.Errorf("extractPackagesFromZip(%q, %q, %v): %v", name, version, zipReader, err)
	}

	v := internal.Version{
		Module: &internal.Module{
			Name: name,
			Series: &internal.Series{
				Name: seriesName,
			},
		},
		Version:    version,
		CommitTime: info.Time,
		ReadMe:     string(readme),
		License:    string(license),
		Packages:   packages,
	}
	if err = db.InsertVersion(&v); err != nil {
		return fmt.Errorf("db.InsertVersion(%+v): %v", v, err)
	}
	return nil
}

// extractPackagesFromZip returns a slice of packages from the module zip r.
func extractPackagesFromZip(module, version string, r *zip.Reader) ([]*internal.Package, error) {
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

		if !f.FileInfo().IsDir() {
			fileDir := fmt.Sprintf("%s/%s", dir, filepath.Dir(f.Name))
			if _, err := os.Stat(fileDir); os.IsNotExist(err) {
				if err := os.MkdirAll(fileDir, os.ModePerm); err != nil {
					return nil, fmt.Errorf("ioutil.TempDir(%q, %q): %v", dir, f.Name, err)
				}
			}

			filename := fmt.Sprintf("%s/%s", dir, f.Name)
			file, err := os.Create(filename)
			if err != nil {
				return nil, fmt.Errorf("os.Create(%q): %v", filename, err)
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
		Dir:  fmt.Sprintf("%s/%s@%s", dir, module, version),
	}
	pattern := fmt.Sprintf("%s/...", module)
	pkgs, err := packages.Load(config, pattern)
	if err != nil {
		return nil, fmt.Errorf("packages.Load(%+v, %q): %v", config, pattern, err)
	}

	if len(pkgs) == 0 {
		return nil, errModuleContainsNoPackages
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

// extractFile reads the contents of the first file from r that passes the
// hasFilename check for expectedFile. It returns an error if such a file
// does not exist.
func extractFile(r *zip.Reader, expectedFile string) ([]byte, error) {
	for _, zipFile := range r.File {
		if hasFilename(zipFile.Name, expectedFile) {
			c, err := readZipFile(zipFile)
			return c, err
		}
	}
	return nil, fmt.Errorf("zip does not contain %q", expectedFile)
}

// containsFile checks if r contains expectedFile.
func containsFile(r *zip.Reader, expectedFile string) bool {
	for _, zipFile := range r.File {
		if hasFilename(zipFile.Name, expectedFile) {
			return true
		}
	}
	return false
}

// hasFilename checks if:
// (1) file is expectedFile, or
// (2) the name of file, without the base, is equal to expectedFile.
// It is case insensitive.
func hasFilename(file string, expectedFile string) bool {
	base := filepath.Base(file)
	return strings.EqualFold(base, expectedFile) ||
		strings.EqualFold(strings.TrimSuffix(base, filepath.Ext(base)), expectedFile)
}

// readZipFile returns the uncompressed contents of f or an error if the
// uncompressed size of f exceeds 1MB.
func readZipFile(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > 1e6 {
		return nil, fmt.Errorf("file size %d exceeds 1MB, skipping", f.UncompressedSize64)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("f.Open() for %q: %v", f.Name, err)
	}
	defer rc.Close()

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll(rc) for %q: %v", f.Name, err)
	}
	return b, nil
}
