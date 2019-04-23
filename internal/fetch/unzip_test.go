// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// writeZip is a helper function that writes a new zip archive containing a
// single file corresponding to the given name and contents.
func writeZip(zipPath, name string, contents []byte) (outerErr error) {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("os.Create(%q): %v", zipPath, err)
	}
	zw := zip.NewWriter(zipFile)
	defer func() {
		if err := zw.Close(); err != nil {
			outerErr = fmt.Errorf("original error: %v; zw.Close() error: %v", outerErr, err)
		}
		if err := zipFile.Close(); err != nil {
			outerErr = fmt.Errorf("original error: %v; zipFile.Close() error: %v", outerErr, err)
		}
	}()
	header := &zip.FileHeader{Name: name}
	w, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("zw.CreateHeader(%+v): %v", header, err)
	}
	if _, err := w.Write(contents); err != nil {
		return fmt.Errorf("w.Write(contents): %v", err)
	}
	return nil
}

func TestWriteFileToDir(t *testing.T) {
	// In order to validate unzip behavior, decrease maxFileSize temporarily for
	// this test.
	defer func(mfs uint64) {
		maxFileSize = mfs
	}(maxFileSize)
	maxFileSize = 16

	// Create an outer working directory.
	tempPrefix := "discovery_unzip_"
	outerDir, err := ioutil.TempDir("", tempPrefix)
	t.Log("outerdir: ", outerDir)
	if err != nil {
		t.Fatalf("ioutil.TempDir(%q, %q): %v", "", tempPrefix, err)
	}
	defer os.RemoveAll(outerDir)

	// We will unzip to an inner directory, so that in case of a regression we
	// don't accidentally unzip outside of our test directory.
	dir := filepath.Join(outerDir, "inner")
	if err := os.Mkdir(dir, os.ModePerm); err != nil {
		t.Fatalf("os.Mkdir(%q): %v", dir, err)
	}

	tests := []struct {
		label     string
		name      string
		contents  []byte
		wantPath  string
		wantError bool
	}{
		{
			label:    "normal unzip",
			name:     "hello.txt",
			contents: []byte("hello world\n"),
			wantPath: "hello.txt",
		}, {
			label:    "nested unzip",
			name:     "foo/.bar/hello.txt",
			contents: []byte("hello world\n"),
			wantPath: "foo/.bar/hello.txt",
		}, {
			label:     "contents too large",
			name:      "hello.txt",
			contents:  []byte("goodbye, cruel world\n"),
			wantError: true,
		}, {
			label:     "non-canonical slashes",
			name:      `foo\hello.txt`,
			wantError: true,
		}, {
			label:    "unclean normal unzip",
			name:     "foo/../hello.txt",
			contents: []byte("hello world\n"),
			// This case could theoretically be allowed, but we exclude it to be
			// consistent with modfetch.
			wantError: true,
		}, {
			label:     "invalid file name",
			name:      ".",
			contents:  []byte("hello world\n"),
			wantError: true,
		}, {
			label:     "clean zip traversal",
			name:      "../hello.txt",
			contents:  []byte("hello world\n"),
			wantError: true,
		}, {
			label:     "unclean zip traversal",
			name:      "foo/../../hello.txt",
			contents:  []byte("hello world\n"),
			wantError: true,
		}, {
			label:     "absolute zip traversal",
			name:      filepath.Join(outerDir, "hello.txt"),
			contents:  []byte("hello world\n"),
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			zipPath := filepath.Join(dir, "test.zip")
			if err := writeZip(zipPath, test.name, test.contents); err != nil {
				t.Fatalf("writeZip(%q, %q, contents): %v", zipPath, test.name, err)
			}

			zr, err := zip.OpenReader(zipPath)
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", zipPath, err)
			}
			defer zr.Close()

			err = writeFileToDir(zr.File[0], dir)
			switch {
			case err != nil && !test.wantError:
				t.Errorf("writeFileToDir(%s, %q): %v, want nil error", zr.File[0].Name, dir, err)
			case err == nil && test.wantError:
				t.Errorf("writeFileToDir(%s, %q): nil error, want error", zr.File[0].Name, dir)
			case err == nil && !test.wantError:
				// Validate contents.
				expectedPath := filepath.Join(dir, test.wantPath)
				got, err := ioutil.ReadFile(expectedPath)
				if err != nil {
					t.Fatalf("ioutil.ReadFile(%q): %v", expectedPath, err)
				}
				if string(got) != string(test.contents) {
					t.Errorf("Wrote contents %q, want %q", string(got), string(test.contents))
				}
			}
		})
	}
}
