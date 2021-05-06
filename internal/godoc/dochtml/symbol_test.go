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
			SymbolMeta: internal.SymbolMeta{
				Name:     "AA",
				Synopsis: "const AA",
				Section:  "Constants",
				Kind:     "Constant",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "BB",
				Synopsis: "const BB",
				Section:  "Constants",
				Kind:     "Constant",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "CC",
				Synopsis: "const CC",
				Section:  "Constants",
				Kind:     "Constant",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "C",
				Synopsis: "const C",
				Section:  "Constants",
				Kind:     "Constant",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "ErrA",
				Synopsis: `var ErrA = errors.New("error A")`,
				Section:  "Variables",
				Kind:     "Variable",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "ErrB",
				Synopsis: `var ErrB = errors.New("error B")`,
				Section:  "Variables",
				Kind:     "Variable",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "A",
				Synopsis: "var A string",
				Section:  "Variables",
				Kind:     "Variable",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "B",
				Synopsis: "var B string",
				Section:  "Variables",
				Kind:     "Variable",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "V",
				Synopsis: "var V = 2",
				Section:  "Variables",
				Kind:     "Variable",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "F",
				Synopsis: "func F()",
				Section:  "Functions",
				Kind:     "Function",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "A",
				Synopsis: "type A int",
				Section:  "Types",
				Kind:     "Type",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "B",
				Synopsis: "type B bool",
				Section:  "Types",
				Kind:     "Type",
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "I1",
				Synopsis: "type I1 interface",
				Section:  "Types",
				Kind:     "Type",
			},
			Children: []*internal.SymbolMeta{
				{
					Name:       "I1.M1",
					Synopsis:   "M1 func()",
					Section:    "Types",
					ParentName: "I1",
					Kind:       "Method",
				},
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "I2",
				Synopsis: "type I2 interface",
				Section:  "Types",
				Kind:     "Type",
			},
			Children: []*internal.SymbolMeta{
				{
					Name:       "I2.M2",
					Synopsis:   "M2 func()",
					Section:    "Types",
					ParentName: "I2",
					Kind:       "Method",
				},
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "Num",
				Synopsis: "type Num int",
				Section:  "Types",
				Kind:     "Type",
			},
			Children: []*internal.SymbolMeta{
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
			SymbolMeta: internal.SymbolMeta{
				Name:     "S1",
				Synopsis: "type S1 struct",
				Section:  "Types",
				Kind:     "Type",
			},
			Children: []*internal.SymbolMeta{
				{
					Name:       "S1.F",
					Synopsis:   "F int",
					Section:    "Types",
					ParentName: "S1",
					Kind:       "Field",
				},
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "S2",
				Synopsis: "type S2 struct",
				Section:  "Types",
				Kind:     "Type",
			},
			Children: []*internal.SymbolMeta{
				{
					Name:       "S2.G",
					Synopsis:   "G int",
					Section:    "Types",
					ParentName: "S2",
					Kind:       "Field",
				},
			},
		},
		{
			SymbolMeta: internal.SymbolMeta{
				Name:     "T",
				Synopsis: "type T int",
				Section:  "Types",
				Kind:     "Type",
			},
			Children: []*internal.SymbolMeta{
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
