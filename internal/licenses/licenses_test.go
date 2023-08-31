// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
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

	// bsd0License is the contents of the 0BSD license. It is detectable
	// by the licensecheck package, but not considered redistributable.
	bsd0License = `Copyright 2019 Google Inc

Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.`

	// apacheSansAppendix is the contents of the Apache 2.0 license, without the appendix.
	apacheSansAppendix = `Apache License
                          Version 2.0, January 2004
                          http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.`

	// unknownLicense is not detectable by the licensecheck package.
	unknownLicense = `THIS IS A LICENSE THAT I JUST MADE UP. YOU CAN DO WHATEVER YOU WANT WITH THIS CODE, TRUST ME.`
)

var mitCoverage = lc.Coverage{
	Percent: 100,
	Match:   []lc.Match{{ID: "MIT"}},
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
			want:      true,
			wantMetas: []*Metadata{{Types: []string{"0BSD"}, FilePath: "LICENSE.md"}},
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
			f, err := os.Open(filepath.Join("testdata", test.filename+".zip"))
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
				for _, l := range d.ModuleLicenses() {
					t.Logf("%v %+v", l.Types, l.Coverage)
				}
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
		{[]string{"MIT", "Unlicense"}, true},
		{[]string{"MIT", "JSON"}, true},
		{[]string{"MIT", "CommonsClause"}, false},
		{[]string{"MIT", "GPL-2.0", "ISC"}, true},
		{[]string{"MIT", "blessing"}, true},
		{[]string{"blessing"}, false},
		{[]string{"Freetype"}, true}, // appears in exceptions/freetype.lre
	} {
		got := Redistributable(test.types)
		if got != test.want {
			t.Errorf("%v: got %t, want %t", test.types, got, test.want)
		}
	}
}

func TestPaths(t *testing.T) {
	zr := newZipReader(t, "m@v1", map[string]string{
		"LICENSE":            "",
		"LICENSE.md":         "",
		"LICENCE":            "",
		"License":            "",
		"COPYING":            "",
		"liCeNse":            "",
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
			[]string{"LICENSE", "LICENCE", "License", "COPYING", "LICENSE.md",
				"liCeNse"},
		},
		{
			NonRootFiles,
			[]string{
				"foo/LICENSE", "foo/LICENSE.md", "foo/LICENCE", "foo/License",
				"foo/COPYING", "pkg/vendor/LICENSE", "foo/license",
			},
		},
		{
			AllFiles,
			[]string{
				"LICENSE", "LICENCE", "License", "COPYING", "LICENSE.md",
				"liCeNse", "foo/LICENSE", "foo/LICENSE.md", "foo/LICENCE", "foo/License",
				"foo/license", "foo/COPYING", "pkg/vendor/LICENSE",
			},
		},
	} {
		t.Run(fmt.Sprintf("which=%d", test.which), func(t *testing.T) {
			d := NewDetector("m", "v1", zr, nil)
			got := d.paths(test.which)
			if diff := cmp.Diff(test.want, got,
				cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("mismatch(-want, +got):\n%s", diff)
			}
		})
	}
}

func TestDetectFile(t *testing.T) {
	for _, test := range []struct {
		file string
		want []string
	}{
		{"atlantis", []string{"Apache-2.0"}},
		{"hid", []string{"BSD-3-Clause", "LGPL-2.1"}},
		{"freetype", []string{"Freetype"}},
		{"mynewt", []string{"Apache-2.0"}},
		{"rocketlaunchr", []string{"MIT"}},
	} {
		t.Run(test.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", test.file+".df"))
			if err != nil {
				t.Fatal(err)
			}
			got, _ := DetectFile(data, test.file, nil)
			if !cmp.Equal(got, test.want) {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestDetectFiles(t *testing.T) {
	defer func(m int64) { maxLicenseSize = m }(maxLicenseSize)
	maxLicenseSize = int64(len(mitLicense) * 10)
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
				{Types: []string{"0BSD"}, FilePath: "COPYING", Coverage: lc.Coverage{
					Percent: 100,
					Match:   []lc.Match{{ID: "0BSD"}},
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
				{Types: []string{"0BSD", "MIT"}, FilePath: "LICENSE", Coverage: lc.Coverage{
					Percent: 100,
					Match: []lc.Match{
						{ID: "MIT"},
						{ID: "0BSD"},
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
		commodo consequat. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod
		tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim
		veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea
		commodo consequat.`,
			},
			want: []*Metadata{
				{
					Types:    []string{"UNKNOWN"},
					FilePath: "foo/LICENSE",
					Coverage: lc.Coverage{
						Percent: 69.361,
						Match:   []lc.Match{{ID: "MIT"}},
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
				"COPYING": strings.Repeat(mitLicense, 11),
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
		{
			name: "apache sans appendix",
			contents: map[string]string{
				"LICENSE": apacheSansAppendix,
			},
			want: []*Metadata{
				{
					Types:    []string{"Apache-2.0"},
					FilePath: "LICENSE",
					Coverage: lc.Coverage{
						Percent: 100,
						Match: []lc.Match{{
							ID: "Apache-2.0",
						}},
					},
				},
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			d := NewDetector("m", "v1", newZipReader(t, "m@v1", test.contents), log.Printf)
			paths := d.paths(AllFiles)
			gotLics := d.detectFiles(paths)
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
			zr := newZipReader(t, module+"@"+version, test.contents)
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

func TestInvalidContentDirPath(t *testing.T) {
	// Make sure we don't crash if the zip's content directory path is invalid according to fs.ValidPath.
	invalidPath := "a//v1.0.0"
	if fs.ValidPath(invalidPath) {
		t.Fatalf("%q is valid", invalidPath)
	}
	zr := newZipReader(t, invalidPath, map[string]string{"a": "b"})
	d := NewDetector("a", "v1.0.0", zr, nil)
	got := d.ModuleLicenses()
	if len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
	got = d.AllLicenses()
	if len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

// newZipReader creates an in-memory zip of the given contents and returns a reader to it.
func newZipReader(t *testing.T, contentDir string, contents map[string]string) *zip.Reader {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range contents {
		fw, err := zw.Create(path.Join(contentDir, name))
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

func BenchmarkBuildDFA(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s, err := lc.NewScanner(append(exceptionLicenses, lc.BuiltinLicenses()...))
		if err != nil {
			b.Fatal(err)
		}
		_ = s
	}
}
