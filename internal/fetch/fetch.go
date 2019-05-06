// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"go/doc"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/thirdparty/modfile"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"golang.org/x/tools/go/packages"
)

var (
	errModuleContainsNoPackages = errors.New("module contains 0 packages")
	errReadmeNotFound           = errors.New("module does not contain a README")

	// fetchTimeout bounds the time allowed for fetching a single module.  It is
	// mutable for testing purposes.
	fetchTimeout = 5 * time.Minute

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
func FetchAndInsertVersion(modulePath, version string, proxyClient *proxy.Client, db *postgres.DB) (err error) {
	defer func() {
		if err != nil && err != context.DeadlineExceeded {
			if dberr := db.UpdateVersionLogError(context.Background(), modulePath, version, err); dberr != nil {
				log.Printf("db.UpdateVersionLogError(ctx, %q, %q, %v): %v", modulePath, version, err, dberr)
			}
		}
	}()
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

	readmeFilePath, readmeContents, err := extractReadmeFromZip(modulePath, version, zipReader)
	if err != nil && err != errReadmeNotFound {
		return fmt.Errorf("extractReadmeFromZip(%q, %q, zipReader): %v", modulePath, version, err)
	}

	licenses, err := license.Detect(moduleVersionDir(modulePath, version), zipReader)
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
			SeriesPath:     seriesPath,
			ModulePath:     modulePath,
			Version:        version,
			CommitTime:     info.Time,
			ReadmeFilePath: readmeFilePath,
			ReadmeContents: readmeContents,
			VersionType:    versionType,
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

// moduleVersionDir formats the content subdirectory for the given
// modulePath and version.
func moduleVersionDir(modulePath, version string) string {
	return fmt.Sprintf("%s@%s", modulePath, version)
}

// extractReadmeFromZip returns the file path and contents of the first file
// from r that is a README file. errReadmeNotFound is returned if a README is
// not found.
func extractReadmeFromZip(modulePath, version string, r *zip.Reader) (string, []byte, error) {
	for _, zipFile := range r.File {
		if hasFilename(zipFile.Name, "README") {
			c, err := readZipFile(zipFile)
			if err != nil {
				return "", nil, fmt.Errorf("readZipFile(%q): %v", zipFile.Name, err)
			}
			return strings.TrimPrefix(zipFile.Name, moduleVersionDir(modulePath, version)+"/"), c, nil
		}
	}
	return "", nil, errReadmeNotFound
}

// extractPackagesFromZip returns a slice of packages from the module zip r.
// It matches against the given licenses to determine the subset of licenses
// that applies to each package.
func extractPackagesFromZip(modulePath, version string, r *zip.Reader, licenses []*license.License) ([]*internal.Package, error) {
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

	// If the module doesn't have an explicit go.mod file at the root,
	// write one ourselves. Otherwise, it's not possible for go/packages
	// to know where it's located on disk when it's the main module.
	goMod := filepath.Join(workDir, modulePath+"@"+version, "go.mod")
	if _, err := os.Stat(goMod); os.IsNotExist(err) {
		if err := writeGoModFile(goMod, modulePath); err != nil {
			return nil, fmt.Errorf("writeGoModFile(%q, %q): %v", goMod, modulePath, err)
		}
	}

	pkgs, err := loadAndProcessPackages(workDir, modulePath, version, licenses)
	if err != nil {
		return nil, fmt.Errorf("loadAndProcessPackages(%q, %q, %q, licenses): %v", workDir, modulePath, version, err)
	}
	return pkgs, nil
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
			// Skip files that are not .go files and are greater than maxFileSize.
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

// writeGoModFile writes a go.mod file with a module statement at filename.
//
// It can be used on modules that don't have an explicit go.mod file,
// so that it's possible to treat such modules as main modules.
func writeGoModFile(filename, modulePath string) error {
	var f modfile.File
	if err := f.AddModuleStmt(modulePath); err != nil {
		return fmt.Errorf("f.AddModuleStmt(%q): %v", modulePath, err)
	}
	b, err := f.Format()
	if err != nil {
		return fmt.Errorf("f.Format(): %v", err)
	}
	err = ioutil.WriteFile(filename, b, 0600)
	if err != nil {
		return fmt.Errorf("ioutil.WriteFile(%q, b, 0600): %v", filename, err)
	}
	return nil
}

// loadAndProcessPackages loads packages using the default build context
// from the given modulePath and version relative to the working directory.
// It matches each package to an applicable license and computes its imports,
// documentation, and other package-specific fields.
//
// If there were no packages found in the module, the error value
// errModuleContainsNoPackages is returned.
func loadAndProcessPackages(workDir, modulePath, version string, licenses []*license.License) ([]*internal.Package, error) {
	// TODO: find a way to test that the configuration doesn't use the internet to fetch dependencies
	moduleRoot := filepath.Join(workDir, moduleVersionDir(modulePath, version))
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles,
		Dir:  moduleRoot,
	}
	pattern := fmt.Sprintf("%s/...", modulePath)
	pkgs, err := packages.Load(config, pattern)
	if err != nil {
		return nil, fmt.Errorf("packages.Load(config, %q): %v", pattern, err)
	}
	if len(pkgs) == 0 {
		return nil, errModuleContainsNoPackages
	}
	if len(pkgs) > maxPackagesPerModule {
		return nil, fmt.Errorf("%d packages found in %q; exceeds limit %d for maxPackagePerModule", len(pkgs), modulePath, maxPackagesPerModule)
	}
	// TODO: consider p.Errors and act accordingly; issue b/131836733

	licenseMatcher := license.NewMatcher(licenses)
	var packages []*internal.Package
	for _, p := range pkgs {
		fset, d, err := computeDoc(p)
		if err != nil {
			return nil, err
		}

		if len(d.Imports) > maxImportsPerPackage {
			return nil, fmt.Errorf("%d imports found package %q in module %q; exceeds limit %d for maxImportsPerPackage", len(pkgs), p.PkgPath, modulePath, maxImportsPerPackage)
		}
		var imports []*internal.Import
		for _, i := range d.Imports {
			imports = append(imports, &internal.Import{
				Name: path.Base(i), // TODO: this is a heuristic that just uses last path element for now; need to use database to do better; issue b/131835416
				Path: i,
			})
		}

		docHTML, err := renderDocHTML(fset, d)
		if err != nil {
			return nil, err
		}

		var packageDir string
		if len(p.GoFiles) > 0 {
			packageDir = filepath.Dir(strings.TrimPrefix(p.GoFiles[0], moduleRoot+"/"))
		} else {
			// by default, all root-level licenses should apply
			packageDir = "."
		}

		packages = append(packages, &internal.Package{
			Name:              p.Name,
			Path:              p.PkgPath,
			Licenses:          licenseMatcher.Match(packageDir),
			Synopsis:          doc.Synopsis(d.Doc),
			Suffix:            strings.TrimPrefix(strings.TrimPrefix(p.PkgPath, modulePath), "/"),
			Imports:           imports,
			DocumentationHTML: docHTML,
		})
	}
	return packages, nil
}

// hasFilename checks if file is expectedFile or if the name of file, without
// the base, is equal to expectedFile. It is case insensitive.
func hasFilename(file string, expectedFile string) bool {
	base := filepath.Base(file)
	return strings.EqualFold(file, expectedFile) ||
		strings.EqualFold(base, expectedFile) ||
		strings.EqualFold(strings.TrimSuffix(base, filepath.Ext(base)), expectedFile)
}
