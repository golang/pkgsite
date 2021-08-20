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

func TestFSSignature(t *testing.T) {
	zip1 := newzip(t, [][2]string{
		{"file1", "abc"},
		{"file2", "def"},
	})
	zip2 := newzip(t, [][2]string{ // same files, different order, different prefix
		{"file2", "def"},
		{"file1", "abc"},
	})
	zip3 := newzip(t, [][2]string{ // different files
		{"file1", "abc"},
		{"file2d", "ef"},
	})

	sig := func(z []byte) string {
		r, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
		if err != nil {
			t.Fatal(err)
		}
		s, err := FSSignature(r)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	if sig(zip1) != sig(zip2) {
		t.Error("same files, different order: got different signatures, want same")
	}
	if sig(zip1) == sig(zip3) {
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
