// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseParams(t *testing.T) {
	type Nested struct {
		ListParams
	}
	type DeepNested struct {
		Nested
	}
	type EmbeddedPtr struct {
		*ListParams
	}
	type extraParams struct {
		Int64    int64     `form:"i64"`
		Slice    []string  `form:"slice"`
		PtrInt   *int      `form:"ptr"`
		PtrPtr   **int     `form:"ptrptr"`
		PtrSlice []*string `form:"ptrslice"`
		private  int       `form:"private"` // Should be ignored
	}
	type overflowParams struct {
		Int8 int8 `form:"i8"`
	}
	type unsupportedParams struct {
		Float float64 `form:"float"`
	}

	for _, test := range []struct {
		name    string
		values  url.Values
		dst     any
		want    any
		wantErr bool
	}{
		{
			name:   "PackageParams",
			values: url.Values{"module": {"m"}, "version": {"v1.0.0"}, "goos": {"linux"}, "imports": {"true"}},
			dst:    &PackageParams{},
			want: &PackageParams{
				Module:  "m",
				Version: "v1.0.0",
				GOOS:    "linux",
				Imports: true,
			},
		},
		{
			name:   "Boolean presence",
			values: url.Values{"imports": {"true"}, "licenses": {"1"}},
			dst:    &PackageParams{},
			want: &PackageParams{
				Imports:  true,
				Licenses: true,
			},
		},
		{
			name:   "Boolean presence (ModuleParams)",
			values: url.Values{"licenses": {"0"}, "readme": {"false"}},
			dst:    &ModuleParams{},
			want: &ModuleParams{
				Licenses: false,
				Readme:   false,
			},
		},
		{
			name:   "Empty bool",
			values: url.Values{"imports": {""}},
			dst:    &PackageParams{},
			want: &PackageParams{
				Imports: false,
			},
		},
		{
			name:    "Invalid bool (on)",
			values:  url.Values{"imports": {"on"}},
			dst:     &PackageParams{},
			wantErr: true,
		},
		{
			name:   "Deeply nested embedding",
			values: url.Values{"limit": {"100"}},
			dst:    &DeepNested{},
			want: &DeepNested{
				Nested: Nested{
					ListParams: ListParams{Limit: 100},
				},
			},
		},
		{
			name: "Extra types (int64, slice, ptr, ptrptr, ptrslice)",
			values: url.Values{
				"i64":      {"9223372036854775807"},
				"slice":    {"a", "b"},
				"ptr":      {"42"},
				"ptrptr":   {"84"},
				"ptrslice": {"one", "two"},
				"private":  {"1"},
			},
			dst: &extraParams{},
			want: &extraParams{
				Int64:    9223372036854775807,
				Slice:    []string{"a", "b"},
				PtrInt:   intPtr(42),
				PtrPtr:   intPtrPtr(84),
				PtrSlice: []*string{stringPtr("one"), stringPtr("two")},
				private:  0, // Ignored
			},
		},
		{
			name:    "Malformed int",
			values:  url.Values{"limit": {"10.5"}},
			dst:     &SymbolsParams{},
			wantErr: true,
		},
		{
			name:    "Malformed bool",
			values:  url.Values{"imports": {"maybe"}},
			dst:     &PackageParams{},
			wantErr: true,
		},
		{
			name:    "Empty int",
			values:  url.Values{"limit": {""}},
			dst:     &SymbolsParams{},
			wantErr: true,
		},
		{
			name:    "Not a pointer",
			values:  url.Values{},
			dst:     PackageParams{},
			wantErr: true,
		},
		{
			name:    "Int8 overflow",
			values:  url.Values{"i8": {"128"}},
			dst:     &overflowParams{},
			wantErr: true,
		},
		{
			name:    "Unsupported type",
			values:  url.Values{"float": {"1.2"}},
			dst:     &unsupportedParams{},
			wantErr: true,
		},
		{
			name:   "Embedded pointer",
			values: url.Values{"limit": {"50"}},
			dst:    &EmbeddedPtr{},
			want: &EmbeddedPtr{
				ListParams: &ListParams{Limit: 50},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ParseParams(test.values, test.dst)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseParams() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if diff := cmp.Diff(test.want, test.dst, cmp.AllowUnexported(extraParams{})); diff != "" {
				t.Errorf("ParseParams() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func intPtr(i int) *int { return &i }
func intPtrPtr(i int) **int {
	p := intPtr(i)
	return &p
}
func stringPtr(s string) *string { return &s }
