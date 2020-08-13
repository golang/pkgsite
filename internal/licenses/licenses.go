// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package licenses detects licenses and determines whether they are redistributable.
// The functions in this package do not return errors; instead, they log any problems
// they encounter and fail closed by reporting that the module or package is not
// redistributable.
//
// Example (modproxy):
//   d := licenses.NewDetector(modulePath, version, zipReader, log.Infof)
//   modRedist := d.ModuleIsRedistributable()
//
// Example (discovery):
//   d := licenses.NewDetector(modulePath, version, zipReader, log.Infof)
//   modRedist := d.ModuleIsRedistributable()
//   lics := d.AllLicenses()
//   pkgRedist, pkgMetas := d.PackageInfo(pkgSubdir)
package licenses

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/licensecheck"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

//go:generate rm -f exceptions.gen.go
//go:generate go run gen_exceptions.go

const (
	// classifyThreshold is the minimum confidence percentage/threshold to
	// classify a license
	classifyThreshold = 90

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
var maxLicenseSize uint64 = modzip.MaxLICENSE

// Metadata holds information extracted from a license file.
type Metadata struct {
	// Types is the set of license types, as determined by the licensecheck package.
	Types []string
	// FilePath is the '/'-separated path to the license file in the module zip,
	// relative to the contents directory.
	FilePath string
	// The output of licensecheck.Cover.
	Coverage licensecheck.Coverage
}

// A License is a classified license file path and its contents.
type License struct {
	*Metadata
	Contents []byte
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

	// redistributableLicenseTypes is the list of license types, as reported by
	// licensecheck, that allow redistribution.
	redistributableLicenseTypes = map[string]bool{
		// Licenses acceptable by OSI.
		"AGPL-3.0":             true,
		"Apache-2.0":           true,
		"Artistic-2.0":         true,
		"BlueOak-1.0":          true,
		"BSD-0-Clause":         true,
		"BSD-2-Clause":         true,
		"BSD-2-Clause-FreeBSD": true,
		"BSD-3-Clause":         true,
		"BSL-1.0":              true,
		"CC-BY-3.0":            true,
		"CC-BY-4.0":            true,
		"CC-BY-SA-3.0":         true,
		"CC-BY-SA-4.0":         true,
		"CC0-1.0":              true,
		"EPL-1.0":              true,
		"EPL-2.0":              true,
		"GPL2":                 true,
		"GPL3":                 true,
		"ISC":                  true,
		"JSON":                 true,
		"LGPL-2.1":             true,
		"LGPL-3.0":             true,
		"MIT":                  true,
		"MIT-0":                true,
		"MPL-2.0":              true,
		"NCSA":                 true,
		"OpenSSL":              true,
		"OSL-3.0":              true,
		"Unlicense":            true,
		"Zlib":                 true,
	}

	// These aren't technically licenses, but they are recognized by
	// licensecheck and safe to ignore.
	ignorableLicenseTypes = map[string]bool{
		"CC-Notice":          true,
		"GooglePatentClause": true,
	}
)

// osiNameOverrides maps a licensecheck license type to the corresponding OSI
// name, if they differ.
var osiNameOverrides = map[string]string{
	"GPL2": "GPL-2.0",
	"GPL3": "GPL-3.0",
}

// nonOSILicenses lists licenses that are not approved by OSI.
var nonOSILicenses = map[string]bool{
	"BlueOak-1.0":          true,
	"BSD-0-Clause":         true,
	"BSD-2-Clause-FreeBSD": true,
	"CC-BY-3.0":            true,
	"CC-BY-4.0":            true,
	"CC-BY-SA-3.0":         true,
	"CC-BY-SA-4.0":         true,
	"CC0-1.0":              true,
	"JSON":                 true,
	"MIT-0":                true,
	"OpenSSL":              true,
	"Unlicense":            true,
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
	for l := range redistributableLicenseTypes {
		osiName := osiNameOverrides[l]
		if osiName == "" {
			osiName = l
		}
		var link string
		if !nonOSILicenses[l] {
			link = fmt.Sprintf("https://opensource.org/licenses/%s", osiName)
		}
		lics = append(lics, AcceptedLicenseInfo{osiName, link})
	}
	sort.Slice(lics, func(i, j int) bool { return lics[i].Name < lics[j].Name })
	return lics
}

var checker *licensecheck.Checker = licensecheck.New(licensecheck.BuiltinLicenses())

// A Detector detects licenses in a module and its packages.
type Detector struct {
	modulePath     string
	version        string
	zr             *zip.Reader
	logf           func(string, ...interface{})
	moduleRedist   bool
	moduleLicenses []*License // licenses at module root directory, or list from exceptions
	allLicenses    []*License
	licsByDir      map[string][]*License // from directory to list of licenses
}

// NewDetector returns a Detector for the given module and version.
// zr should be the zip file for that module and version.
// logf is for logging; if nil, no logging is done.
func NewDetector(modulePath, version string, zr *zip.Reader, logf func(string, ...interface{})) *Detector {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	d := &Detector{
		modulePath: modulePath,
		version:    version,
		zr:         zr,
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
	d.moduleLicenses = d.detectFiles(d.Files(RootFiles))
	d.moduleRedist = Redistributable(types(d.moduleLicenses))
}

// computeAllLicenseInfo collects all the detected licenses in the zip and
// stores them in the allLicenses field of d. It also maps detected licenses to
// their directories, to optimize Detector.PackageInfo.
func (d *Detector) computeAllLicenseInfo() {
	d.allLicenses = []*License{}
	d.allLicenses = append(d.allLicenses, d.moduleLicenses...)
	nonRootLicenses := d.detectFiles(d.Files(NonRootFiles))
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

// Files returns a list of license files from the zip. The which argument
// determines the location of the files considered.
func (d *Detector) Files(which WhichFiles) []*zip.File {
	cdir := contentsDir(d.modulePath, d.version)
	prefix := pathPrefix(cdir)
	var files []*zip.File
	for _, f := range d.zr.File {
		if !fileNamesLowercase[strings.ToLower(path.Base(f.Name))] {
			continue
		}
		if !strings.HasPrefix(f.Name, prefix) {
			d.logf("potential license file %q found outside of the expected path %q", f.Name, cdir)
			continue
		}
		// Skip files we should ignore.
		if ignoreFiles[d.modulePath+" "+strings.TrimPrefix(f.Name, prefix)] {
			continue
		}
		if which == RootFiles && path.Dir(f.Name) != cdir {
			// Skip f since it's not at root.
			continue
		}
		if which == NonRootFiles && path.Dir(f.Name) == cdir {
			// Skip f since it is at root.
			continue
		}
		if isVendoredFile(f.Name) {
			// Skip if f is in the vendor directory.
			continue
		}
		if err := module.CheckFilePath(f.Name); err != nil {
			// Skip if the file path is bad.
			d.logf("module.CheckFilePath(%q): %v", f.Name, err)
			continue
		}
		files = append(files, f)
	}
	return files
}

// isVendoredFile reports if the given file is in a proper subdirectory nested
// under a 'vendor' directory, to allow for Go packages named 'vendor'.
//
// e.g. isVendoredFile("vendor/LICENSE") == false, and
//      isVendoredFile("vendor/foo/LICENSE") == true
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
func (d *Detector) detectFiles(files []*zip.File) []*License {
	prefix := pathPrefix(contentsDir(d.modulePath, d.version))
	var licenses []*License
	for _, f := range files {
		bytes, err := readZipFile(f)
		if err != nil {
			d.logf("reading zip file %s: %v", f.Name, err)
			licenses = append(licenses, &License{
				Metadata: &Metadata{
					Types:    []string{unknownLicenseType},
					FilePath: strings.TrimPrefix(f.Name, prefix),
				},
			})
			continue
		}
		types, cov := DetectFile(bytes, f.Name, d.logf)
		licenses = append(licenses, &License{
			Metadata: &Metadata{
				Types:    types,
				FilePath: strings.TrimPrefix(f.Name, prefix),
				Coverage: cov,
			},
			Contents: bytes,
		})
	}
	return licenses
}

// DetectFile return the set of license types for the given file contents. It
// also returns the licensecheck coverage information. The filename is used
// solely for logging.
func DetectFile(contents []byte, filename string, logf func(string, ...interface{})) ([]string, licensecheck.Coverage) {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	if types := exceptionFileTypes(contents); types != nil {
		logf("%s is an exception", filename)
		return types, licensecheck.Coverage{}
	}
	cov, ok := checker.Cover(contents, licensecheck.Options{})
	if !ok {
		logf("%s checker.Cover failed, skipping", filename)
		return []string{unknownLicenseType}, licensecheck.Coverage{}
	}
	if cov.Percent < float64(coverageThreshold) {
		logf("%s license coverage too low (%+v), skipping", filename, cov)
		return []string{unknownLicenseType}, cov
	}
	types := make(map[string]bool)
	for _, m := range cov.Match {
		if m.Percent >= classifyThreshold {
			types[canonicalizeName(m.Name)] = true
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
func Redistributable(licenseTypes []string) bool {
	if len(licenseTypes) == 0 {
		return false
	}
	for _, t := range licenseTypes {
		if !redistributableLicenseTypes[t] && !ignorableLicenseTypes[t] {
			return false
		}
	}
	return true
}

var canonicalNames = map[string]string{
	"AGPL-Header":         "AGPL-3.0",
	"GPL-Header":          "GPL2",
	"GPL-NotLater-Header": "GPL3",
	"LGPL-Header":         "LGPL-2.1",
}

// canonicalizeName puts a license name in a standard form.
func canonicalizeName(name string) string {
	if c := canonicalNames[name]; c != "" {
		return c
	}
	name = strings.TrimSuffix(name, "-Short")
	return strings.TrimSuffix(name, "-Header")
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

func readZipFile(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxLicenseSize {
		return nil, fmt.Errorf("file size %d exceeds max license size %d", f.UncompressedSize64, maxLicenseSize)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return ioutil.ReadAll(io.LimitReader(rc, int64(maxLicenseSize)))
}

func contentsDir(modulePath, version string) string {
	return modulePath + "@" + version
}

// pathPrefix appends a "/" to its argument if the argument is non-empty.
func pathPrefix(s string) string {
	if s != "" {
		return s + "/"
	}
	return ""
}
