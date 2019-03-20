// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
)

func TestFetchAndInsertVersion(t *testing.T) {
	testCases := []struct {
		name        string
		version     string
		versionData *internal.Version
	}{
		{
			name:    "my/module",
			version: "v1.0.0",
			versionData: &internal.Version{
				Module: &internal.Module{
					Path: "my/module",
				},
				Version:    "v1.0.0",
				CommitTime: time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
				ReadMe:     "README FILE FOR TESTING.",
				License:    "BSD-3-Clause",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			teardownDB, db := postgres.SetupCleanDB(t)
			defer teardownDB(t)

			teardownProxy, client := proxy.SetupTestProxy(t)
			defer teardownProxy(t)

			if err := FetchAndInsertVersion(test.name, test.version, client, db); err != nil {
				t.Fatalf("FetchVersion(%q, %q, %v, %v): %v", test.name, test.version, client, db, err)
			}

			got, err := db.GetVersion(test.name, test.version)
			if err != nil {
				t.Fatalf("db.GetVersion(%q, %q): %v", test.name, test.version, err)
			}

			// Set CreatedAt and UpdatedAt to nil for testing, since these are
			// set by the database.
			got.CreatedAt = time.Time{}
			got.UpdatedAt = time.Time{}

			// got.CommitTime has a timezone location of +0000, while
			// test.versionData.CommitTime has a timezone location of UTC.
			// These are equal according to time.Equal, but fail for
			// reflect.DeepEqual. Convert the DB time to UTC.
			got.CommitTime = got.CommitTime.UTC()

			if diff := cmp.Diff(*got, *test.versionData); diff != "" {
				t.Errorf("db.GetVersion(%q, %q) mismatch(-want +got):\n%s", test.name, test.version, diff)
			}
		})
	}
}

func TestParseModulePathAndVersion(t *testing.T) {
	testCases := []struct {
		name    string
		url     string
		module  string
		version string
		err     error
	}{
		{
			name:    "ValidFetchURL",
			url:     "https://proxy.com/module/@v/v1.0.0",
			module:  "module",
			version: "v1.0.0",
			err:     nil,
		},
		{
			name: "InvalidFetchURL",
			url:  "https://proxy.com/",
			err:  errors.New(`invalid path: "https://proxy.com/"`),
		},
		{
			name: "InvalidFetchURLNoModule",
			url:  "https://proxy.com/@v/version",
			err:  errors.New(`invalid path: "https://proxy.com/@v/version"`),
		},
		{
			name: "InvalidFetchURLNoVersion",
			url:  "https://proxy.com/module/@v/",
			err:  errors.New(`invalid path: "https://proxy.com/module/@v/"`),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse(test.url)
			if err != nil {
				t.Errorf("url.Parse(%q): %v", test.url, err)
			}

			m, v, err := ParseModulePathAndVersion(u)
			if test.err != nil {
				if err == nil {
					t.Fatalf("ParseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				}
				if test.err.Error() != err.Error() {
					t.Fatalf("ParseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				} else {
					return
				}
			} else if err != nil {
				t.Fatalf("ParseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
			}

			if test.module != m || test.version != v {
				t.Fatalf("ParseModulePathAndVersion(%v): %q, %q, %v; want = %q, %q, %v",
					u, m, v, err, test.module, test.version, test.err)
			}
		})
	}
}

func TestHasFilename(t *testing.T) {
	for _, test := range []struct {
		file         string
		expectedFile string
		want         bool
	}{
		{
			file:         "my/module@v1.0.0/README.md",
			expectedFile: "README.md",
			want:         true,
		},
		{
			file:         "rEaDme",
			expectedFile: "README",
			want:         true,
		}, {
			file:         "README.FOO",
			expectedFile: "README",
			want:         true,
		},
		{
			file:         "FOO_README",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "README_FOO",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "README.FOO.FOO",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "my/module@v1.0.0/LICENSE",
			expectedFile: "my/module@v1.0.0/LICENSE",
			want:         true,
		},
	} {
		{
			t.Run(test.file, func(t *testing.T) {
				got := hasFilename(test.file, test.expectedFile)
				if got != test.want {
					t.Errorf("hasFilename(%q, %q) = %t: %t", test.file, test.expectedFile, got, test.want)
				}
			})
		}
	}

}

func TestSeriesPathForModule(t *testing.T) {
	for input, want := range map[string]string{
		"module/":          "module",
		"module/v2/":       "module",
		"my/module":        "my/module",
		"my/module/v":      "my/module/v",
		"my/module/v0":     "my/module/v0",
		"my/module/v1":     "my/module/v1",
		"my/module/v23456": "my/module",
		"v2/":              "v2",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := seriesPathForModule(input)
			if err != nil {
				t.Errorf("seriesPathForModule(%q): %v", input, err)
			}
			if got != want {
				t.Errorf("seriesPathForModule(%q) = %q, want %q", input, got, want)
			}
		})
	}

	wantErr := "module name cannot be empty"
	if _, err := seriesPathForModule(""); err == nil || err.Error() != wantErr {
		t.Errorf("seriesPathForModule(%q) returned error: %v; want %v", "", err, wantErr)
	}
}

func TestContainsFile(t *testing.T) {
	for _, test := range []struct {
		name, version, file string
		want                bool
	}{
		{
			name:    "my/module",
			version: "v1.0.0",
			file:    "README",
			want:    true,
		},
		{
			name:    "my/module",
			version: "v1.0.0",
			file:    "my/module@v1.0.0/LICENSE",
			want:    true,
		},
		{
			name:    "empty/module",
			version: "v1.0.0",
			file:    "README",
			want:    false,
		},
		{
			name:    "empty/module",
			version: "v1.0.0",
			file:    "LICENSE",
			want:    false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(t)
			defer teardownProxy(t)

			reader, err := client.GetZip(test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(%q %q): %v", test.name, test.version, err)
			}

			if got := containsFile(reader, test.file); got != test.want {
				t.Errorf("containsFile(%q, %q) = %t, want %t", fmt.Sprintf("%s %s", test.name, test.version), test.file, got, test.want)
			}
		})
	}
}

func TestExtractFile(t *testing.T) {
	for _, test := range []struct {
		name, version, file string
		want                []byte
		err                 error
	}{
		{
			name:    "my/module",
			version: "v1.0.0",
			file:    "my/module@v1.0.0/README.md",
			want:    []byte("README FILE FOR TESTING."),
		},
		{
			name:    "empty/module",
			version: "v1.0.0",
			file:    "empty/nonexistent/README.md",
			err:     errors.New(`zip does not contain "README.md"`),
		},
	} {
		t.Run(test.file, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(t)
			defer teardownProxy(t)

			reader, err := client.GetZip(test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(%q, %q): %v", test.name, test.version, err)
			}

			got, err := extractFile(reader, filepath.Base(test.file))
			if err != nil {
				if test.err == nil || test.err.Error() != err.Error() {
					t.Errorf("extractFile(%q, %q): \n %v, want \n %v",
						fmt.Sprintf("%q %q", test.name, test.version), filepath.Base(test.file), err, test.err)
				} else {
					return
				}
			}

			if !bytes.Equal(test.want, got) {
				t.Errorf("extractFile(%q, %q) = %q, want %q", test.name, test.file, got, test.want)
			}
		})
	}
}

func TestExtractPackagesFromZip(t *testing.T) {
	for _, test := range []struct {
		zip      string
		name     string
		version  string
		packages map[string]*internal.Package
	}{
		{
			zip:     "module.zip",
			name:    "my/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"foo": &internal.Package{
					Name:     "foo",
					Path:     "my/module/foo",
					Synopsis: "package foo",
				},
				"bar": &internal.Package{
					Name:     "bar",
					Path:     "my/module/bar",
					Synopsis: "package bar",
				},
			},
		},
		{
			name:     "empty/module",
			version:  "v1.0.0",
			packages: map[string]*internal.Package{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(t)
			defer teardownProxy(t)

			reader, err := client.GetZip(test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(%q %q): %v", test.name, test.version, err)
			}

			packages, err := extractPackagesFromZip(test.name, test.version, reader)
			if err != nil && len(test.packages) != 0 {
				t.Fatalf("extractPackagesFromZip(%q, %q): %v", test.name, test.zip, err)
			}

			for _, got := range packages {
				want, ok := test.packages[got.Name]
				if !ok {
					t.Errorf("extractPackagesFromZip(%q, %q) returned unexpected package: %q", test.name, test.zip, got.Name)
				}
				if want.Path != got.Path {
					t.Errorf("extractPackagesFromZip(%q, %q) returned unexpected path for package %q: %q, want %q",
						test.name, test.zip, got.Name, got.Path, want.Path)
				}
				if want.Synopsis != got.Synopsis {
					t.Errorf("extractPackagesFromZip(%q, %q) returned unexpected synopsis for package %q: %q, want %q",
						test.name, test.zip, got.Name, got.Synopsis, want.Synopsis)
				}

				delete(test.packages, got.Name)
			}
		})
	}
}

func TestDetectLicense(t *testing.T) {
	testCases := []struct {
		name, zipName, contentsDir, want string
	}{
		{
			name:        "valid_license",
			zipName:     "license",
			contentsDir: "rsc.io/quote@v1.4.1",
			want:        "MIT",
		}, {
			name:        "valid_license_md_format",
			zipName:     "licensemd",
			contentsDir: "rsc.io/quote@v1.4.1",
			want:        "MIT",
		},
		{
			name:        "valid_license_copying",
			zipName:     "copying",
			contentsDir: "golang.org/x/text@v0.0.3",
			want:        "Apache-2.0",
		},
		{
			name:        "valid_license_copying_md",
			zipName:     "copyingmd",
			contentsDir: "golang.org/x/text@v0.0.3",
			want:        "Apache-2.0",
		}, {
			name:        "low_coverage_license",
			zipName:     "lowcoveragelicenses",
			contentsDir: "rsc.io/quote@v1.4.1",
		}, {
			name:        "no_license",
			zipName:     "nolicense",
			contentsDir: "rsc.io/quote@v1.5.2",
		}, {
			name:        "vendor_license_should_ignore",
			zipName:     "vendorlicense",
			contentsDir: "rsc.io/quote@v1.5.2",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			testDir := filepath.Join("testdata/licenses", test.zipName)
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

			if got := detectLicense(z, test.contentsDir); got != test.want {
				t.Errorf("detectLicense(%q) = %q, want %q", test.name, got, test.want)
			}
		})
	}
}
