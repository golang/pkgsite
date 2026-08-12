// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestTypeString(t *testing.T) {
	testCases := []struct {
		expr string
		want string
	}{
		{"int", "int"},
		{"string", "string"},
		{"[]byte", "[]byte"},
		{"*MyType", "*MyType"},
		{"[5]*int", "[5]*int"},
		{"map[string]int", "map[string]int"},
		{"chan int", "chan int"},
		{"<-chan int", "<-chan int"},
		{"func()", "func()"},
		{"func(int, int) bool", "func(int, int) bool"},
		{"func(a, b int) (c bool, _ int)", "func(int, int) (bool, int)"},
	}

	for _, tc := range testCases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parser.ParseExpr(%q): %v", tc.expr, err)
			}
			got := typeString(expr)
			if got != tc.want {
				t.Errorf("typeString(%q) = %q, want %q", tc.expr, got, tc.want)
			}
		})
	}

	t.Run("generic func", func(t *testing.T) {
		testCases := []struct {
			decl string
			want string
		}{
			{
				"func _[T any, U ~int](x int, y U, z T) bool",
				"func[any, ~int](int, #1, #0) bool",
			},
			{
				"func _[T any, U any](m map[T][]U, ch <-chan T, f func(*T) U) (T, *U, error)",
				"func[any, any](map[#0][]#1, <-chan #0, func(*#0) #1) (#0, *#1, error)",
			},
			{
				"func _[T any](s struct{ a T })",
				"func[any](struct{ a #0 })",
			},
			{
				"func _[T any](s struct{ T })",
				"func[any](struct{ #0 })",
			},
		}
		for _, tc := range testCases {
			prog := "package p\n" + tc.decl
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", prog, 0)
			if err != nil {
				t.Fatalf("parser.ParseFile(%q): %v", tc.decl, err)
			}
			fnType := f.Decls[0].(*ast.FuncDecl).Type
			got := typeString(fnType)
			if got != tc.want {
				t.Errorf("typeString() for %q = %q, want %q", tc.decl, got, tc.want)
			}
		}
	})

	if got, want := typeString(nil), "?"; got != want {
		t.Errorf("typeString(nil) = %q, want %q", got, want)
	}
}
