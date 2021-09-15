// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package search

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestProcessArg(t *testing.T) {
	for _, test := range []struct {
		arg, want string
	}{
		{"$1", "replace($1, '_', '-')"},
		{"to_tsquery('symbols', $3)", "to_tsquery('symbols', replace($3, '_', '-'))"},
		{"foo($10)", "foo(replace($10, '_', '-'))"},
	} {
		got := processArg(test.arg)
		if diff := cmp.Diff(test.want, got); diff != "" {
			t.Errorf("mismatch (-want, +got):\n%s", diff)
		}

	}
}

func TestParseInputType(t *testing.T) {
	for _, test := range []struct {
		name, q string
		want    InputType
	}{
		{"no dot symbol name", "DB", InputTypeNoDot},
		{"one dot symbol name", "DB.Begin", InputTypeOneDot},
		{"one dot package dot symbol name", "sql.DB", InputTypeOneDot},
		{"two dots package name dot symbol name", "sql.DB.Begin", InputTypeTwoDots},
		{"two dots stdlib package path dot symbol name", "database/sql.DB.Begin", InputTypeTwoDots},
		{"multiword two words", "foo bar", InputTypeMultiWord},
		{"multiword three words", "foo bar baz", InputTypeMultiWord},
		{"two dots package path dot symbol name not supported", "github.com/foo/bar.DB", InputTypeNoMatch},
		{"three dots package path dot symbol name not supported", "github.com/foo/bar.DB.Begin", InputTypeNoMatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ParseInputType(test.q)
			if got != test.want {
				t.Errorf("ParseInputType(%q) = %q; want = %q", test.q, got, test.want)
			}
		})
	}
}

// TestGenerateQuery ensure that go generate was run and the generated queries
// are up to date with the raw queries.
func TestGenerateQuery(t *testing.T) {
	for _, test := range []struct {
		name, q, want string
	}{
		{"querySearchSymbol", SymbolQuery(SearchTypeSymbol), querySearchSymbol},
		{"querySearchPackageDotSymbol", SymbolQuery(SearchTypePackageDotSymbol), querySearchPackageDotSymbol},
		{"querySearchMultiWordExact", SymbolQuery(SearchTypeMultiWordExact), querySearchMultiWordExact},
	} {
		t.Run(test.name, func(t *testing.T) {
			if diff := cmp.Diff(test.want, test.q); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
