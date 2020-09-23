// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestShortOneLineNode(t *testing.T) {
	src := `
		package insane

		func (p *private) Method1() string { return "" }

		func Foo(ctx Context, s struct {
			Fizz struct {
				Field int
			}
			Buzz interface {
				Method() int
			}
		}) (_ private) {
			return
		}

		func (s *Struct2) Method() {}

		func NewStruct2() *Struct2 {
			return nil
		}

		func NArgs(a, b string) (a, b string) { return }

		func (mx *Mux) Issue41486(fn func(r Router)) Router { return }

		type t struct{}`

	want := []struct {
		result string
		err    bool
	}{
		{result: `(p) Method1()`},
		{result: `Foo(ctx, s)`},
		{result: `(s) Method()`},
		{result: `NewStruct2()`},
		{result: `NArgs(a, b)`},
		{result: `(mx) Issue41486(fn)`},
		{err: true},
	}

	// Parse src but stop after processing the imports.
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &Renderer{fset: fset}
	for i, d := range f.Decls {
		got, err := renderer.ShortSynopsis(d)
		if err != nil && !want[i].err {
			t.Errorf("test %d, ShortSynopsis(): got unexpected error: %v", i, err)
		}
		if err == nil && want[i].err {
			t.Errorf("test %d, ShortSynopsis(): got nil error, want non-nil error", i)
		}
		if got != want[i].result {
			t.Errorf("test %d, ShortSynopsis():\ngot  %s\nwant %s", i, got, want[i].result)
		}
	}
}
