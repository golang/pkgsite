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
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
	maxFileSize          = uint64(1e7)
	maxPackagesPerModule = 10000
	maxImportsPerPackage = 1000
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
// (3) Process the contents (series path, readme, licenses, and packages)
// (4) Write the data to the discovery database
func FetchAndInsertVersion(modulePath, version string, proxyClient *proxy.Client, db *postgres.DB) error {
	// Unlike other actions (which use a Timeout middleware), we set a fixed
	// timeout for FetchAndInsertVersion.  This allows module processing to
	// succeed even for extremely short lived requests.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	if err := module.CheckPath(modulePath); err != nil {
		return fmt.Errorf("fetch: invalid module name %v: %v", modulePath, err)
	}

	versionType, err := parseVersion(version)
	if err != nil {
		return fmt.Errorf("parseVersion(%q): %v", version, err)
	}

	info, err := proxyClient.GetInfo(ctx, modulePath, version)
	if err != nil {
		return fmt.Errorf("proxyClient.GetInfo(%q, %q): %v", modulePath, version, err)
	}

	zipReader, err := proxyClient.GetZip(ctx, modulePath, version)
	if err != nil {
		return fmt.Errorf("proxyClient.GetZip(%q, %q): %v", modulePath, version, err)
	}

	readme, err := extractReadmeFromZip(zipReader)
	if err != nil {
		return fmt.Errorf("extractReadmeFromZip(zipReader): %v", err)
	}

	licenses, err := detectLicenses(zipReader)
	if err != nil {
		log.Printf("Error detecting licenses for %v@%v: %v", modulePath, version, err)
	}

	packages, err := extractPackagesFromZip(modulePath, version, zipReader, licenses)
	if err != nil {
		return fmt.Errorf("extractPackagesFromZip(%q, %q, zipReader, %v): %v", modulePath, version, licenses, err)
	}

	seriesPath, _, _ := module.SplitPathVersion(modulePath)
	v := &internal.Version{
		VersionInfo: internal.VersionInfo{
			SeriesPath:  seriesPath,
			ModulePath:  modulePath,
			Version:     version,
			CommitTime:  info.Time,
			ReadMe:      readme,
			VersionType: versionType,
		},
		Packages: packages,
	}
	if err = db.InsertVersion(ctx, v, licenses); err != nil {
		return fmt.Errorf("db.InsertVersion for %q %q: %v", modulePath, version, err)
	}
	if err = db.InsertDocuments(ctx, v); err != nil {
		return fmt.Errorf("db.InsertDocuments for %q %q: %v", modulePath, version, err)
	}
	return nil
}

// extractReadmeFromZip returns the README content, if found, else nil.  It
// returns error if the README file cannot be read.
func extractReadmeFromZip(r *zip.Reader) ([]byte, error) {
	var (
		readme []byte
		err    error
	)
	readmeFile := "README"
	if containsFile(r, readmeFile) {
		readme, err = extractFile(r, readmeFile)
		if err != nil {
			return nil, fmt.Errorf("extractFile(%v, %q): %v", r, readmeFile, err)
		}
	}
	return readme, nil
}

// extractPackagesFromZip returns a slice of packages from the module zip r.
// It matches against the given licenses to determine the subset of licenses
// that applies to each package.
func extractPackagesFromZip(modulePath, version string, r *zip.Reader, licenses []*internal.License) ([]*internal.Package, error) {
	// Create a temporary directory to write the contents of the module zip.
	tempPrefix := "discovery_"
	workDir, err := ioutil.TempDir("", tempPrefix)
	if err != nil {
		return nil, fmt.Errorf("ioutil.TempDir(%q, %q): %v", "", tempPrefix, err)
	}
	defer os.RemoveAll(workDir)

	if err := extractModuleFiles(workDir, modulePath, r); err != nil {
		return nil, fmt.Errorf("extractModuleFiles(%q, %q, zipReader): %v", workDir, modulePath, err)
	}

	pkgs, err := loadPackages(workDir, modulePath, version)
	if err != nil && err != errModuleContainsNoPackages {
		return nil, fmt.Errorf("loadPackages(%q, %q, %q, zipReader): %v", workDir, modulePath, version, err)
	}

	packages, err := transformPackages(workDir, modulePath, pkgs, licenses)
	if err != nil {
		return nil, fmt.Errorf("transformPackages(%q, %q, pkgs, licenses): %v", workDir, modulePath, err)
	}
	return packages, nil
}

// extractModuleFiles extracts files contained in the given module within the
// working directory. It returns error if the given *zip.Reader contains files
// outside of the expected module path, if a Go file exceeds the maximum
// allowable file size, or if there is an error writing the extracted file.
// Notably, it simply skips over non-Go files that exceed the maximum file size.
func extractModuleFiles(workDir, modulePath string, r *zip.Reader) error {
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, modulePath) && !strings.HasPrefix(modulePath, f.Name) {
			return fmt.Errorf("expected files to have shared prefix %q, found %q", modulePath, f.Name)
		}

		if !f.FileInfo().IsDir() {
			// Skip files that are not .go files and are greater than 10MB.
			if filepath.Ext(f.Name) != ".go" && f.UncompressedSize64 > maxFileSize {
				continue
			}

			if err := writeFileToDir(f, workDir); err != nil {
				return fmt.Errorf("writeFileToDir(%q, %q): %v", f.Name, workDir, err)
			}
		}
	}
	return nil
}

// loadPackages calls packages.Load for the given modulePath and version within
// the working directory.
//
// It returns the special error errModuleContainsNoPackages if the module
// contains no packages.
func loadPackages(workDir, modulePath, version string) ([]*packages.Package, error) {
	config := &packages.Config{
		Mode: packages.LoadSyntax,
		Dir:  fmt.Sprintf("%s/%s@%s", workDir, modulePath, version),
	}
	pattern := fmt.Sprintf("%s/...", modulePath)
	pkgs, err := packages.Load(config, pattern)
	if err != nil {
		return nil, fmt.Errorf("packages.Load(config, %q): %v", pattern, err)
	}

	if len(pkgs) == 0 {
		return nil, errModuleContainsNoPackages
	}

	return pkgs, nil
}

// licenseMatcher is a map of directory prefix -> license files, that can be
// used to match packages to their applicable licenses.
type licenseMatcher map[string][]internal.LicenseInfo

// newLicenseMatcher creates a licenseMatcher that can be used match licenses
// against packages extracted to the given workDir.
func newLicenseMatcher(workDir string, licenses []*internal.License) licenseMatcher {
	var matcher licenseMatcher = make(map[string][]internal.LicenseInfo)
	for _, l := range licenses {
		
		path := filepath.Join(workDir, l.FilePath)
		prefix := filepath.ToSlash(filepath.Clean(filepath.Dir(path)))
		matcher[prefix] = append(matcher[prefix], l.LicenseInfo)
	}
	return matcher
}

// matchLicenses returns the slice of licenses that apply to the given package.
// A license applies to a package if it is contained in a directory that
// precedes the package directory in a directory hierarchy (i.e., a direct or
// indirect parent of the package directory).
func (m licenseMatcher) matchLicenses(p *packages.Package) []*internal.LicenseInfo {
	if len(p.GoFiles) == 0 {
		return nil
	}
	// Since we're only operating on modules, package dir should be deterministic
	// based on the location of Go files.
	pkgDir := filepath.ToSlash(filepath.Clean(filepath.Dir(p.GoFiles[0])))

	var licenseFiles []*internal.LicenseInfo
	for prefix, lics := range m {
		// append a slash so that prefix a/b does not match a/bc/d
		if strings.HasPrefix(pkgDir+"/", prefix+"/") {
			for _, lic := range lics {
				lf := lic
				licenseFiles = append(licenseFiles, &lf)
			}
		}
	}
	return licenseFiles
}

// transformPackages maps a slice of *packages.Package to our internal
// representation (*internal.Package).  To do so, it maps package data
// and matches packages with their applicable licenses.
func transformPackages(workDir, modulePath string, pkgs []*packages.Package, licenses []*internal.License) ([]*internal.Package, error) {
	matcher := newLicenseMatcher(workDir, licenses)
	packages := []*internal.Package{}

	if len(pkgs) > maxPackagesPerModule {
		return nil, fmt.Errorf("%d packages found in %q; exceeds limit %d for maxPackagePerModule", len(pkgs), modulePath, maxPackagesPerModule)
	}

	for _, p := range pkgs {
		var imports []*internal.Import
		if len(p.Imports) > maxImportsPerPackage {
			return nil, fmt.Errorf("%d imports found package %q in module %q; exceeds limit %d for maxImportsPerPackage", len(pkgs), p.PkgPath, modulePath, maxImportsPerPackage)
		}
		for _, i := range p.Imports {
			imports = append(imports, &internal.Import{
				Name: i.Name,
				Path: i.PkgPath,
			})
		}

		packages = append(packages, &internal.Package{
			Name:     p.Name,
			Path:     p.PkgPath,
			Licenses: matcher.matchLicenses(p),
			Synopsis: synopsis(p),
			Suffix:   strings.TrimPrefix(strings.TrimPrefix(p.PkgPath, modulePath), "/"),
			Imports:  imports,
		})
	}
	return packages, nil
}

// synopsis returns the first sentence of the package documentation, or an
// empty string if it cannot be determined.
func synopsis(p *packages.Package) string {
	files := make(map[string]*ast.File)
	for i, f := range p.Syntax {
		files[string(i)] = f
	}

	apkg := &ast.Package{
		Name:  p.Name,
		Files: files,
	}
	d := doc.New(apkg, p.PkgPath, 0)
	return doc.Synopsis(d.Doc)
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
