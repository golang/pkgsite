// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	// MIT is detectable by the licensecheck package, and is considered redistributable.
	mit = `Copyright 2019 Google Inc

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.`

	// BSD-0-Clause is detectable by the licensecheck package, but not considered redistributable.
	bsd0 = `Copyright 2019 Google Inc

Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.`

	// unknown is not detectable by the licensecheck package.
	unknown = `THIS IS A LICENSE THAT I JUST MADE UP. YOU CAN DO WHATEVER YOU WANT WITH THIS CODE, TRUST ME.`
)

func TestDetect(t *testing.T) {
	testCases := []struct {
		name, subdir string
		contents     map[string]string
		want         []*Metadata
	}{
		{
			name: "valid license",
			contents: map[string]string{
				"foo/LICENSE": mit,
			},
			want: []*Metadata{{Type: "MIT", FilePath: "foo/LICENSE"}},
		}, {
			name: "valid license md format",
			contents: map[string]string{
				"foo/LICENSE.md": mit,
			},
			want: []*Metadata{{Type: "MIT", FilePath: "foo/LICENSE.md"}},
		}, {
			name: "valid license trim prefix",
			contents: map[string]string{
				"rsc.io/quote@v1.4.1/LICENSE.md": mit,
			},
			subdir: "rsc.io/quote@v1.4.1",
			want:   []*Metadata{{Type: "MIT", FilePath: "LICENSE.md"}},
		}, {
			name: "multiple licenses",
			contents: map[string]string{
				"LICENSE":        mit,
				"bar/LICENSE.md": mit,
				"foo/COPYING":    bsd0,
			},
			want: []*Metadata{
				{Type: "MIT", FilePath: "LICENSE"},
				{Type: "MIT", FilePath: "bar/LICENSE.md"},
				{Type: "BSD-0-Clause", FilePath: "foo/COPYING"},
			},
		}, {
			name: "multiple licenses in a single file",
			contents: map[string]string{
				"LICENSE": mit + "\n" + bsd0,
			},
			want: []*Metadata{
				{Type: "BSD-0-Clause", FilePath: "LICENSE"},
				{Type: "MIT", FilePath: "LICENSE"},
			},
		}, {
			name: "unknown license",
			contents: map[string]string{
				"LICENSE": unknown,
			},
			want: []*Metadata{
				{Type: unknownLicense, FilePath: "LICENSE"},
			},
		}, {
			name: "low coverage license",
			contents: map[string]string{
				"LICENSE": mit + `
Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod
tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim
veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea
commodo consequat.`,
			},
			want: []*Metadata{
				{Type: unknownLicense, FilePath: "LICENSE"},
			},
		}, {
			name: "no license",
			contents: map[string]string{
				"foo/blah.go": "package foo\n\nconst Foo = 42",
			},
		}, {
			name: "invalid license file name",
			contents: map[string]string{
				"MYLICENSEFILE": mit,
			},
		}, {
			name: "ignores licenses in vendored packages, but not packages named vendor",
			contents: map[string]string{
				"pkg/vendor/LICENSE": mit,
				"vendor/pkg/LICENSE": mit,
			},
			want: []*Metadata{
				{Type: "MIT", FilePath: "pkg/vendor/LICENSE"},
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var (
				z   *zip.Reader
				err error
			)
			if test.contents != nil {
				z, err = makeZipReader(test.contents)
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := Detect(test.subdir, z)
			if err != nil {
				t.Errorf("detectLicenses(z): %v", err)
			}
			sort.Slice(got, func(i, j int) bool {
				if got[i].FilePath < got[j].FilePath {
					return true
				} else if got[i].FilePath > got[j].FilePath {
					return false
				}
				return got[i].Type < got[j].Type
			})
			var gotFiles []*Metadata
			for _, l := range got {
				gotFiles = append(gotFiles, &l.Metadata)
			}
			if diff := cmp.Diff(test.want, gotFiles); diff != "" {
				t.Errorf("detectLicense(z) mismatch (-want +got):\n%s", diff)
			}
		})
	}
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

func makeZipReader(contents map[string]string) (*zip.Reader, error) {
	bs := &bytes.Buffer{}
	if err := writeZip(bs, contents); err != nil {
		return nil, err
	}
	reader := bytes.NewReader(bs.Bytes())
	return zip.NewReader(reader, int64(bs.Len()))
}
