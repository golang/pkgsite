// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/url"
	"strings"
	"testing"
)

func TestPkgGoDevURL(t *testing.T) {
	testCases := []struct {
		from, to string
	}{
		{
			from: "https://godoc.org",
			to:   "https://pkg.go.dev?utm_source=godoc",
		},
		{
			from: "https://godoc.org/-/about",
			to:   "https://pkg.go.dev/about?utm_source=godoc",
		},
		{
			from: "https://godoc.org/-/go",
			to:   "https://pkg.go.dev/std?utm_source=godoc",
		},
		{
			from: "https://godoc.org/-/subrepo",
			to:   "https://pkg.go.dev/search?q=golang.org%2Fx&utm_source=godoc",
		},
		{
			from: "https://godoc.org/C",
			to:   "https://pkg.go.dev/C?utm_source=godoc",
		},
		{
			from: "https://godoc.org/?q=foo",
			to:   "https://pkg.go.dev/search?q=foo&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?imports",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=imports&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?importers",
			to:   "https://pkg.go.dev/cloud.google.com/go/storage?tab=importedby&utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?status.svg",
			to:   "https://pkg.go.dev/badge/cloud.google.com/go/storage?utm_source=godoc",
		},
		{
			from: "https://godoc.org/cloud.google.com/go/storage?status.png",
			to:   "https://pkg.go.dev/badge/cloud.google.com/go/storage?utm_source=godoc",
		},
		{
			from: "https://godoc.org/github.com/golang/go",
			to:   "https://pkg.go.dev/std?utm_source=godoc",
		},
		{
			from: "https://godoc.org/github.com/golang/go/src",
			to:   "https://pkg.go.dev/std?utm_source=godoc",
		},
		{
			from: "https://godoc.org/github.com/golang/go/src/cmd/vet",
			to:   "https://pkg.go.dev/cmd/vet?utm_source=godoc",
		},
		{
			from: "https://godoc.org/golang.org/x/vgo/vendor/cmd/go/internal/modfile",
			to:   "https://pkg.go.dev/?utm_source=godoc",
		},
		{
			from: "https://godoc.org/golang.org/x/vgo/vendor",
			to:   "https://pkg.go.dev/?utm_source=godoc",
		},
		{
			from: "https://godoc.org/cryptoscope.co/go/specialκ",
			to:   "https://golang.org/issue/43036",
		},
		{
			from: "https://godoc.org/github.com/badimportpath//doubleslash",
			to:   "https://pkg.go.dev/github.com/badimportpath//doubleslash?utm_source=godoc",
		},
		{
			from: "https://godoc.org/github.com/google/go-containerregistry/",
			to:   "https://pkg.go.dev/github.com/google/go-containerregistry?utm_source=godoc",
		},
	}

	for _, tc := range testCases {
		t.Run(strings.ReplaceAll(tc.from, "/", " "), func(t *testing.T) {
			u, err := url.Parse(tc.from)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tc.from, err)
			}
			to := pkgGoDevURL(u)
			if got, want := to.String(), tc.to; got != want {
				t.Errorf("pkgGoDevURL(%q) = %q; want %q", u, got, want)
			}
		})
	}
}
