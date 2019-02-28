// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
)

// structToString returns a string containing the fields and values of a given
// struct i. It is used in error messages for tests.
func structToString(i interface{}) string {
	s := reflect.ValueOf(i).Elem()
	typeOfT := s.Type()

	var b strings.Builder
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		fmt.Fprintf(&b, fmt.Sprintf("%d: %s %s = %v \n", i, typeOfT.Field(i).Name, f.Type(), f.Interface()))
	}
	return b.String()
}

// zipFiles compresses the files inside dir files into a single zip archive
// file. filename is the output zip file's name.
func zipFiles(filename string, dir string) (func(), error) {
	cleanup := func() { os.Remove(filename) }

	newZipFile, err := os.Create(filename)
	if err != nil {
		return cleanup, fmt.Errorf("os.Create(%q): %v", filename, err)
	}
	defer newZipFile.Close()

	zipWriter := zip.NewWriter(newZipFile)
	defer zipWriter.Close()

	return cleanup, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fileToZip, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("os.Open(%q): %v", path, err)
		}
		defer fileToZip.Close()

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("zipFileInfoHeader(%v): %v", info.Name(), err)
		}

		// Using FileInfoHeader() above only uses the basename of the file. If we want
		// to preserve the folder structure we can overwrite this with the full path.
		header.Name = strings.TrimPrefix(strings.TrimPrefix(path, dir), "/")

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("zipWriter.CreateHeader(%+v): %v", header, err)
		}

		if _, err = io.Copy(writer, fileToZip); err != nil {
			return fmt.Errorf("io.Copy(%v, %+v): %v", writer, fileToZip, err)
		}
		return nil
	})
}

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
					Name: "my/module",
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

			teardownProxyClient, client := proxy.SetupTestProxyClient(t)
			defer teardownProxyClient(t)

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

			if !reflect.DeepEqual(*got, *test.versionData) {
				t.Errorf("db.GetVersion(%q, %q): \n %s \n want: \n %s",
					test.name, test.version, structToString(got), structToString(test.versionData))
			}
		})
	}
}

func TestParseNameAndVersion(t *testing.T) {
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

			m, v, err := ParseNameAndVersion(u)
			if test.err != nil {
				if err == nil {
					t.Fatalf("ParseNameAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				}
				if test.err.Error() != err.Error() {
					t.Fatalf("ParseNameAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				} else {
					return
				}
			} else if err != nil {
				t.Fatalf("ParseNameAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
			}

			if test.module != m || test.version != v {
				t.Fatalf("ParseNameAndVersion(%v): %q, %q, %v; want = %q, %q, %v",
					u, m, v, err, test.module, test.version, test.err)
			}
		})
	}
}

func TestHasFilename(t *testing.T) {
	for _, test := range []struct {
		name         string
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

func TestSeriesNameForModule(t *testing.T) {
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
			got, err := seriesNameForModule(input)
			if err != nil {
				t.Errorf("seriesNameForModule(%q): %v", input, err)
			}
			if got != want {
				t.Errorf("seriesNameForModule(%q) = %q, want %q", input, got, want)
			}
		})
	}

	wantErr := "module name cannot be empty"
	if _, err := seriesNameForModule(""); err == nil || err.Error() != wantErr {
		t.Errorf("seriesNameForModule(%q) returned error: %v; want %v", "", err, wantErr)
	}
}

func TestContainsFile(t *testing.T) {
	for _, test := range []struct {
		zip  string
		File string
		Want bool
	}{
		{
			zip:  "module.zip",
			File: "README",
			Want: true,
		},
		{
			zip:  "module.zip",
			File: "LICENSE",
			Want: false,
		},
		{
			zip:  "empty.zip",
			File: "README",
			Want: false,
		},
		{
			zip:  "empty.zip",
			File: "LICENSE",
			Want: false,
		},
	} {
		t.Run(test.zip, func(t *testing.T) {
			name := filepath.Join("testdata/modules", test.zip)
			rc, err := zip.OpenReader(name)
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", name, err)
			}
			defer rc.Close()
			z := &rc.Reader

			if got := containsFile(z, test.File); got != test.Want {
				t.Errorf("containsFile(%q, %q) = %t, want %t", name, test.File, got, test.Want)
			}
		})
	}
}

func TestExtractFile(t *testing.T) {
	for _, test := range []struct {
		zip  string
		file string
		err  error
	}{
		{
			zip:  "module.zip",
			file: "my/module@v1.0.0/README.md",
			err:  nil,
		},
		{
			zip:  "empty.zip",
			file: "empty/nonexistent/README.md",
			err:  errors.New(`zip does not contain "README.md"`),
		},
	} {
		t.Run(test.zip, func(t *testing.T) {
			rc, err := zip.OpenReader(filepath.Join("testdata/modules", test.zip))
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", test.zip, err)
			}
			defer rc.Close()
			z := &rc.Reader

			got, err := extractFile(z, filepath.Base(test.file))
			if err != nil {
				if test.err == nil || test.err.Error() != err.Error() {
					t.Errorf("extractFile(%q, %q): \n %v, want \n %v",
						test.zip, filepath.Base(test.file), err, test.err)
				} else {
					return
				}
			}

			f := filepath.Join("testdata/modules", test.file)
			want, err := ioutil.ReadFile(f)
			if err != nil {
				t.Fatalf("ioutfil.ReadFile(%q) error: %v", f, err)
			}

			if !bytes.Equal(want, got) {
				t.Errorf("extractFile(%q, %q) = %q, want %q", test.zip, test.file, got, want)
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
		err      error
	}{
		{
			zip:     "module.zip",
			name:    "my/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"foo": &internal.Package{
					Name: "foo",
					Path: "my/module/foo",
				},
				"bar": &internal.Package{
					Name: "bar",
					Path: "my/module/bar",
				},
			},
		},
		{
			zip:      "empty.zip",
			name:     "empty/module",
			version:  "v1.0.0",
			packages: map[string]*internal.Package{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rc, err := zip.OpenReader(filepath.Join("testdata/modules", test.zip))
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", test.zip, err)
			}
			defer rc.Close()
			z := &rc.Reader

			packages, err := extractPackagesFromZip(test.name, test.version, z)
			if err != nil && len(test.packages) != 0 {
				t.Fatalf("zipToPackages(%q, %q): %v", test.name, test.zip, err)
			}

			for _, got := range packages {
				want, ok := test.packages[got.Name]
				if !ok {
					t.Errorf("zipToPackages(%q, %q) returned unexpected package: %q", test.name, test.zip, got.Name)
				}
				if want.Path != got.Path {
					t.Errorf("zipToPackages(%q, %q) returned unexpected path for package %q: %q, want %q",
						test.name, test.zip, got.Name, got.Path, want.Path)
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
			cleanUpZip, err := zipFiles(testDir+".zip", testDir)
			defer cleanUpZip()
			if err != nil {
				t.Fatalf("zipFiles(%q): %v", test.zipName, err)
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
