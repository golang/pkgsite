// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/ioutil"
	"testing"
)

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

	testReadmeFilename := "testdata/module/README.md"
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
