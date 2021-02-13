// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
)

func TestGetSymbols(t *testing.T) {
	LoadTemplates(templateSource)
	fset,
		d := mustLoadPackage("everydecl")
	got,
		err := GetSymbols(d,
		fset)
	if err != nil {
		t.Fatal(err)
	}
	want := []*internal.Symbol{
		{
			Name:     "C",
			Synopsis: "const C = 1",
			Section:  "Constants",
			Kind:     "Constant",
		},
		{
			Name:     "V",
			Synopsis: "var V = 2",
			Section:  "Variables",
			Kind:     "Variable",
		},
		{
			Name:     "F",
			Synopsis: "func F()",
			Section:  "Functions",
			Kind:     "Function",
		},
		{
			Name:     "A",
			Synopsis: "type A int",
			Section:  "Types",
			Kind:     "Type",
		},
		{
			Name:     "B",
			Synopsis: "type B bool",
			Section:  "Types",
			Kind:     "Type",
		},
		{
			Name:     "I1",
			Synopsis: "type I1 interface{ ... }",
			Section:  "Types",
			Kind:     "Type",
			Children: []*internal.Symbol{
				{
					Name:       "I1.M1",
					Synopsis:   "M1",
					Section:    "Types",
					ParentName: "I1",
					Kind:       "Method",
				},
			},
		},
		{
			Name:     "I2",
			Synopsis: "type I2 interface{ ... }",
			Section:  "Types",
			Kind:     "Type",
			Children: []*internal.Symbol{
				{
					Name:       "I2.M2",
					Synopsis:   "M2",
					Section:    "Types",
					ParentName: "I2",
					Kind:       "Method",
				},
			},
		},
		{
			Name:     "S1",
			Synopsis: "type S1 struct{ ... }",
			Section:  "Types",
			Kind:     "Type",
			Children: []*internal.Symbol{
				{
					Name:       "S1.F",
					Synopsis:   "F",
					Section:    "Types",
					ParentName: "S1",
					Kind:       "Field",
				},
			},
		},
		{
			Name:     "S2",
			Synopsis: "type S2 struct{ ... }",
			Section:  "Types",
			Kind:     "Type",
			Children: []*internal.Symbol{
				{
					Name:       "S2.G",
					Synopsis:   "G",
					Section:    "Types",
					ParentName: "S2",
					Kind:       "Field",
				},
			},
		},
		{
			Name:     "T",
			Synopsis: "type T int",
			Section:  "Types",
			Kind:     "Type",
			Children: []*internal.Symbol{
				{
					Name:       "CT",
					Synopsis:   "const CT T = 3",
					Section:    "Types",
					ParentName: "T",
					Kind:       "Constant",
				},
				{
					Name:       "VT",
					Synopsis:   "var VT T",
					Section:    "Types",
					ParentName: "T",
					Kind:       "Variable",
				},
				{
					Name:       "TF",
					Synopsis:   "func TF() T",
					Section:    "Types",
					ParentName: "T",
					Kind:       "Function",
				},
				{
					Name:       "T.M",
					Synopsis:   "func (T) M()",
					Section:    "Types",
					ParentName: "T",
					Kind:       "Method",
				},
			},
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatal(diff)
	}
}
