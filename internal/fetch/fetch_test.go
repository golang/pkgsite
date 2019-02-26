// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/ioutil"
	"net/url"
	"strings"
	"testing"
)

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

func TestIsReadme(t *testing.T) {
	for input, want := range map[string]bool{
		"rEaDme":         true,
		"README.FOO":     true,
		"FOO_README":     false,
		"README_FOO":     false,
		"README.FOO.FOO": false,
	} {
		got := isReadme(input)
		if got != want {
			t.Errorf("isReadme(%q) = %t: %t", input, got, want)
		}
	}

	got := isReadme("")
	if got != false {
		t.Errorf("isReadme(%q) = %t: %t", "", got, false)
	}

}

func TestContainsReadme(t *testing.T) {
	testZip := "testdata/module.zip"
	zipReader, err := zip.OpenReader(testZip)
	if err != nil {
		t.Fatalf("zip.OpenReader(%q) error: %v", testZip, err)
	}
	defer zipReader.Close()

	if !containsReadme(zipReader) {
		t.Errorf("containsReadme(%q) = false, want true", testZip)
	}
}

func TestContainsReadmeEmptyZip(t *testing.T) {
	testZip := "testdata/empty.zip"
	zipReader, err := zip.OpenReader(testZip)
	if err != nil {
		t.Fatalf("zip.OpenReader(%q) error: %v", testZip, err)
	}
	defer zipReader.Close()

	if containsReadme(zipReader) {
		t.Errorf("containsReadme(%q) = true, want false", testZip)
	}
}

func TestReadZip(t *testing.T) {
	testZip := "testdata/module.zip"
	zipReader, err := zip.OpenReader(testZip)
	if err != nil {
		t.Fatalf("zip.OpenReader(%q) error: %v", testZip, err)
	}
	defer zipReader.Close()

	got, err := readReadme(zipReader)
	if err != nil {
		t.Errorf("readReadme(%q) error: %v", testZip, err)
	}

	testReadmeFilename := "testdata/my/module/README.md"
	want, err := ioutil.ReadFile(testReadmeFilename)
	if err != nil {
		t.Errorf("readReadme(%q) error: %v", testReadmeFilename, err)
	}

	if !bytes.Equal(want, got) {
		t.Errorf("readReadme(%q) = %q, want %q", testZip, got, want)
	}
}

func TestReadZipEmptyZip(t *testing.T) {
	testZip := "testdata/empty.zip"
	zipReader, err := zip.OpenReader(testZip)
	if err != nil {
		t.Fatalf("zip.OpenReader(%q) error: %v", testZip, err)
	}
	defer zipReader.Close()

	_, err = readReadme(zipReader)
	if err != errReadmeNotFound {
		t.Errorf("readReadme(%q) error: %v, want %v", testZip, err, errReadmeNotFound)
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
		got, err := seriesNameForModule(input)
		if err != nil {
			t.Errorf("seriesNameForModule(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("seriesNameForModule(%q) = %q, want %q", input, got, want)
		}
	}

	want := errors.New("module name cannot be empty")
	if _, got := seriesNameForModule(""); got.Error() != want.Error() {
		t.Errorf("seriesNameForModule(%q) returned error: %v; want %v", "", got, want)
	}
}

func TestExtractPackagesFromZip(t *testing.T) {
	testZip := "testdata/module.zip"
	zipReader, err := zip.OpenReader(testZip)
	if err != nil {
		t.Fatalf("zip.OpenReader(%q): %v", testZip, err)
	}

	name := "my/module"
	packages, err := extractPackagesFromZip(name, zipReader)
	if err != nil {
		t.Fatalf("zipToPackages(%q, %q): %v", name, testZip, err)
	}

	expectedNamesToPath := map[string]string{
		"foo": "my/module/foo",
		"bar": "my/module/bar",
	}
	for _, p := range packages {
		expectedPath, ok := expectedNamesToPath[p.Name]
		if !ok {
			t.Errorf("zipToPackages(%q, %q) returned unexpected package: %q", name, testZip, p.Name)
		}
		if expectedPath != p.Path {
			t.Errorf("zipToPackages(%q, %q) returned unexpected path for package %q: %q, want %q", name, testZip, p.Name, p.Path, expectedPath)
		}

		delete(expectedNamesToPath, p.Name)
	}
}

func TestExtractPackagesFromZipEmptyZip(t *testing.T) {
	testZip := "testdata/empty.zip"
	zipReader, err := zip.OpenReader(testZip)
	if err != nil {
		t.Fatalf("zip.OpenReader(%q): %v", testZip, err)
	}

	name := "empty/module"
	_, err = extractPackagesFromZip(name, zipReader)
	if !strings.HasSuffix(err.Error(), "returned 0 packages") {
		t.Fatalf("zipToPackages(%q, %q): %v", name, testZip, err)
	}
}
