package fetch

import (
	"archive/zip"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/licensecheck"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/thirdparty/module"
)

const (
	// licenseClassifyThreshold is the minimum confidence percentage/threshold
	// to classify a license
	licenseClassifyThreshold = 96 // TODO: run more tests to figure out the best percent.

	// licenseCoverageThreshold is the minimum percentage of the file that must contain license text.
	licenseCoverageThreshold = 90
)

// detectLicense searches for possible license files in a subdirectory within
// the provided zip path, runs them against a license classifier, and provides
// all licenses with a confidence score that meet the licenseClassifyThreshold.
//
// It returns an error if the given file path is invalid, if the uncompressed
// size of the license file is too large, if a license is discovered outside of
// the expected path, or if an error occurs during extraction.
func detectLicenses(subdir string, r *zip.Reader) ([]*internal.License, error) {
	var licenses []*internal.License
	for _, f := range r.File {
		if !internal.LicenseFileNames[filepath.Base(f.Name)] || strings.Contains(f.Name, "/vendor/") {
			// Only consider licenses with an acceptable file name, and not in the
			// vendor directory.
			continue
		}
		if err := module.CheckFilePath(f.Name); err != nil {
			return nil, fmt.Errorf("module.CheckFilePath(%q): %v", f.Name, err)
		}
		prefix := ""
		if subdir != "" {
			prefix = subdir + "/"
		}
		if !strings.HasPrefix(f.Name, prefix) {
			return nil, fmt.Errorf("potential license file %q found outside of the expected path %s", f.Name, subdir)
		}
		if f.UncompressedSize64 > 1e7 {
			return nil, fmt.Errorf("potential license file %q exceeds maximum uncompressed size %d", f.Name, int(1e7))
		}

		bytes, err := readZipFile(f)
		if err != nil {
			return nil, fmt.Errorf("readZipFile(%s): %v", f.Name, err)
		}

		cov, ok := licensecheck.Cover(bytes, licensecheck.Options{})
		if !ok || cov.Percent < licenseCoverageThreshold {
			continue
		}

		m := cov.Match[0]
		if m.Percent > licenseClassifyThreshold {
			license := &internal.License{
				LicenseInfo: internal.LicenseInfo{
					Type:     m.Name,
					FilePath: strings.TrimPrefix(f.Name, prefix),
				},
				Contents: bytes,
			}
			licenses = append(licenses, license)
		}
	}
	return licenses, nil
}
