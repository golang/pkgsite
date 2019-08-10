// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dzip

import (
	"archive/zip"
	"fmt"
	"io/ioutil"

	"golang.org/x/discovery/internal/derrors"
)

// MaxFileSize is the maximum filesize that is allowed for reading.
// The fetch process should fail if it encounters a file exceeding
// this limit.
//
// It is mutable for testing purposes.
var MaxFileSize = uint64(3e7)

// ReadZipFile decompresses zip file f and returns its uncompressed contents.
// The caller can check f.UncompressedSize64 before calling ReadZipFile to
// get the expected uncompressed size of f.
func ReadZipFile(f *zip.File) (_ []byte, err error) {
	defer derrors.Add(&err, "dzip.ReadZipFile(%q)", f.Name)

	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("f.Open(): %v", err)
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("ioutil.ReadAll(r): %v", err)
	}
	if err := r.Close(); err != nil {
		return nil, fmt.Errorf("closing: %v", err)
	}
	return b, nil
}
