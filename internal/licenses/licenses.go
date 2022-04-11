// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package licenses detects licenses and determines whether they are redistributable.
// The functions in this package do not return errors; instead, they log any problems
// they encounter and fail closed by reporting that the module or package is not
// redistributable.
//
// Example (modproxy):
//
//	d := licenses.NewDetector(modulePath, version, zipReader, log.Infof)
//	modRedist := d.ModuleIsRedistributable()
//
// Example (discovery):
//
//	d := licenses.NewDetector(modulePath, version, zipReader, log.Infof)
//	modRedist := d.ModuleIsRedistributable()
//	lics := d.AllLicenses()
//	pkgRedist, pkgMetas := d.PackageInfo(pkgSubdir)
package licenses

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/licensecheck"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
	"golang.org/x/pkgsite/internal/log"
)

//go:generate rm -f exceptions.gen.go
//go:generate go run gen_exceptions.go

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

// Metadata holds information extracted from a license file.
type Metadata struct {
	// Types is the set of license types, as determined by the licensecheck package.
	Types []string
	// FilePath is the '/'-separated path to the license file in the module zip,
	// relative to the contents directory.
	FilePath string
	Coverage licensecheck.Coverage
}

// A License is a classified license file path and its contents.
type License struct {
	*Metadata
	Contents []byte
}

// RemoveNonRedistributableData methods removes the license contents
// if the license is non-redistributable.
func (l *License) RemoveNonRedistributableData() {
	if !Redistributable(l.Types) {
		l.Contents = nil
	}
}

var (
	FileNames = []string{
		"COPYING",
		"COPYING.md",
		"COPYING.markdown",
		"COPYING.txt",
		"LICENCE",
		"LICENCE.md",
		"LICENCE.markdown",
		"LICENCE.txt",
		"LICENSE",
		"LICENSE.md",
		"LICENSE.markdown",
		"LICENSE.txt",
		"LICENSE-2.0.txt",
		"LICENCE-2.0.txt",
		"LICENSE-APACHE",
		"LICENCE-APACHE",
		"LICENSE-APACHE-2.0.txt",
		"LICENCE-APACHE-2.0.txt",
		"LICENSE-MIT",
		"LICENCE-MIT",
		"LICENSE.MIT",
		"LICENCE.MIT",
		"LICENSE.code",
		"LICENCE.code",
		"LICENSE.docs",
		"LICENCE.docs",
		"LICENSE.rst",
		"LICENCE.rst",
		"MIT-LICENSE",
		"MIT-LICENCE",
		"MIT-LICENSE.md",
		"MIT-LICENCE.md",
		"MIT-LICENSE.markdown",
		"MIT-LICENCE.markdown",
		"MIT-LICENSE.txt",
		"MIT-LICENCE.txt",
		"MIT_LICENSE",
		"MIT_LICENCE",
		"UNLICENSE",
		"UNLICENCE",
	}

	// standardRedistributableLicenseTypes is the list of license types, as reported by
	// licensecheck, that allow redistribution, and also have a name that is an OSI or SPDX
	// identifier.
	standardRedistributableLicenseTypes = []string{
		// Licenses acceptable by OSI.
		"AFL-3.0",
		"AGPL-3.0",
		"AGPL-3.0-only",
		"AGPL-3.0-or-later",
		"Apache-1.1",
		"Apache-2.0",
		"Artistic-2.0",
		"BlueOak-1.0.0",
		"0BSD",
		"BSD-1-Clause",
		"BSD-2-Clause",
		"BSD-2-Clause-Patent",
		"BSD-2-Clause-Views",
		"BSD-3-Clause",
		"BSD-3-Clause-Clear",
		"BSD-3-Clause-Open-MPI",
		"BSD-4-Clause",
		"BSD-4-Clause-UC",
		"BSL-1.0",
		"CC-BY-3.0",
		"CC-BY-4.0",
		"CC-BY-SA-3.0",
		"CC-BY-SA-4.0",
		"CECILL-2.1",
		"CC0-1.0",
		"EPL-1.0",
		"EPL-2.0",
		"EUPL-1.2",
		"GPL-2.0",
		"GPL-2.0-only",
		"GPL-2.0-or-later",
		"GPL-3.0",
		"GPL-3.0-only",
		"GPL-3.0-or-later",
		"HPND",
		"ISC",
		"JSON",
		"LGPL-2.1",
		"LGPL-2.1-or-later",
		"LGPL-3.0",
		"LGPL-3.0-or-later",
		"MIT",
		"MIT-0",
		"MPL-2.0",
		"MPL-2.0-no-copyleft-exception",
		"MulanPSL-2.0",
		"NIST-PD",
		"NIST-PD-fallback",
		"NCSA",
		"OpenSSL",
		"OSL-3.0",
		"PostgreSQL", // TODO: ask legal
		"Python-2.0",
		"Unlicense",
		"UPL-1.0",
		"Zlib",
	}

	// These aren't technically licenses, but they are recognized by
	// licensecheck and safe to ignore.
	ignorableLicenseTypes = map[string]bool{
		"CC-Notice":          true,
		"GooglePatentClause": true,
		"GooglePatentsFile":  true,
		"blessing":           true,
		"OFL-1.1":            true, // concerns fonts only
	}

	// redistributableLicenseTypes is the set of license types, as reported by
	// licensecheck, that allow redistribution. It consists of the standard
	// types along with some exception types.
	redistributableLicenseTypes = map[string]bool{}
)

func init() {
	for _, t := range standardRedistributableLicenseTypes {
		redistributableLicenseTypes[t] = true
	}
	// Add here all other types defined in the exceptions.
	redistributableLicenseTypes["Freetype"] = true

	// exceptionTypes is a map from License IDs from LREs in the exception
	// directory to license types. Any type mentioned in an exception should
	// be redistributable. If not, there's a problem.
	for _, types := range exceptionTypes {
		for _, t := range types {
			if !redistributableLicenseTypes[t] {
				log.Fatalf(context.Background(), "%s is an exception type that is not redistributable.", t)
			}
		}
	}
}

// nonOSILicenses lists licenses that are not approved by OSI.
var nonOSILicenses = map[string]bool{
	"BlueOak-1.0.0":      true,
	"BSD-2-Clause-Views": true,
	"CC-BY-3.0":          true,
	"CC-BY-4.0":          true,
	"CC-BY-SA-3.0":       true,
	"CC-BY-SA-4.0":       true,
	"CC0-1.0":            true,
	"JSON":               true,
	"NIST":               true,
	"OpenSSL":            true,
}

// fileNamesLowercase has all the entries of FileNames, downcased and made a set
// for fast case-insensitive matching.
var fileNamesLowercase = map[string]bool{}

func init() {
	for _, f := range FileNames {
		fileNamesLowercase[strings.ToLower(f)] = true
	}
}

// AcceptedLicenseInfo describes a license that is accepted by the discovery site.
type AcceptedLicenseInfo struct {
	Name string
	URL  string
}

// AcceptedLicenses returns a sorted slice of license types that are accepted as
// redistributable. Its result is intended to be displayed to users.
func AcceptedLicenses() []AcceptedLicenseInfo {
	var lics []AcceptedLicenseInfo
	for _, identifier := range standardRedistributableLicenseTypes {
		var link string
		if nonOSILicenses[identifier] {
			link = fmt.Sprintf("https://spdx.org/licenses/%s.html", identifier)
		} else {
			link = fmt.Sprintf("https://opensource.org/licenses/%s", identifier)
		}
		lics = append(lics, AcceptedLicenseInfo{identifier, link})
	}
	sort.Slice(lics, func(i, j int) bool { return lics[i].Name < lics[j].Name })
	return lics
}

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
	logf           func(string, ...interface{})
	moduleRedist   bool
	moduleLicenses []*License // licenses at module root directory, or list from exceptions
	allLicenses    []*License
	licsByDir      map[string][]*License // from directory to list of licenses
}

// NewDetector returns a Detector for the given module and version.
// zr should be the zip file for that module and version.
// logf is for logging; if nil, no logging is done.
// Deprecated: use NewDetectorFS.
func NewDetector(modulePath, version string, zr *zip.Reader, logf func(string, ...interface{})) *Detector {
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
func NewDetectorFS(modulePath, version string, fsys fs.FS, logf func(string, ...interface{})) *Detector {
	if logf == nil {
		logf = func(string, ...interface{}) {}
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
func (d *Detector) ModuleLicenses() []*License {
	return d.moduleLicenses
}

// AllLicenses returns all the licenses detected in the entire module, including
// package licenses.
func (d *Detector) AllLicenses() []*License {
	if d.allLicenses == nil {
		d.computeAllLicenseInfo()
	}
	return d.allLicenses
}

// PackageInfo reports whether the package at dir, a directory relative to the
// module root, is redistributable. It also returns all the licenses that apply
// to the package.
func (d *Detector) PackageInfo(dir string) (isRedistributable bool, lics []*License) {
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
	isRedistributable = d.ModuleIsRedistributable() && (len(ltypes) == 0 || Redistributable(ltypes))
	// A package's licenses include the ones we've already computed, as well
	// as the module licenses.
	return isRedistributable, append(lics, d.moduleLicenses...)
}

// computeModuleInfo determines values for the moduleRedist and moduleLicenses fields of d.
func (d *Detector) computeModuleInfo() {
	// Check that all licenses in the contents directory are redistributable.
	d.moduleLicenses = d.detectFiles(d.paths(RootFiles))
	d.moduleRedist = Redistributable(types(d.moduleLicenses))
}

// computeAllLicenseInfo collects all the detected licenses in the zip and
// stores them in the allLicenses field of d. It also maps detected licenses to
// their directories, to optimize Detector.PackageInfo.
func (d *Detector) computeAllLicenseInfo() {
	d.allLicenses = []*License{}
	d.allLicenses = append(d.allLicenses, d.moduleLicenses...)
	nonRootLicenses := d.detectFiles(d.paths(NonRootFiles))
	d.allLicenses = append(d.allLicenses, nonRootLicenses...)
	d.licsByDir = map[string][]*License{}
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
func (d *Detector) detectFiles(pathnames []string) []*License {
	var licenses []*License
	for _, p := range pathnames {
		bytes, err := d.readFile(p)
		if err != nil {
			d.logf("reading file %s: %v", p, err)
			licenses = append(licenses, &License{
				Metadata: &Metadata{
					Types:    []string{unknownLicenseType},
					FilePath: p,
				},
			})
			continue
		}
		types, cov := DetectFile(bytes, p, d.logf)
		licenses = append(licenses, &License{
			Metadata: &Metadata{
				Types:    types,
				FilePath: p,
				Coverage: cov,
			},
			Contents: bytes,
		})
	}
	return licenses
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
	return ioutil.ReadAll(io.LimitReader(f, int64(maxLicenseSize)))
}

// DetectFile return the set of license types for the given file contents. It
// also returns the licensecheck coverage information. The filename is used
// solely for logging.
func DetectFile(contents []byte, filename string, logf func(string, ...interface{})) ([]string, licensecheck.Coverage) {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	cov := scanner().Scan(contents)
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
	return setToSortedSlice(types), cov
}

// Redistributable reports whether the set of license types establishes that a
// module or package is redistributable.
// All the licenses we see that are relevant must be redistributable, and
// we must see at least one such license.
func Redistributable(licenseTypes []string) bool {
	sawRedist := false
	for _, t := range licenseTypes {
		if ignorableLicenseTypes[t] {
			continue
		}
		if !redistributableLicenseTypes[t] {
			return false
		}
		sawRedist = true
	}
	return sawRedist
}

func types(lics []*License) []string {
	var types []string
	for _, l := range lics {
		types = append(types, l.Types...)
	}
	return types
}

func setToSortedSlice(m map[string]bool) []string {
	var s []string
	for e := range m {
		s = append(s, e)
	}
	sort.Strings(s)
	return s
}
