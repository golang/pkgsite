// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package licensedetect detects licenses and determines whether they are redistributable.
// The functions in this package do not return errors; instead, they log any problems
// they encounter and fail closed by reporting that the module or package is not
// redistributable.
//
// Example (modproxy):
//
//	d := licensedetect.NewDetector(modulePath, version, zipReader, log.Infof)
//	modRedist := d.ModuleIsRedistributable()
//
// Example (discovery):
//
//	d := licensedetect.NewDetector(modulePath, version, zipReader, log.Infof)
//	modRedist := d.ModuleIsRedistributable()
//	lics := d.AllLicenses()
//	pkgRedist, pkgMetas := d.PackageInfo(pkgSubdir)
package licensedetect

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/google/licensecheck"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/log"
)

const (
	// coverageThreshold is the minimum percentage of the file that must contain
	// license text.
	coverageThreshold = 75

	// unknownLicenseType is for text in a license file that's not recognized.
	unknownLicenseType = "UNKNOWN"
)

// maxLicenseSize is the maximum allowable size (in bytes) for a license file.
// There are some license files larger than 1 million bytes: https://github.com/vmware/vic/LICENSE
// and github.com/goharbor/harbor/LICENSE, for example.
// var for testing
var maxLicenseSize int64 = modzip.MaxLICENSE

func fromLicensecheckCoverage(c licensecheck.Coverage) licenses.Coverage {
	var matches []licenses.Match
	if c.Match != nil {
		matches = make([]licenses.Match, len(c.Match))
		for i, m := range c.Match {
			matches[i] = licenses.Match{
				ID:    m.ID,
				Type:  uint(m.Type),
				Start: m.Start,
				End:   m.End,
				IsURL: m.IsURL,
			}
		}
	}
	return licenses.Coverage{
		Percent: c.Percent,
		Match:   matches,
	}
}

// fileNamesLowercase has all the entries of FileNames, downcased and made a set
// for fast case-insensitive matching.
var fileNamesLowercase = map[string]bool{}

var (
	// OmitExceptions causes the list of exceptions to be omitted from license detection.
	// It is intended only to speed up testing, and must be set before the first use
	// of this package.
	OmitExceptions bool

	_scanner    *licensecheck.Scanner
	scannerOnce sync.Once
)

func scanner() *licensecheck.Scanner {
	scannerOnce.Do(func() {
		if OmitExceptions {
			exceptionLicenses = nil
		}
		var err error
		_scanner, err = licensecheck.NewScanner(append(exceptionLicenses, licensecheck.BuiltinLicenses()...))
		if err != nil {
			log.Fatalf(context.Background(), "licensecheck.NewScanner: %v", err)
		}
	})
	return _scanner
}

// A Detector detects licenses in a module and its packages.
type Detector struct {
	modulePath     string
	version        string
	fsys           fs.FS
	logf           func(string, ...any)
	moduleRedist   bool
	moduleLicenses []*licenses.License // licenses at module root directory, or list from exceptions
	allLicenses    []*licenses.License
	licsByDir      map[string][]*licenses.License // from directory to list of licenses
}

// NewDetector returns a Detector for the given module and version.
// zr should be the zip file for that module and version.
// logf is for logging; if nil, no logging is done.
// Deprecated: use NewDetectorFS.
func NewDetector(modulePath, version string, zr *zip.Reader, logf func(string, ...any)) *Detector {
	sub, err := fs.Sub(zr, modulePath+"@"+version)
	// This should only fail if the prefix is not a valid path, which shouldn't be possible.
	if err != nil && logf != nil {
		logf("fs.Sub: %v", err)
	}
	return NewDetectorFS(modulePath, version, sub, logf)
}

// NewDetectorFS returns a Detector for the given module and version.
// fsys should represent the content directory of the module (not the zip root).
// logf is for logging; if nil, no logging is done.
func NewDetectorFS(modulePath, version string, fsys fs.FS, logf func(string, ...any)) *Detector {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	d := &Detector{
		modulePath: modulePath,
		version:    version,
		fsys:       fsys,
		logf:       logf,
	}
	d.computeModuleInfo()
	return d
}

// ModuleIsRedistributable reports whether the given module is redistributable.
func (d *Detector) ModuleIsRedistributable() bool {
	return d.moduleRedist
}

// ModuleLicenses returns the licenses that apply to the module.
func (d *Detector) ModuleLicenses() []*licenses.License {
	return d.moduleLicenses
}

// AllLicenses returns all the licenses detected in the entire module, including
// package licenses.
func (d *Detector) AllLicenses() []*licenses.License {
	if d.allLicenses == nil {
		d.computeAllLicenseInfo()
	}
	return d.allLicenses
}

// PackageInfo reports whether the package at dir, a directory relative to the
// module root, is redistributable. It also returns all the licenses that apply
// to the package.
func (d *Detector) PackageInfo(dir string) (isRedistributable bool, lics []*licenses.License) {
	cleanDir := filepath.ToSlash(filepath.Clean(dir))
	if path.IsAbs(cleanDir) || strings.HasPrefix(cleanDir, "..") {
		return false, nil
	}
	if d.allLicenses == nil {
		d.computeAllLicenseInfo()
	}
	// Collect all the license metadata for directories dir and above, excluding the root.
	for prefix, plics := range d.licsByDir {
		// append a slash so that prefix a/b does not match a/bc/d
		if strings.HasPrefix(cleanDir+"/", prefix+"/") {
			lics = append(lics, plics...)
		}
	}
	// A package is redistributable if its module is, and if other licenses on
	// the path to the root are redistributable. Note that this is not the same
	// as asking if the module licenses plus the package licenses are
	// redistributable. A module that is granted an exception (see DetectFiles)
	// may have licenses that are non-redistributable.
	ltypes := types(lics)
	isRedistributable = d.ModuleIsRedistributable() && (len(ltypes) == 0 || licenses.Redistributable(ltypes))
	// A package's licenses include the ones we've already computed, as well
	// as the module licenses.
	return isRedistributable, append(lics, d.moduleLicenses...)
}

// computeModuleInfo determines values for the moduleRedist and moduleLicenses fields of d.
func (d *Detector) computeModuleInfo() {
	// Check that all licenses in the contents directory are redistributable.
	d.moduleLicenses = d.detectFiles(d.paths(RootFiles))
	d.moduleRedist = licenses.Redistributable(types(d.moduleLicenses))
}

// computeAllLicenseInfo collects all the detected licenses in the zip and
// stores them in the allLicenses field of d. It also maps detected licenses to
// their directories, to optimize Detector.PackageInfo.
func (d *Detector) computeAllLicenseInfo() {
	d.allLicenses = []*licenses.License{}
	d.allLicenses = append(d.allLicenses, d.moduleLicenses...)
	nonRootLicenses := d.detectFiles(d.paths(NonRootFiles))
	d.allLicenses = append(d.allLicenses, nonRootLicenses...)
	d.licsByDir = map[string][]*licenses.License{}
	for _, l := range nonRootLicenses {
		prefix := path.Dir(l.FilePath)
		d.licsByDir[prefix] = append(d.licsByDir[prefix], l)
	}
}

// WhichFiles describes which files from the zip should be returned by Detector.Files.
type WhichFiles int

const (
	// Only files from the root (contents) directory.
	RootFiles WhichFiles = iota
	// Only files that are not in the root directory.
	NonRootFiles
	// All files; the union of root and non-root.
	AllFiles
)

// paths returns a list of license file paths from the Detector's filesystem.
// The which argument determines the location of the files considered.
// If paths encounters an error, it logs it and returns nil.
func (d *Detector) paths(which WhichFiles) []string {
	if d.fsys == nil {
		return nil
	}
	var paths []string
	err := fs.WalkDir(d.fsys, ".", func(pathname string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() {
			return nil
		}
		if !fileNamesLowercase[strings.ToLower(de.Name())] {
			return nil
		}
		// Skip files we should ignore.
		if ignoreFiles[d.modulePath+" "+pathname] {
			return nil
		}
		if which == RootFiles && path.Dir(pathname) != "." {
			// Skip f since it's not at root.
			return nil
		}
		if which == NonRootFiles && path.Dir(pathname) == "." {
			// Skip f since it is at root.
			return nil
		}
		if isVendoredFile(pathname) {
			// Skip if f is in the vendor directory.
			return nil
		}
		if err := module.CheckFilePath(pathname); err != nil {
			// Skip if the file path is bad.
			d.logf("module.CheckFilePath(%q): %v", pathname, err)
			return nil
		}
		paths = append(paths, pathname)
		return nil
	})
	if err != nil {
		d.logf("licenses.Detector.paths: %v", err)
		return nil
	}
	return paths
}

// isVendoredFile reports if the given file is in a proper subdirectory nested
// under a 'vendor' directory, to allow for Go packages named 'vendor'.
// For example:
//   - isVendoredFile("vendor/LICENSE") == false, and
//   - isVendoredFile("vendor/foo/LICENSE") == true.
func isVendoredFile(name string) bool {
	var vendorOffset int
	if strings.HasPrefix(name, "vendor/") {
		vendorOffset = len("vendor/")
	} else if i := strings.Index(name, "/vendor/"); i >= 0 {
		vendorOffset = i + len("/vendor/")
	} else {
		// no vendor directory
		return false
	}
	// check if the file is in a proper subdirectory of vendor
	return strings.Contains(name[vendorOffset:], "/")
}

// detectFiles runs DetectFile on each of the given files.
// If a file cannot be read, the error is logged and a license
// of type unknown is added.
func (d *Detector) detectFiles(pathnames []string) []*licenses.License {
	var lics []*licenses.License
	for _, p := range pathnames {
		bytes, err := d.readFile(p)
		if err != nil {
			d.logf("reading file %s: %v", p, err)
			lics = append(lics, &licenses.License{
				Metadata: &licenses.Metadata{
					Types:    []string{unknownLicenseType},
					FilePath: p,
				},
			})
			continue
		}
		types, cov := DetectFile(bytes, p, d.logf)
		lics = append(lics, &licenses.License{
			Metadata: &licenses.Metadata{
				Types:    types,
				FilePath: p,
				Coverage: cov,
			},
			Contents: bytes,
		})
	}
	return lics
}

func (d *Detector) readFile(pathname string) ([]byte, error) {
	f, err := d.fsys.Open(pathname)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxLicenseSize {
		return nil, fmt.Errorf("file size %d exceeds max license size %d", info.Size(), maxLicenseSize)
	}
	return io.ReadAll(io.LimitReader(f, int64(maxLicenseSize)))
}

// DetectFile return the set of license types for the given file contents. It
// also returns the licensecheck coverage information. The filename is used
// solely for logging.
func DetectFile(contents []byte, filename string, logf func(string, ...any)) ([]string, licenses.Coverage) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	cov := fromLicensecheckCoverage(scanner().Scan(contents))
	if cov.Percent < float64(coverageThreshold) {
		logf("%s license coverage too low (%+v), skipping", filename, cov)
		return []string{unknownLicenseType}, cov
	}
	types := make(map[string]bool)
	for _, m := range cov.Match {
		ts := exceptionTypes[m.ID]
		if ts == nil {
			ts = []string{m.ID}
		}
		for _, t := range ts {
			types[t] = true
		}
	}
	if len(types) == 0 {
		logf("%s failed to classify license (%+v), skipping", filename, cov)
		return []string{unknownLicenseType}, cov
	}
	return slices.Sorted(maps.Keys(types)), cov
}

func types(lics []*licenses.License) []string {
	var types []string
	for _, l := range lics {
		types = append(types, l.Types...)
	}
	return types
}
