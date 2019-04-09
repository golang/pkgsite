// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sos.googlesource.com/sos/license"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/tools/go/packages"
)

// fetchTimeout bounds the time allowed for fetching a single module.  It is
// mutable for testing purposes.
var fetchTimeout = 5 * time.Minute

var (
	errModuleContainsNoPackages = errors.New("module contains 0 packages")

	// maxFileSize is the maximum filesize that is allowed for reading.
	// If a .go file is encountered that exceeds maxFileSize, the fetch request
	// will fail.  All other filetypes will be ignored.
	maxFileSize = uint64(1e7)
)

// ParseModulePathAndVersion returns the module and version specified by p. p is
// assumed to have the structure /<module>/@v/<version>.
func ParseModulePathAndVersion(p string) (string, string, error) {
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/@v/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid path: %q", p)
	}

	// TODO(julieqiu): Check module name is valid using
	// https://github.com/golang/go/blob/c97e576/src/cmd/go/internal/module/module.go#L123
	// Check version is valid using
	// https://github.com/golang/go/blob/c97e576/src/cmd/go/internal/modload/query.go#L183
	if parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid path: %q", p)
	}

	return parts[0], parts[1], nil
}

// parseVersion returns the VersionType of a given a version.
func parseVersion(version string) (internal.VersionType, error) {
	if !semver.IsValid(version) {
		return "", fmt.Errorf("semver.IsValid(%q): false", version)
	}

	prerelease := semver.Prerelease(version)
	if prerelease == "" {

		return internal.VersionTypeRelease, nil
	}
	prerelease = prerelease[1:] // remove starting dash

	// if prerelease looks like a commit then return VersionTypePseudo
	matched, err := regexp.MatchString(`^[0-9]{14}-[0-9a-z]{12}$`, prerelease)
	if err != nil {
		return "", fmt.Errorf("regexp.MatchString(`^[0-9]{14}-[0-9a-z]{12}$`, %v): %v", prerelease, err)
	}

	if matched {
		rawTime := strings.Split(prerelease, "-")[0]
		layout := "20060102150405"
		t, err := time.Parse(layout, rawTime)

		if err == nil && t.Before(time.Now()) {
			return internal.VersionTypePseudo, nil
		}
	}

	return internal.VersionTypePrerelease, nil
}

// FetchAndInsertVersion downloads the given module version from the module proxy, processes
// the contents, and writes the data to the database. The fetch service will:
// (1) Get the version commit time from the proxy
// (2) Download the version zip from the proxy
// (3) Process the contents (series name, readme, license, and packages)
// (4) Write the data to the discovery database
func FetchAndInsertVersion(name, version string, proxyClient *proxy.Client, db *postgres.DB) error {
	// Unlike other actions (which use a Timeout middleware), we set a fixed
	// timeout for FetchAndInsertVersion.  This allows module processing to
	// succeed even for extremely short lived requests.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	if err := module.CheckPath(name); err != nil {
		return fmt.Errorf("fetch: invalid module name %v: %v", name, err)
	}

	versionType, err := parseVersion(version)
	if err != nil {
		return fmt.Errorf("parseVersion(%q): %v", version, err)
	}

	info, err := proxyClient.GetInfo(ctx, name, version)
	if err != nil {
		return fmt.Errorf("proxyClient.GetInfo(%q, %q): %v", name, version, err)
	}

	zipReader, err := proxyClient.GetZip(ctx, name, version)
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
		return fmt.Errorf("extractPackagesFromZip(%q, %q): %v", name, version, err)
	}

	contentDir := fmt.Sprintf("%s@%s", name, version)
	license, err := detectLicense(zipReader, contentDir)
	if err != nil {
		return fmt.Errorf("detectLicense(zipReader, %q): %v", contentDir, err)
	}

	v := &internal.Version{
		Module: &internal.Module{
			Path: name,
			Series: &internal.Series{
				Path: seriesName,
			},
		},
		Version:     version,
		CommitTime:  info.Time,
		ReadMe:      readme,
		License:     license,
		Packages:    packages,
		VersionType: versionType,
	}
	if err = db.InsertVersion(ctx, v); err != nil {
		return fmt.Errorf("db.InsertVersion for %q %q: %v", name, version, err)
	}
	if err = db.InsertDocuments(ctx, v); err != nil {
		return fmt.Errorf("db.InsertDocuments for %q %q: %v", name, version, err)
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
			// Skip files that are not .go files and are greater than 10MB.
			if filepath.Ext(f.Name) != ".go" && f.UncompressedSize64 > maxFileSize {
				continue
			}

			if err := writeFileToDir(f, dir); err != nil {
				return nil, fmt.Errorf("writeFileToDir(%q, %q): %v", f.Name, dir, err)
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
		return nil, fmt.Errorf("packages.Load(config, %q): %v", pattern, err)
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
			Suffix:   strings.TrimPrefix(strings.TrimPrefix(p.PkgPath, module), "/"),
		})
	}
	return packages, nil
}

// writeFileToDir writes the contents of f to the directory dir.
func writeFileToDir(f *zip.File, dir string) (err error) {
	fileDir := filepath.Join(dir, filepath.Dir(f.Name))
	if _, err := os.Stat(fileDir); os.IsNotExist(err) {
		if err := os.MkdirAll(fileDir, os.ModePerm); err != nil {
			return fmt.Errorf("os.MkdirAll(%q, os.ModePerm): %v", fileDir, err)
		}
	}

	filename := filepath.Join(dir, f.Name)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("os.Create(%q): %v", filename, err)
	}
	defer func() {
		cerr := file.Close()
		if err == nil && cerr != nil {
			err = fmt.Errorf("file.Close: %v", cerr)
		}
	}()

	b, err := readZipFile(f)
	if err != nil {
		return fmt.Errorf("readZipFile(%q): %v", f.Name, err)
	}

	if _, err := file.Write(b); err != nil {
		return fmt.Errorf("file.Write: %v", err)
	}
	return nil
}

// seriesPathForModule reports the series name for the given module. The series
// name is the shared base path of a group of major-version variants. For
// example, my.mod/module, my.mod/module/v2, my.mod/module/v3 are a single series, with the
// series name my.mod/module.
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
		// my.mod/module/v2 has series name my.mod/module
		// my.mod/module/v1 has series name my.mod/module/v1
		// my.mod/module/v2x has series name my.mod/module/v2x
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
// uncompressed size of f exceeds maxFileSize.
func readZipFile(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxFileSize {
		return nil, fmt.Errorf("file size %d exceeds %d, skipping", f.UncompressedSize64, maxFileSize)
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
func detectLicense(r *zip.Reader, contentsDir string) (string, error) {
	for _, f := range r.File {
		if filepath.Dir(f.Name) != contentsDir || !licenseFileNames[filepath.Base(f.Name)] || f.UncompressedSize64 > 1e7 {
			
			
			continue
		}

		bytes, err := readZipFile(f)
		if err != nil {
			return "", fmt.Errorf("readZipFile(%s): %v", f.Name, err)
		}

		cov, ok := license.Cover(bytes, license.Options{})
		if !ok || cov.Percent < licenseCoverageThreshold {
			continue
		}

		m := cov.Match[0]
		if m.Percent > licenseClassifyThreshold {
			return m.Name, nil
		}
	}
	return "", nil
}
