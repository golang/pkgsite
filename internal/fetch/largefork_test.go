// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

func TestZipSignature(t *testing.T) {
	zip1 := newzip(t, [][2]string{
		{"p/file1", "abc"},
		{"p/file2", "def"},
	})
	zip2 := newzip(t, [][2]string{ // same files, different order, different prefix
		{"q/file2", "def"},
		{"q/file1", "abc"},
	})
	zip3 := newzip(t, [][2]string{ // different files
		{"r/file1", "abc"},
		{"r/file2d", "ef"},
	})

	sig := func(z []byte, prefix string) string {
		r, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
		if err != nil {
			t.Fatal(err)
		}
		s, err := ZipSignature(r, prefix)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	if sig(zip1, "p") != sig(zip2, "q") {
		t.Error("same files, different order: got different signatures, want same")
	}
	if sig(zip1, "p") == sig(zip3, "r") {
		t.Error("different files: got same signatures, wanted different")
	}
}

func newzip(t *testing.T, files [][2]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		fw, err := zw.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(fw, f[1])
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
