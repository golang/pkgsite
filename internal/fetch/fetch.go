// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sos.googlesource.com/sos/license"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/tools/go/packages"
)

var (
	errModuleContainsNoPackages = errors.New("module contains 0 packages")
)

// ParseModulePathAndVersion returns the module and version specified by u. u is
// assumed to be a valid url following the structure http(s)://<fetchURL>/<module>@<version>.
func ParseModulePathAndVersion(u *url.URL) (string, string, error) {
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

	seriesName, err := seriesPathForModule(name)
	if err != nil {
		return fmt.Errorf("seriesPathForModule(%q): %v", name, err)
	}

	packages, err := extractPackagesFromZip(name, version, zipReader)
	if err != nil && err != errModuleContainsNoPackages {
		return fmt.Errorf("extractPackagesFromZip(%q, %q, %v): %v", name, version, zipReader, err)
	}

	v := internal.Version{
		Module: &internal.Module{
			Path: name,
			Series: &internal.Series{
				Path: seriesName,
			},
		},
		Version:    version,
		CommitTime: info.Time,
		ReadMe:     string(readme),
		License:    detectLicense(zipReader, fmt.Sprintf("%s@%s", name, version)),
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
		files := make(map[string]*ast.File)
		for i, f := range p.CompiledGoFiles {
			files[f] = p.Syntax[i]
		}

		apkg := &ast.Package{
			Name:  p.Name,
			Files: files,
		}
		d := doc.New(apkg, p.PkgPath, 0)

		packages = append(packages, &internal.Package{
			Name:     p.Name,
			Path:     p.PkgPath,
			Synopsis: doc.Synopsis(d.Doc),
		})
	}
	return packages, nil
}

// seriesPathForModule reports the series name for the given module. The series
// name is the shared base path of a group of major-version variants. For
// example, my/module, my/module/v2, my/module/v3 are a single series, with the
// series name my/module.
func seriesPathForModule(name string) (string, error) {
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
		version, err := strconv.Atoi(suffix[1:])
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

// hasFilename checks if file is expectedFile or if the name of file, without
// the base, is equal to expectedFile. It is case insensitive.
func hasFilename(file string, expectedFile string) bool {
	base := filepath.Base(file)
	return strings.EqualFold(file, expectedFile) ||
		strings.EqualFold(base, expectedFile) ||
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

const (
	// licenseClassifyThreshold is the minimum confidence percentage/threshold
	// to classify a license
	licenseClassifyThreshold = 96 // TODO: run more tests to figure out the best percent.

	// licenseCoverageThreshold is the minimum percentage of the file that must contain license text.
	licenseCoverageThreshold = 90
)

var (
	// licenseFileNames is the list of file names that could contain a license.
	licenseFileNames = map[string]bool{
		"LICENSE":    true,
		"LICENSE.md": true,
		"COPYING":    true,
		"COPYING.md": true,
	}
)

// detectLicense searches for possible license files in the contents directory
// of the provided zip path, runs them against a license classifier, and provides all
// licenses with a confidence score that meet the licenseClassifyThreshold.
func detectLicense(r *zip.Reader, contentsDir string) string {
	for _, f := range r.File {
		if filepath.Dir(f.Name) != contentsDir || !licenseFileNames[filepath.Base(f.Name)] {
			
			
			continue
		}
		bytes, err := readZipFile(f)
		if err != nil {
			log.Printf("readZipFile(%s): %v", f.Name, err)
			continue
		}

		cov, ok := license.Cover(bytes, license.Options{})
		if !ok || cov.Percent < licenseCoverageThreshold {
			continue
		}

		m := cov.Match[0]
		if m.Percent > licenseClassifyThreshold {
			return m.Name
		}
	}
	return ""
}
