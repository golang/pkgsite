// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"go/parser"
	"testing"
)

type otherType struct{}

func (otherType) change(syntaxType) changeKind { return changeOther }

func TestSimpleType(t *testing.T) {
	parse := func(src string) *simpleType {
		t.Helper()
		expr, err := parser.ParseExpr(src)
		if err != nil {
			t.Fatalf("parser.ParseExpr(%q): %v", src, err)
		}
		return newSimpleType(expr)
	}

	testCases := []struct {
		name string
		old  *simpleType
		new  *simpleType
		want changeKind
	}{
		{
			name: "same",
			old:  parse("int"),
			new:  parse("int"),
			want: changeOther,
		},
		{
			name: "different",
			old:  parse("int"),
			new:  parse("string"),
			want: changeBreaking,
		},
		{
			name: "both empty",
			old:  &simpleType{},
			new:  &simpleType{},
			want: changeOther,
		},
	}

	t.Run("change", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				got := tc.old.change(tc.new)
				if got != tc.want {
					t.Errorf("%s: change = %v, want %v", tc.name, got, tc.want)
				}
			})
		}

		// non-*simpleType should return changeBreaking
		ot := &otherType{}
		s := parse("int")
		if got := s.change(ot); got != changeBreaking {
			t.Errorf("expected s.change(otherType) to be changeBreaking, got %v", got)
		}
	})
}

func TestNewSimpleType(t *testing.T) {
	expr, err := parser.ParseExpr("int")
	if err != nil {
		t.Fatalf("parser.ParseExpr: %v", err)
	}
	st := newSimpleType(expr)
	if got, want := st.typeString, "int"; got != want {
		t.Errorf("newSimpleType: got %q, want %q", got, want)
	}
}
