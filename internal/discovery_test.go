// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"testing"

	"golang.org/x/pkgsite/internal/stdlib"
)

func TestSeriesPathForModule(t *testing.T) {
	for _, test := range []struct {
		modulePath, wantSeriesPath string
	}{
		{
			modulePath:     "github.com/foo",
			wantSeriesPath: "github.com/foo",
		},
		{
			modulePath:     "github.com/foo/v2",
			wantSeriesPath: "github.com/foo",
		},
		{
			modulePath:     "std",
			wantSeriesPath: "std",
		},
		{
			modulePath:     "gopkg.in/russross/blackfriday.v2",
			wantSeriesPath: "gopkg.in/russross/blackfriday",
		},
	} {
		t.Run(test.modulePath, func(t *testing.T) {
			if got := SeriesPathForModule(test.modulePath); got != test.wantSeriesPath {
				t.Errorf("SeriesPathForModule(%q) = %q; want = %q", test.modulePath, got, test.wantSeriesPath)
			}
		})
	}
}

func TestMajorVersionForModule(t *testing.T) {
	for _, test := range []struct {
		in, want string
	}{
		{"m.com", ""},
		{"m.com/v2", "v2"},
		{"gopkg.in/m.v1", "v1"},
		{"m.com/v2.1", ""},
		{"", ""},
	} {
		got := MajorVersionForModule(test.in)
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.in, got, test.want)
		}
	}
}

func TestV1Path(t *testing.T) {
	for _, test := range []struct {
		modulePath, suffix string
		want               string
	}{
		{"mod.com/foo", "bar", "mod.com/foo/bar"},
		{"mod.com/foo/v2", "bar", "mod.com/foo/bar"},
		{"std", "bar/baz", "bar/baz"},
	} {
		p := test.suffix
		if test.modulePath != stdlib.ModulePath {
			p = test.modulePath + "/" + test.suffix
		}
		got := V1Path(p, test.modulePath)
		if got != test.want {
			t.Errorf("V1Path(%q, %q) = %q, want %q",
				test.modulePath, p, got, test.want)
		}
	}
}
