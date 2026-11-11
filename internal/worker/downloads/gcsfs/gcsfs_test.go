// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcsfs

import (
	"errors"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/worker/downloads/fakestorage"
)

func TestFS(t *testing.T) {
	modTime := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	memFS := fstest.MapFS{
		"top.txt": &fstest.MapFile{
			Data:    []byte("hello world"),
			ModTime: modTime,
		},
		"date=2026-10-20/counts-00000-of-00002.csv": &fstest.MapFile{
			Data:    []byte("module_path,version,mod_count,zip_count\nexample.com/a,v1.0.0,20,100\n"),
			ModTime: modTime,
		},
		"date=2026-10-20/counts-00001-of-00002.csv": &fstest.MapFile{
			Data:    []byte("module_path,version,mod_count,zip_count\nexample.com/b,v1.0.0,10,50\n"),
			ModTime: modTime,
		},
		"date=2026-10-20-extra/other.csv": &fstest.MapFile{
			Data:    []byte("other"),
			ModTime: modTime,
		},
	}

	const bucket = "test-bucket"
	srv := fakestorage.NewServer(bucket, memFS)
	defer srv.Close()

	t.Setenv("STORAGE_EMULATOR_HOST", srv.URL)

	gfs, err := New(t.Context(), bucket)
	if err != nil {
		t.Fatal(err)
	}
	defer gfs.Close()

	// Verify ReadDir on root.
	rootEntries, err := fs.ReadDir(gfs, ".")
	if err != nil {
		t.Fatal(err)
	}
	var gotRoot []string
	for _, e := range rootEntries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		gotRoot = append(gotRoot, name)
	}
	wantRoot := []string{"date=2026-10-20/", "date=2026-10-20-extra/", "top.txt"}
	if diff := cmp.Diff(wantRoot, gotRoot); diff != "" {
		t.Errorf("ReadDir(.) mismatch (-want +got):\n%s", diff)
	}

	// Verify ReadDir on subdirectory does not include sibling directory sharing prefix.
	subEntries, err := fs.ReadDir(gfs, "date=2026-10-20")
	if err != nil {
		t.Fatal(err)
	}
	var gotSub []string
	for _, e := range subEntries {
		gotSub = append(gotSub, e.Name())
	}
	wantSub := []string{"counts-00000-of-00002.csv", "counts-00001-of-00002.csv"}
	if diff := cmp.Diff(wantSub, gotSub); diff != "" {
		t.Errorf("ReadDir(date=2026-10-20) mismatch (-want +got):\n%s", diff)
	}

	// Verify Open and Stat on a file.
	f, err := gfs.Open("top.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Name(), "top.txt"; got != want {
		t.Errorf("info.Name() = %q, want %q", got, want)
	}
	if got, want := info.Size(), int64(len("hello world")); got != want {
		t.Errorf("info.Size() = %d, want %d", got, want)
	}
	if got, want := info.ModTime(), modTime; !got.Equal(want) {
		t.Errorf("info.ModTime() = %v, want %v", got, want)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "hello world"; got != want {
		t.Errorf("ReadAll = %q, want %q", got, want)
	}

	// Verify Open and ReadDir on nonexistent paths return fs.ErrNotExist.
	if _, err := gfs.Open("nonexistent.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open(nonexistent.txt) = %v, want fs.ErrNotExist", err)
	}
	if _, err := gfs.ReadDir("nonexistent"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadDir(nonexistent) = %v, want fs.ErrNotExist", err)
	}

	// Verify fs.Sub works with gcsfs.FS.
	subFS, err := fs.Sub(gfs, "date=2026-10-20")
	if err != nil {
		t.Fatal(err)
	}
	subData, err := fs.ReadFile(subFS, "counts-00000-of-00002.csv")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(subData), string(memFS["date=2026-10-20/counts-00000-of-00002.csv"].Data); got != want {
		t.Errorf("ReadFile via fs.Sub = %q, want %q", got, want)
	}

	// Verify invalid paths return fs.ErrInvalid.
	if _, err := gfs.Open("."); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("Open(.) = %v, want fs.ErrInvalid", err)
	}
	if _, err := gfs.Open("../bad"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("Open(../bad) = %v, want fs.ErrInvalid", err)
	}
	if _, err := gfs.ReadDir("../bad"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("ReadDir(../bad) = %v, want fs.ErrInvalid", err)
	}
}
