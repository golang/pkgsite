// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	lc "github.com/google/licensecheck"
)

const (
	// mitLicense is the contents of the MIT license. It is detectable by the
	// licensecheck package, and is considered redistributable.
	mitLicense = `Copyright 2019 Google Inc

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.`

	// bsd0License is the contents of the BSD-0-Clause license. It is detectable
	// by the licensecheck package, but not considered redistributable.
	bsd0License = `Copyright 2019 Google Inc

Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.`

	// unknownLicense is not detectable by the licensecheck package.
	unknownLicense = `THIS IS A LICENSE THAT I JUST MADE UP. YOU CAN DO WHATEVER YOU WANT WITH THIS CODE, TRUST ME.`
)

var mitCoverage = lc.Coverage{
	Percent: 100,
	Match:   []lc.Match{{Name: "MIT", Type: lc.MIT, Percent: 100}},
}

// testDataPath returns a path corresponding to a path relative to the calling
// test file. For convenience, rel is assumed to be "/"-delimited.
//
// It panics on failure.
func testDataPath(rel string) string {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		panic("unable to determine relative path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), filepath.FromSlash(rel)))
}

func TestModuleIsRedistributable(t *testing.T) {
	// More thorough tests of the helper functions are below. This end-to-end test covers
	// a few key cases using actual zip files.
	for _, test := range []struct {
		filename  string
		module    string
		version   string
		want      bool
		wantMetas []*Metadata
	}{
		{
			filename:  "xtime",
			module:    "golang.org/x/time",
			version:   "v0.0.0-20191024005414-555d28b269f0",
			want:      true,
			wantMetas: []*Metadata{{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"}},
		},
		{
			filename:  "smasher",
			module:    "github.com/smasher164/mem",
			version:   "v0.0.0-20191114064341-4e07bd0f0d69",
			want:      false,
			wantMetas: []*Metadata{{Types: []string{"BSD-0-Clause"}, FilePath: "LICENSE.md"}},
		},
		{
			filename: "gioui",
			module:   "gioui.org",
			version:  "v0.0.0-20200103103112-ccbcbdbfbd4f",
			want:     true,
			wantMetas: []*Metadata{
				{Types: []string{"MIT"}, FilePath: "LICENSE-MIT"},
				{Types: []string{"Unlicense"}, FilePath: "UNLICENSE"},
			},
		},
		{
			filename: "gonum",
			module:   "gonum.org/v1/gonum",
			version:  "v0.6.2",
			want:     true,
			wantMetas: []*Metadata{
				{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
				{Types: []string{"MIT"}, FilePath: "graph/formats/cytoscapejs/testdata/LICENSE"},
				{Types: []string{"MIT"}, FilePath: "graph/formats/sigmajs/testdata/LICENSE.txt"},
			},
		},
	} {
		t.Run(test.filename, func(t *testing.T) {
			f, err := os.Open(filepath.Join(testDataPath("testdata"), test.filename+".zip"))
			if err != nil {
				t.Fatal(err)
			}
			fi, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			zr, err := zip.NewReader(f, fi.Size())
			if err != nil {
				t.Fatal(err)
			}
			d := NewDetector(test.module, test.version, zr, nil)
			got := d.ModuleIsRedistributable()
			if got != test.want {
				t.Fatalf("got %t, want %t", got, test.want)
			}
			var gotMetas []*Metadata
			for _, lic := range d.AllLicenses() {
				gotMetas = append(gotMetas, lic.Metadata)
			}
			opts := []cmp.Option{
				cmpopts.IgnoreFields(Metadata{}, "Coverage"),
				cmpopts.SortSlices(func(m1, m2 *Metadata) bool { return m1.FilePath < m2.FilePath }),
			}
			if diff := cmp.Diff(test.wantMetas, gotMetas, opts...); diff != "" {
				t.Errorf("mismatch(-want, +got):\n%s", diff)
			}
		})
	}
}

func TestRedistributable(t *testing.T) {
	for _, test := range []struct {
		types []string
		want  bool
	}{
		{nil, false},
		{[]string{unknownLicenseType}, false},
		{[]string{"MIT"}, true},
		{[]string{"MIT", "Unlicense"}, false},
		{[]string{"MIT", "GPL2", "ISC"}, true},
	} {
		got := Redistributable(test.types)
		if got != test.want {
			t.Errorf("%v: got %t, want %t", test.types, got, test.want)
		}
	}
}

func TestFiles(t *testing.T) {
	zr := newZipReader(t, "m@v1", map[string]string{
		"LICENSE":            "",
		"LICENSE.md":         "",
		"LICENCE":            "",
		"License":            "",
		"COPYING":            "",
		"license":            "",
		"foo/LICENSE":        "",
		"foo/LICENSE.md":     "",
		"foo/LICENCE":        "",
		"foo/License":        "",
		"foo/COPYING":        "",
		"foo/license":        "",
		"vendor/pkg/LICENSE": "", // vendored files ignored
		"pkg/vendor/LICENSE": "", // not a vendored file, but a package named "vendor"
	})
	for _, test := range []struct {
		which WhichFiles
		want  []string
	}{
		{
			RootFiles,
			[]string{"m@v1/LICENSE", "m@v1/LICENCE", "m@v1/License", "m@v1/COPYING", "m@v1/LICENSE.md"},
		},
		{
			NonRootFiles,
			[]string{
				"m@v1/foo/LICENSE", "m@v1/foo/LICENSE.md", "m@v1/foo/LICENCE", "m@v1/foo/License", "m@v1/foo/COPYING",
				"m@v1/pkg/vendor/LICENSE",
			},
		},
		{
			AllFiles,
			[]string{
				"m@v1/LICENSE", "m@v1/LICENCE", "m@v1/License", "m@v1/COPYING", "m@v1/LICENSE.md",
				"m@v1/foo/LICENSE", "m@v1/foo/LICENSE.md", "m@v1/foo/LICENCE", "m@v1/foo/License", "m@v1/foo/COPYING",
				"m@v1/pkg/vendor/LICENSE",
			},
		},
	} {
		t.Run(fmt.Sprintf("which=%d", test.which), func(t *testing.T) {
			d := NewDetector("m", "v1", zr, nil)
			gotFiles := d.Files(test.which)
			var got []string
			for _, f := range gotFiles {
				got = append(got, f.Name)
			}
			if diff := cmp.Diff(test.want, got,
				cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("mismatch(-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDetectFiles(t *testing.T) {
	defer func(m uint64) { maxLicenseSize = m }(maxLicenseSize)
	maxLicenseSize = uint64(len(mitLicense) * 3)
	testCases := []struct {
		name     string
		contents map[string]string
		want     []*Metadata
	}{
		{
			name: "valid license",
			contents: map[string]string{
				"foo/LICENSE": mitLicense,
			},
			want: []*Metadata{{Types: []string{"MIT"}, FilePath: "foo/LICENSE", Coverage: mitCoverage}},
		},

		{
			name: "multiple licenses",
			contents: map[string]string{
				"LICENSE":        mitLicense,
				"foo/LICENSE.md": mitLicense,
				"COPYING":        bsd0License,
			},
			want: []*Metadata{
				{Types: []string{"BSD-0-Clause"}, FilePath: "COPYING", Coverage: lc.Coverage{
					Percent: 100,
					Match:   []lc.Match{{Name: "BSD-0-Clause", Type: lc.BSD, Percent: 100}},
				}},
				{Types: []string{"MIT"}, FilePath: "LICENSE", Coverage: mitCoverage},
				{Types: []string{"MIT"}, FilePath: "foo/LICENSE.md", Coverage: mitCoverage},
			},
		},
		{
			name: "multiple licenses in a single file",
			contents: map[string]string{
				"LICENSE": mitLicense + "\n" + bsd0License,
			},
			want: []*Metadata{
				{Types: []string{"BSD-0-Clause", "MIT"}, FilePath: "LICENSE", Coverage: lc.Coverage{
					Percent: 100,
					Match: []lc.Match{
						{Name: "MIT", Type: lc.MIT, Percent: 100},
						{Name: "BSD-0-Clause", Type: lc.BSD, Percent: 100},
					},
				}},
			},
		},
		{
			name: "unknown license",
			contents: map[string]string{
				"LICENSE": unknownLicense,
			},
			want: []*Metadata{
				{Types: []string{"UNKNOWN"}, FilePath: "LICENSE"},
			},
		},
		{
			name: "low coverage license",
			contents: map[string]string{
				"foo/LICENSE": mitLicense + `
		Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod
		tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim
		veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea
		commodo consequat.`,
			},
			want: []*Metadata{
				{
					Types:    []string{"UNKNOWN"},
					FilePath: "foo/LICENSE",
					Coverage: lc.Coverage{
						Percent: 81.9095,
						Match:   []lc.Match{{Name: "MIT", Type: lc.MIT, Percent: 100}},
					},
				},
			},
		},
		{
			name: "no license",
			contents: map[string]string{
				"blah.go": "package foo\n\nconst Foo = 42",
			},
			want: nil,
		},
		{
			name: "unreadable license",
			contents: map[string]string{
				"LICENSE": mitLicense,
				"COPYING": strings.Repeat(mitLicense, 4),
			},
			want: []*Metadata{
				{
					Types:    []string{"UNKNOWN"},
					FilePath: "COPYING",
				},
				{
					Types:    []string{"MIT"},
					FilePath: "LICENSE",
					Coverage: mitCoverage,
				},
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			d := NewDetector("m", "v1", newZipReader(t, "m@v1", test.contents), nil)
			files := d.Files(AllFiles)
			gotLics := d.detectFiles(files)
			sort.Slice(gotLics, func(i, j int) bool {
				return gotLics[i].FilePath < gotLics[j].FilePath
			})
			var got []*Metadata
			for _, l := range gotLics {
				got = append(got, l.Metadata)
			}

			opts := []cmp.Option{
				cmp.Comparer(coveragePercentEqual),
				cmpopts.IgnoreFields(lc.Match{}, "Start", "End"),
			}
			if diff := cmp.Diff(test.want, got, opts...); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsException(t *testing.T) {
	const (
		exceptionModule = "gioui.org"
		version         = "v1.2.3"
	)
	for _, test := range []struct {
		name     string
		module   string
		contents map[string]string
		want     bool
	}{
		{
			name:   "valid exception",
			module: exceptionModule,
			contents: map[string]string{
				"COPYING":     exceptions[exceptionModule]["COPYING"].contents,
				"UNLICENSE":   exceptions[exceptionModule]["UNLICENSE"].contents,
				"LICENSE-MIT": exceptions[exceptionModule]["LICENSE-MIT"].contents,
			},
			want: true,
		},
		{
			name:     "not a known exception",
			module:   "golang.org/x/tools",
			contents: nil, // irrelevant
			want:     false,
		},
		{
			name:   "missing a file",
			module: exceptionModule,
			contents: map[string]string{
				"COPYING":   exceptions[exceptionModule]["COPYING"].contents,
				"UNLICENSE": exceptions[exceptionModule]["UNLICENSE"].contents,
			},
			want: false,
		},
		{
			name:   "changed file contents",
			module: exceptionModule,
			contents: map[string]string{
				"COPYING":     exceptions[exceptionModule]["COPYING"].contents,
				"UNLICENSE":   exceptions[exceptionModule]["UNLICENSE"].contents,
				"LICENSE-MIT": exceptions[exceptionModule]["LICENSE-MIT"].contents + " Amen.",
			},
			want: false,
		},
		{
			name:   "extra files",
			module: exceptionModule,
			contents: map[string]string{
				"COPYING":        exceptions[exceptionModule]["COPYING"].contents,
				"UNLICENSE":      exceptions[exceptionModule]["UNLICENSE"].contents,
				"LICENSE-MIT":    exceptions[exceptionModule]["LICENSE-MIT"].contents,
				"subdir/LICENSE": mitLicense,
			},
			want: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			zr := newZipReader(t, contentsDir(test.module, version), test.contents)
			d := NewDetector(test.module, version, zr, nil)
			got, _ := d.isException()
			if got != test.want {
				t.Errorf("got %t, want %t", got, test.want)
			}
		})
	}
}

func TestPackageInfo(t *testing.T) {
	const (
		module  = "mod"
		version = "v1.2.3"
	)
	meta := func(typ, path string) *Metadata {
		return &Metadata{Types: []string{typ}, FilePath: path}
	}

	for _, test := range []struct {
		name       string
		contents   map[string]string
		wantRedist bool
		wantMetas  []*Metadata
	}{
		{
			name: "no package license",
			contents: map[string]string{
				"LICENSE":        mitLicense,
				"dir/pkg/foo.go": "package pkg",
			},
			wantRedist: true,
			wantMetas:  []*Metadata{meta("MIT", "LICENSE")},
		},
		{
			name: "redistributable package license",
			contents: map[string]string{
				"LICENSE":            mitLicense,
				"dir/pkg/foo.go":     "package pkg",
				"dir/pkg/License.md": mitLicense,
			},
			wantRedist: true,
			wantMetas:  []*Metadata{meta("MIT", "LICENSE"), meta("MIT", "dir/pkg/License.md")},
		},
		{

			name: "module is but package is not",
			contents: map[string]string{
				"LICENSE":            mitLicense,
				"dir/pkg/foo.go":     "package pkg",
				"dir/pkg/License.md": unknownLicense,
			},
			wantRedist: false,
			wantMetas:  []*Metadata{meta("MIT", "LICENSE"), meta(unknownLicenseType, "dir/pkg/License.md")},
		},
		{
			name: "package is but module is not",
			contents: map[string]string{
				"LICENSE":            unknownLicense,
				"dir/pkg/foo.go":     "package pkg",
				"dir/pkg/License.md": mitLicense,
			},
			wantRedist: false,
			wantMetas:  []*Metadata{meta(unknownLicenseType, "LICENSE"), meta("MIT", "dir/pkg/License.md")},
		},
		{
			name: "intermediate directories",
			contents: map[string]string{
				"LICENSE":            mitLicense,
				"dir/pkg/foo.go":     "package pkg",
				"dir/pkg/License.md": mitLicense,
				"dir/LICENSE.txt":    unknownLicense,
			},
			wantRedist: false,
			wantMetas: []*Metadata{
				meta("MIT", "LICENSE"),
				meta(unknownLicenseType, "dir/LICENSE.txt"),
				meta("MIT", "dir/pkg/License.md"),
			},
		},
		{
			name: "other package",
			contents: map[string]string{
				"LICENSE":            mitLicense,
				"dir/pkg/foo.go":     "package pkg",
				"dir/pkg/License.md": mitLicense,
				"dir/LICENSE.txt":    mitLicense,
				"dir/pkg2/LICENSE":   unknownLicense, // should be ignored
			},
			wantRedist: true,
			wantMetas: []*Metadata{
				meta("MIT", "LICENSE"),
				meta("MIT", "dir/LICENSE.txt"),
				meta("MIT", "dir/pkg/License.md"),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			zr := newZipReader(t, contentsDir(module, version), test.contents)
			d := NewDetector(module, version, zr, nil)
			gotRedist, gotLics := d.PackageInfo("dir/pkg")
			if gotRedist != test.wantRedist {
				t.Errorf("redist: got %t, want %t", gotRedist, test.wantRedist)
			}
			var gotMetas []*Metadata
			for _, l := range gotLics {
				gotMetas = append(gotMetas, l.Metadata)
			}
			opts := []cmp.Option{
				cmpopts.IgnoreFields(Metadata{}, "Coverage"),
				cmpopts.SortSlices(func(m1, m2 *Metadata) bool { return m1.FilePath < m2.FilePath }),
			}
			if diff := cmp.Diff(test.wantMetas, gotMetas, opts...); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// newZipReader creates an in-memory zip of the given contents and returns a reader to it.
func newZipReader(t *testing.T, contentsDir string, contents map[string]string) *zip.Reader {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range contents {
		fw, err := zw.Create(path.Join(contentsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.WriteString(fw, content)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

// coveragePercentEqual considers two floats the same if they are within 4
// percentage points, and both are on the same side of 90% (our threshold).
func coveragePercentEqual(a, b float64) bool {
	if (a >= 90) != (b >= 90) {
		return false
	}
	return math.Abs(a-b) <= 4
}
