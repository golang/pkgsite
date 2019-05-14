// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package testhelper provides shared functionality and constants to be used in
// Discovery tests. It should only be imported by test files.
package testhelper

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
)

const (
	// MITLicense is the contents of the MIT license. It is detectable by the
	// licensecheck package, and is considered redistributable.
	MITLicense = `Copyright 2019 Google Inc

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.`

	// BSD0License is the contents of the BSD-0-Clause license. It is detectable
	// by the licensecheck package, but not considered redistributable.
	BSD0License = `Copyright 2019 Google Inc

Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.`

	// UnknownLicense is not detectable by the licensecheck package.
	UnknownLicense = `THIS IS A LICENSE THAT I JUST MADE UP. YOU CAN DO WHATEVER YOU WANT WITH THIS CODE, TRUST ME.`
)

// InsecureHTTPClient is used to disable TLS verification when running against
// a test server.
var InsecureHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

func writeZip(w io.Writer, contents map[string]string) (err error) {
	zw := zip.NewWriter(w)
	defer func() {
		if cerr := zw.Close(); cerr != nil {
			err = fmt.Errorf("error: %v, close error: %v", err, cerr)
		}
	}()

	for name, content := range contents {
		fw, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("ZipWriter::Create(): %v", err)
		}
		_, err = io.WriteString(fw, content)
		if err != nil {
			return fmt.Errorf("io.WriteString(...): %v", err)
		}
	}
	return nil
}

// ZipContents creates an in-memory zip of the given contents.
func ZipContents(contents map[string]string) ([]byte, error) {
	bs := &bytes.Buffer{}
	if err := writeZip(bs, contents); err != nil {
		return nil, err
	}
	return bs.Bytes(), nil
}

// TestDataPath returns a path corresponding to a path relative to the calling
// test file. For convenience, rel is assumed to be "/"-delimited.
//
// It panics on failure.
func TestDataPath(rel string) string {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		panic("unable to determine relative path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), filepath.FromSlash(rel)))
}
