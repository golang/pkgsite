// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"path"
	"sort"
	"strings"

	"github.com/google/licensecheck"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/thirdparty/module"
)

const (
	// classifyThreshold is the minimum confidence percentage/threshold
	// to classify a license
	classifyThreshold = 90

	// coverageThreshold is the minimum percentage of the file that must contain license text.
	coverageThreshold = 90

	// maxLicenseSize is the maximum allowable size (in bytes) for a license
	// file.
	maxLicenseSize = 1e7
)


// extraction.
var licenseFileNames = map[string]bool{
	"LICENSE":     true,
	"LICENSE.md":  true,
	"LICENSE.txt": true,
	"LICENCE":     true,
	"LICENCE.md":  true,
	"LICENCE.txt": true,
	"COPYING":     true,
	"COPYING.md":  true,
	"COPYING.txt": true,
}

// FileNames returns the slice of file names to be considered for license
// detection.
func FileNames() []string {
	var names []string
	for f := range licenseFileNames {
		names = append(names, f)
	}
	sort.Strings(names)
	return names
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

// Files returns zip files that are considered to be potential license
// candidates. It returns an error if any potential license files are invalid.
func Files(contentsDir string, r *zip.Reader) (_ []*zip.File, err error) {
	defer derrors.Add(&err, "license.Files(%q)", contentsDir)
	prefix := pathPrefix(contentsDir)

	var files []*zip.File
	for _, f := range r.File {
		if !licenseFileNames[path.Base(f.Name)] || isVendoredFile(f.Name) {
			// Only consider licenses with an acceptable file name, and not in the
			// vendor directory.
			continue
		}
		if err := module.CheckFilePath(f.Name); err != nil {
			return nil, fmt.Errorf("module.CheckFilePath(%q): %v", f.Name, err)
		}
		if !strings.HasPrefix(f.Name, prefix) {
			return nil, fmt.Errorf("potential license file %q found outside of the expected path %s", f.Name, contentsDir)
		}
		if f.UncompressedSize64 > maxLicenseSize {
			return nil, fmt.Errorf("potential license file %q exceeds maximum uncompressed size %d", f.Name, int(1e7))
		}
		files = append(files, f)
	}
	return files, nil
}

// Detect searches for possible license files in a subdirectory within the
// provided zip path, runs them against a license classifier, and provides all
// licenses with a confidence score that meets a confidence threshold.
//
// It returns an error if the given file path is invalid, if the uncompressed
// size of the license file is too large, if a license is discovered outside of
// the expected path, or if an error occurs during extraction.
func Detect(contentsDir string, r *zip.Reader) (_ []*License, err error) {
	defer derrors.Add(&err, "license.Detect(%q)", contentsDir)
	files, err := Files(contentsDir, r)
	if err != nil {
		return nil, err
	}
	prefix := pathPrefix(contentsDir)
	var licenses []*License
	for _, f := range files {
		lic, err := detectFile(f, prefix)
		if err != nil {
			return nil, err
		}
		licenses = append(licenses, lic)
	}
	return licenses, nil
}

func detectFile(f *zip.File, prefix string) (_ *License, err error) {
	defer derrors.Wrap(&err, "license.detectFile(%q, %q)", f.Name, prefix)

	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("f.Open(): %v", err)
	}
	defer rc.Close()

	contents, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll: %v", err)
	}

	// At this point we have a valid license candidate, and so expect a match.
	// If we don't find one, we still return all information about the license,
	// but with an empty list of types.
	filePath := strings.TrimPrefix(f.Name, prefix)
	var types []string
	cov, ok := licensecheck.Cover(contents, licensecheck.Options{})
	if ok && cov.Percent >= coverageThreshold {
		matchedTypes := make(map[string]bool)
		for _, m := range cov.Match {
			if m.Percent >= classifyThreshold {
				matchedTypes[m.Name] = true
			}
		}
		for t := range matchedTypes {
			types = append(types, t)
		}
		sort.Strings(types)
	}
	return &License{
		Metadata: &Metadata{
			Types:    types,
			FilePath: filePath,
			Coverage: cov,
		},
		Contents: string(contents),
	}, nil
}

// pathPrefix is used to defermine whether or not a license file path is within
// the contents directory.
func pathPrefix(contentsDir string) string {
	if contentsDir != "" {
		return contentsDir + "/"
	}
	return ""
}
