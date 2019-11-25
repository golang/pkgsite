// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"testing"

	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/testing/sample"
)

func TestFileSource(t *testing.T) {
	for _, tc := range []struct {
		modulePath, version, filePath, want string
	}{
		{
			modulePath: sample.ModulePath,
			version:    sample.VersionString,
			filePath:   "LICENSE.txt",
			want:       fmt.Sprintf("%s@%s/%s", sample.ModulePath, sample.VersionString, "LICENSE.txt"),
		},
		{
			modulePath: stdlib.ModulePath,
			version:    "v1.13.0",
			filePath:   "README.md",
			want:       fmt.Sprintf("go.googlesource.com/go/+/refs/tags/%s/%s", "go1.13", "README.md"),
		},
		{
			modulePath: stdlib.ModulePath,
			version:    "v1.13.invalid",
			filePath:   "README.md",
			want:       fmt.Sprintf("go.googlesource.com/go/+/refs/heads/master/%s", "README.md"),
		},
	} {
		t.Run(fmt.Sprintf("%s@%s/%s", tc.modulePath, tc.version, tc.filePath), func(t *testing.T) {
			if got := fileSource(tc.modulePath, tc.version, tc.filePath); got != tc.want {
				t.Errorf("fileSource(%q, %q, %q) = %q; want = %q", tc.modulePath, tc.version, tc.filePath, got, tc.want)
			}
		})
	}
}

func TestHackUpDocumentation(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"nothing burger", "nothing burger"},
		{`<a href="/pkg/foo">foo</a>`, `<a href="/foo?tab=doc">foo</a>`},
		{`<a href="/pkg/foo"`, `<a href="/pkg/foo"`},
		{
			`<a href="/pkg/foo"><a href="/pkg/bar">bar</a></a>`,
			`<a href="/foo?tab=doc"><a href="/pkg/bar">bar</a></a>`,
		},
		{
			`<a href="/pkg/foo">foo</a>
		   <a href="/pkg/bar">bar</a>`,
			`<a href="/foo?tab=doc">foo</a>
		   <a href="/bar?tab=doc">bar</a>`,
		},
		{`<ahref="/pkg/foo">foo</a>`, `<ahref="/pkg/foo">foo</a>`},
		{`<allhref="/pkg/foo">foo</a>`, `<allhref="/pkg/foo">foo</a>`},
		{`<a nothref="/pkg/foo">foo</a>`, `<a nothref="/pkg/foo">foo</a>`},
		{`<a href="/pkg/foo#identifier">foo</a>`, `<a href="/foo?tab=doc#identifier">foo</a>`},
		{`<a href="#identifier">foo</a>`, `<a href="#identifier">foo</a>`},
		{`<span id="Indirect.Type"></span>func (in <a href="#Indirect">Indirect</a>) Type() <a href="/pkg/reflect">reflect</a>.<a href="/pkg/reflect#Type">Type</a>`,
			`<span id="Indirect.Type"></span>func (in <a href="#Indirect">Indirect</a>) Type() <a href="/reflect?tab=doc">reflect</a>.<a href="/reflect?tab=doc#Type">Type</a>`},
	}

	for _, test := range tests {
		if got := string(hackUpDocumentation(test.body)); got != test.want {
			t.Errorf("hackUpDocumentation(%s) = %s, want %s", test.body, got, test.want)
		}
	}
}
