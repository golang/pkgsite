// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"

	"golang.org/x/pkgsite/internal"
)

func TestUnitURLPath(t *testing.T) {
	for _, test := range []struct {
		path, modpath, version, want string
	}{
		{
			"m.com/p", "m.com", "latest",
			"/m.com/p",
		},
		{
			"m.com/p", "m.com", "v1.2.3",
			"/m.com@v1.2.3/p",
		},
		{
			"math", "std", "latest",
			"/math",
		},
		{
			"math", "std", "v1.2.3",
			"/math@go1.2.3",
		},
		{
			"math", "std", "go1.2.3",
			"/math@go1.2.3",
		},
	} {
		got := unitURLPath(&internal.UnitMeta{Path: test.path, ModulePath: test.modpath}, test.version)
		if got != test.want {
			t.Errorf("unitURLPath(%q, %q, %q) = %q, want %q", test.path, test.modpath, test.version, got, test.want)
		}
	}
}

func TestCanonicalURLPath(t *testing.T) {
	for _, test := range []struct {
		path, modpath, version, want string
	}{

		{
			"m.com/p", "m.com", "v1.2.3",
			"/m.com@v1.2.3/p",
		},

		{
			"math", "std", "v1.2.3",
			"/math@go1.2.3",
		},
		{
			"math", "std", "go1.2.3",
			"/math@go1.2.3",
		},
	} {
		um := &internal.UnitMeta{
			Path:       test.path,
			ModulePath: test.modpath,
			Version:    test.version,
		}
		got := canonicalURLPath(um)
		if got != test.want {
			t.Errorf("canonicalURLPath(%q, %q, %q) = %q, want %q", test.path, test.modpath, test.version, got, test.want)
		}
	}
}
