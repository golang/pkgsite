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

	"golang.org/x/discovery/internal/thirdparty/module"
)

// maxFileSize is the maximum filesize that is allowed for reading.  If a .go
// file is encountered that exceeds maxFileSize, the fetch request will fail.
// All other filetypes will be ignored.
//
// It is mutable for testing purposes.
var maxFileSize = uint64(3e7)

// readZipFile returns the uncompressed contents of f or an error if the
// uncompressed size of f exceeds maxFileSize.
func readZipFile(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxFileSize {
		return nil, fmt.Errorf("file size %d exceeds %d, skipping", f.UncompressedSize64, maxFileSize)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("f.Open() for %q: %v", f.Name, err)
	}
	defer rc.Close()

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll(rc) for %q: %v", f.Name, err)
	}
	return b, nil
}

// writeFileToDir writes the contents of f to the directory dir.  It returns an
// error if the file has an invalid path, if the file exceeds the maximum
// allowable file size, or if there is an error in file extraction.
func writeFileToDir(f *zip.File, dir string) (err error) {
	if err := module.CheckFilePath(f.Name); err != nil {
		return fmt.Errorf("module.CheckFilePath(%q): %v", f.Name, err)
	}

	dest := filepath.Clean(filepath.Join(dir, filepath.FromSlash(f.Name)))
	fileDir := filepath.Dir(dest)
	if _, err := os.Stat(fileDir); os.IsNotExist(err) {
		if err := os.MkdirAll(fileDir, os.ModePerm); err != nil {
			return fmt.Errorf("os.MkdirAll(%q, os.ModePerm): %v", fileDir, err)
		}
	}

	file, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("os.Create(%q): %v", dest, err)
	}
	defer func() {
		cerr := file.Close()
		if err == nil && cerr != nil {
			err = fmt.Errorf("file.Close: %v", cerr)
		}
	}()

	b, err := readZipFile(f)
	if err != nil {
		return fmt.Errorf("readZipFile(%q): %v", f.Name, err)
	}

	if _, err := file.Write(b); err != nil {
		return fmt.Errorf("file.Write: %v", err)
	}
	return nil
}
