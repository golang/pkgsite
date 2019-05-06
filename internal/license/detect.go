// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/google/licensecheck"
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

// Detect searches for possible license files in a subdirectory within the
// provided zip path, runs them against a license classifier, and provides all
// licenses with a confidence score that meets a confidence threshold.
//
// It returns an error if the given file path is invalid, if the uncompressed
// size of the license file is too large, if a license is discovered outside of
// the expected path, or if an error occurs during extraction.
func Detect(contentsDir string, r *zip.Reader) ([]*License, error) {
	var licenses []*License
	for _, f := range r.File {
		if !fileNames[filepath.Base(f.Name)] || strings.Contains(f.Name, "/vendor/") {
			// Only consider licenses with an acceptable file name, and not in the
			// vendor directory.
			continue
		}
		if err := module.CheckFilePath(f.Name); err != nil {
			return nil, fmt.Errorf("module.CheckFilePath(%q): %v", f.Name, err)
		}
		prefix := ""
		if contentsDir != "" {
			prefix = contentsDir + "/"
		}
		if !strings.HasPrefix(f.Name, prefix) {
			return nil, fmt.Errorf("potential license file %q found outside of the expected path %s", f.Name, contentsDir)
		}
		if f.UncompressedSize64 > maxLicenseSize {
			return nil, fmt.Errorf("potential license file %q exceeds maximum uncompressed size %d", f.Name, int(1e7))
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("f.Open() for %q: %v", f.Name, err)
		}
		defer rc.Close()

		contents, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("ioutil.ReadAll(rc) for %q: %v", f.Name, err)
		}

		cov, ok := licensecheck.Cover(contents, licensecheck.Options{})
		if !ok || cov.Percent < coverageThreshold {
			continue
		}

		m := cov.Match[0]
		if m.Percent > classifyThreshold {
			license := &License{
				Metadata: Metadata{
					Type:     m.Name,
					FilePath: strings.TrimPrefix(f.Name, prefix),
				},
				Contents: contents,
			}
			licenses = append(licenses, license)
		}
	}
	return licenses, nil
}
