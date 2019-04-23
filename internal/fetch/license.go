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
func detectLicenses(r *zip.Reader) ([]*internal.License, error) {
	var licenses []*internal.License
	for _, f := range r.File {
		if !licenseFileNames[filepath.Base(f.Name)] || strings.Contains(f.Name, "/vendor/") {
			// Only consider licenses with an acceptable file name, and not in the
			// vendor directory.
			continue
		}
		if err := module.CheckFilePath(f.Name); err != nil {
			return nil, fmt.Errorf("module.CheckFilePath(%q): %v", f.Name, err)
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
					FilePath: f.Name,
				},
				Contents: bytes,
			}
			licenses = append(licenses, license)
		}
	}
	return licenses, nil
}
