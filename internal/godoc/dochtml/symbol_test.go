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
		d := mustLoadPackage("symbols")
	got,
		err := GetSymbols(d,
		fset)
	if err != nil {
		t.Fatal(err)
	}
	want := []*internal.Symbol{
		{
			Name:     "AA",
			Synopsis: "const AA",
			Section:  "Constants",
			Kind:     "Constant",
		},
		{
			Name:     "BB",
			Synopsis: "const BB",
			Section:  "Constants",
			Kind:     "Constant",
		},
		{
			Name:     "CC",
			Synopsis: "const CC",
			Section:  "Constants",
			Kind:     "Constant",
		},
		{
			Name:     "C",
			Synopsis: "const C",
			Section:  "Constants",
			Kind:     "Constant",
		},
		{
			Name:     "ErrA",
			Synopsis: `var ErrA = errors.New("error A")`,
			Section:  "Variables",
			Kind:     "Variable",
		},
		{
			Name:     "ErrB",
			Synopsis: `var ErrB = errors.New("error B")`,
			Section:  "Variables",
			Kind:     "Variable",
		},
		{
			Name:     "A",
			Synopsis: "var A string",
			Section:  "Variables",
			Kind:     "Variable",
		},

		{
			Name:     "B",
			Synopsis: "var B string",
			Section:  "Variables",
			Kind:     "Variable",
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
					Synopsis:   "type I1 interface, M1 func()",
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
					Synopsis:   "type I2 interface, M2 func()",
					Section:    "Types",
					ParentName: "I2",
					Kind:       "Method",
				},
			},
		},
		{
			Name:     "Num",
			Synopsis: "type Num int",
			Section:  "Types",
			Kind:     "Type",
			Children: []*internal.Symbol{
				{
					Name:       "DD",
					Synopsis:   "const DD",
					Section:    "Types",
					Kind:       "Constant",
					ParentName: "Num",
				},
				{
					Name:       "EE",
					Synopsis:   "const EE",
					Section:    "Types",
					Kind:       "Constant",
					ParentName: "Num",
				},
				{
					Name:       "FF",
					Synopsis:   "const FF",
					Section:    "Types",
					Kind:       "Constant",
					ParentName: "Num",
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
					Synopsis:   "type S1 struct, F int",
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
					Synopsis:   "type S2 struct, G int",
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
					Synopsis:   "const CT",
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
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}
