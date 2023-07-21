// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"net/url"
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

func TestTrimmedEscapedPath(t *testing.T) {
	for _, test := range []struct {
		in, want string
	}{
		{"a.png", "a.png"},
		{" a.png   ", "a.png"},
		{"a b.png", "a%20b.png"},
		{" a b.png ", "a%20b.png"},
		{".a/b.gif", ".a/b.gif"},
	} {
		u, err := url.Parse(test.in)
		if err != nil {
			t.Fatal(err)
		}
		got := trimmedEscapedPath(u)
		if got != test.want {
			t.Errorf("escapePath(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestPackageSubdir(t *testing.T) {
	for _, test := range []struct {
		pkgPath, modulePath string
		want                string
	}{
		// package at module root
		{"github.com/pkg/errors", "github.com/pkg/errors", ""},
		// package inside module
		{"github.com/google/go-cmp/cmp", "github.com/google/go-cmp", "cmp"},
		// stdlib package
		{"context", stdlib.ModulePath, "context"},
	} {
		got := internal.Suffix(test.pkgPath, test.modulePath)
		if got != test.want {
			t.Errorf("internal.Suffix(%q, %q) = %q, want %q", test.pkgPath, test.modulePath, got, test.want)
		}
	}
}
