// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"go/parser"
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
		// FIX: remove argument names
		// {"func(a, b int) (c bool)", "func(int, int) bool"},
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

	if got, want := typeString(nil), "?"; got != want {
		t.Errorf("typeString(nil) = %q, want %q", got, want)
	}
}
