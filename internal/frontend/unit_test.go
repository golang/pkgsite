// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/sample"
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
		got := constructUnitURL(test.path, test.modpath, test.version)
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
		got := canonicalURLPath(test.path, test.modpath, test.version, test.version)
		if got != test.want {
			t.Errorf("canonicalURLPath(%q, %q, %q) = %q, want %q", test.path, test.modpath, test.version, got, test.want)
		}
	}
}

func TestIsValidTab(t *testing.T) {
	testTabs := []string{
		tabMain,
		tabVersions,
		tabImports,
		tabImportedBy,
		tabLicenses,
	}
	for _, test := range []struct {
		name     string
		um       *internal.UnitMeta
		wantTabs []string
	}{
		{
			name:     "module",
			um:       sample.UnitMeta(sample.ModulePath, sample.ModulePath, sample.VersionString, "", true),
			wantTabs: []string{tabMain, tabVersions, tabLicenses},
		},
		{
			name:     "directory",
			um:       sample.UnitMeta(sample.ModulePath+"/go", sample.ModulePath, sample.VersionString, "", true),
			wantTabs: []string{tabMain, tabVersions, tabLicenses},
		},
		{
			name:     "package",
			um:       sample.UnitMeta(sample.ModulePath+"/go/packages", sample.ModulePath, sample.VersionString, "packages", true),
			wantTabs: []string{tabMain, tabVersions, tabImports, tabImportedBy, tabLicenses},
		},
		{
			name:     "command",
			um:       sample.UnitMeta(sample.ModulePath+"/cmd", sample.ModulePath, sample.VersionString, "main", true),
			wantTabs: []string{tabMain, tabVersions, tabImports, tabImportedBy, tabLicenses},
		},
		{
			name:     "non-redist pkg",
			um:       sample.UnitMeta(sample.ModulePath+"/go/packages", sample.ModulePath, sample.VersionString, "packages", false),
			wantTabs: []string{tabMain, tabVersions, tabImports, tabImportedBy},
		},
	} {
		validTabs := map[string]bool{}
		for _, w := range test.wantTabs {
			validTabs[w] = true
		}
		for _, tab := range testTabs {
			t.Run(test.name, func(t *testing.T) {
				got := isValidTabForUnit(tab, test.um)
				_, want := validTabs[tab]
				if got != want {
					t.Errorf("mismatch for %q on tab %q: got %t; want %t", test.um.Path, tab, got, want)
				}
			})
		}
	}
}

func TestMetaDescription(t *testing.T) {
	for _, test := range []struct {
		synopsis, want string
	}{
		{
			synopsis: "",
			want:     "",
		},
		{
			synopsis: "Hello, world.",
			want:     `<meta name="Description" content="Hello, world.">`,
		},
		{
			synopsis: `"><script>alert();</script><br`,
			want:     `<meta name="Description" content="&#34;&gt;&lt;script&gt;alert();&lt;/script&gt;&lt;br">`,
		},
	} {
		got := metaDescription(test.synopsis).String()
		if got != test.want {
			t.Errorf("metaDescription(%q) = %q, want %q", test.synopsis, got, test.want)
		}
	}
}
