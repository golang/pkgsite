// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
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
				License:    "LICENSE FILE FOR TESTING.",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownDB, db := postgres.SetupCleanDB(t)
			defer teardownDB(t)

			teardownProxyClient, client := proxy.SetupTestProxyClient(t)
			defer teardownProxyClient(t)

			if err := FetchAndInsertVersion(tc.name, tc.version, client, db); err != nil {
				t.Fatalf("FetchVersion(%q, %q, %v, %v): %v", tc.name, tc.version, client, db, err)
			}

			got, err := db.GetVersion(tc.name, tc.version)
			if err != nil {
				t.Fatalf("db.GetVersion(%q, %q): %v", tc.name, tc.version, err)
			}

			// Set CreatedAt and UpdatedAt to nil for testing, since these are
			// set by the database.
			got.CreatedAt = time.Time{}
			got.UpdatedAt = time.Time{}

			// got.CommitTime has a timezone location of +0000, while
			// tc.versionData.CommitTime has a timezone location of UTC.
			// These are equal according to time.Equal, but fail for
			// reflect.DeepEqual. Convert the DB time to UTC.
			got.CommitTime = got.CommitTime.UTC()

			if !reflect.DeepEqual(*got, *tc.versionData) {
				t.Errorf("db.GetVersion(%q, %q): \n %s \n want: \n %s",
					tc.name, tc.version, structToString(got), structToString(tc.versionData))
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err != nil {
				t.Errorf("url.Parse(%q): %v", tc.url, err)
			}

			m, v, err := ParseNameAndVersion(u)
			if tc.err != nil {
				if err == nil {
					t.Fatalf("ParseNameAndVersion(%v) error = (%v); want = (%v)", u, err, tc.err)
				}
				if tc.err.Error() != err.Error() {
					t.Fatalf("ParseNameAndVersion(%v) error = (%v); want = (%v)", u, err, tc.err)
				} else {
					return
				}
			} else if err != nil {
				t.Fatalf("ParseNameAndVersion(%v) error = (%v); want = (%v)", u, err, tc.err)
			}

			if tc.module != m || tc.version != v {
				t.Fatalf("ParseNameAndVersion(%v): %q, %q, %v; want = %q, %q, %v",
					u, m, v, err, tc.module, tc.version, tc.err)
			}
		})
	}
}

func TestHasFilename(t *testing.T) {
	for _, tc := range []struct {
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
			t.Run(tc.file, func(t *testing.T) {
				got := hasFilename(tc.file, tc.expectedFile)
				if got != tc.want {
					t.Errorf("hasFilename(%q, %q) = %t: %t", tc.file, tc.expectedFile, got, tc.want)
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
	for _, tc := range []struct {
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
		t.Run(tc.zip, func(t *testing.T) {
			name := filepath.Join("testdata", tc.zip)
			rc, err := zip.OpenReader(name)
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", name, err)
			}
			defer rc.Close()
			z := &rc.Reader

			if got := containsFile(z, tc.File); got != tc.Want {
				t.Errorf("containsFile(%q, %q) = %t, want %t", name, tc.File, got, tc.Want)
			}
		})
	}
}

func TestExtractFile(t *testing.T) {
	for _, tc := range []struct {
		zip  string
		file string
		err  error
	}{
		{
			zip:  "testdata/module.zip",
			file: "my/module@v1.0.0/README.md",
			err:  nil,
		},
		{
			zip:  "testdata/empty.zip",
			file: "empty/nonexistent/README.md",
			err:  errors.New(`zip does not contain "README.md"`),
		},
	} {
		t.Run(tc.zip, func(t *testing.T) {
			rc, err := zip.OpenReader(tc.zip)
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", tc.zip, err)
			}
			defer rc.Close()
			z := &rc.Reader

			got, err := extractFile(z, filepath.Base(tc.file))
			if err != nil {
				if tc.err == nil || tc.err.Error() != err.Error() {
					t.Errorf("extractFile(%q, %q): \n %v, want \n %v",
						tc.zip, filepath.Base(tc.file), err, tc.err)
				} else {
					return
				}
			}

			f := filepath.Join("testdata", tc.file)
			want, err := ioutil.ReadFile(f)
			if err != nil {
				t.Fatalf("ioutfil.ReadFile(%q) error: %v", f, err)
			}

			if !bytes.Equal(want, got) {
				t.Errorf("extractFile(%q, %q) = %q, want %q", tc.zip, tc.file, got, want)
			}
		})
	}
}

func TestExtractPackagesFromZip(t *testing.T) {
	for _, tc := range []struct {
		zip      string
		name     string
		version  string
		packages map[string]*internal.Package
		err      error
	}{
		{
			zip:     "testdata/module.zip",
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
			zip:      "testdata/empty.zip",
			name:     "empty/module",
			version:  "v1.0.0",
			packages: map[string]*internal.Package{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc, err := zip.OpenReader(tc.zip)
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", tc.zip, err)
			}
			defer rc.Close()
			z := &rc.Reader

			packages, err := extractPackagesFromZip(tc.name, tc.version, z)
			if err != nil && len(tc.packages) != 0 {
				t.Fatalf("zipToPackages(%q, %q): %v", tc.name, tc.zip, err)
			}

			for _, got := range packages {
				want, ok := tc.packages[got.Name]
				if !ok {
					t.Errorf("zipToPackages(%q, %q) returned unexpected package: %q", tc.name, tc.zip, got.Name)
				}
				if want.Path != got.Path {
					t.Errorf("zipToPackages(%q, %q) returned unexpected path for package %q: %q, want %q",
						tc.name, tc.zip, got.Name, got.Path, want.Path)
				}

				delete(tc.packages, got.Name)
			}
		})
	}
}
