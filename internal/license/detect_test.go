// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal/proxy"
)

func TestDetect(t *testing.T) {
	makeLicenses := func(licType, licFile string) []*Metadata {
		return []*Metadata{{Type: licType, FilePath: licFile}}
	}
	testCases := []struct {
		name, zipName, subdir string
		want                  []*Metadata
	}{
		{
			name:    "valid_license",
			zipName: "license",
			want:    makeLicenses("MIT", "rsc.io/quote@v1.4.1/LICENSE"),
		}, {
			name:    "valid_license_md_format",
			zipName: "licensemd",
			want:    makeLicenses("MIT", "rsc.io/quote@v1.4.1/LICENSE.md"),
		}, {
			name:    "valid_license_trim_prefix",
			subdir:  "rsc.io/quote@v1.4.1",
			zipName: "licensemd",
			want:    makeLicenses("MIT", "LICENSE.md"),
		}, {
			name:    "valid_license_copying",
			zipName: "copying",
			want:    makeLicenses("Apache-2.0", "golang.org/x/text@v0.0.3/COPYING"),
		}, {
			name:    "valid_license_copying_md",
			zipName: "copyingmd",
			want:    makeLicenses("Apache-2.0", "golang.org/x/text@v0.0.3/COPYING.md"),
		}, {
			name:    "multiple_licenses",
			zipName: "multiplelicenses",
			want: []*Metadata{
				{Type: "MIT", FilePath: "rsc.io/quote@v1.4.1/LICENSE"},
				{Type: "MIT", FilePath: "rsc.io/quote@v1.4.1/bar/LICENSE.md"},
				{Type: "Apache-2.0", FilePath: "rsc.io/quote@v1.4.1/foo/COPYING"},
				{Type: "Apache-2.0", FilePath: "rsc.io/quote@v1.4.1/foo/COPYING.md"},
			},
		}, {
			name:    "low_coverage_license",
			zipName: "lowcoveragelicenses",
		}, {
			name:    "no_license",
			zipName: "nolicense",
		}, {
			name:    "no_license",
			zipName: "nolicense",
		}, {
			name:    "ignores licenses in vendored packages, but not packages named vendor",
			subdir:  "rsc.io/quote@v1.5.2",
			zipName: "vendorlicense",
			want: []*Metadata{
				{Type: "MIT", FilePath: "pkg/vendor/LICENSE"},
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			testDir := filepath.Join("../fetch/testdata/licenses", test.zipName)
			cleanUpZip, err := proxy.ZipFiles(testDir+".zip", testDir, "")
			defer cleanUpZip()
			if err != nil {
				t.Fatalf("proxy.ZipFiles(%q): %v", test.zipName, err)
			}

			if _, err := os.Stat(testDir + ".zip"); err != nil {
				t.Fatalf("os.Stat(%q): %v", testDir+".zip", err)
			}

			rc, err := zip.OpenReader(testDir + ".zip")
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", test.zipName, err)
			}
			defer rc.Close()
			z := &rc.Reader

			got, err := Detect(test.subdir, z)
			if err != nil {
				t.Errorf("detectLicenses(z): %v", err)
			}
			var gotFiles []*Metadata
			for _, l := range got {
				gotFiles = append(gotFiles, &l.Metadata)
			}
			if diff := cmp.Diff(test.want, gotFiles); diff != "" {
				t.Errorf("detectLicense(z) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
