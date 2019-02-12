// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"io/ioutil"
	"testing"
)

func TestIsReadme(t *testing.T) {
	for input, expected := range map[string]bool{
		"rEaDme":         true,
		"README.FOO":     true,
		"FOO_README":     false,
		"README_FOO":     false,
		"README.FOO.FOO": false,
	} {
		got := isReadme(input)
		if got != expected {
			t.Errorf("isReadme(%q) = %t: %t", input, got, expected)
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

	readme, err := readReadme(zipReader)
	if err != nil {
		t.Errorf("readReadme(%q) error: %v", testZip, err)
	}

	testReadmeFilename := "testdata/module/README.md"
	expectedReadme, err := ioutil.ReadFile(testReadmeFilename)
	if err != nil {
		t.Errorf("readReadme(%q) error: %v", testReadmeFilename, err)
	}

	if !bytes.Equal(expectedReadme, readme) {
		t.Errorf("readReadme(%q) = %q, want %q", testZip, readme, expectedReadme)
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
