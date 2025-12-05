// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/godoc/codec"
)

func TestRemoveUnusedASTNodes(t *testing.T) {
	const file = `
// Package-level comment.
package p

// const C
const C = 1

// leave unexported consts
const c = 1

// var V
var V int

// leave unexported vars
var v int

// type T
type T int

// leave unexported types
type t int

// Exp is exported.
func Exp() {}

// unexp is not exported, but the comment is preserved for notes.
func unexp() {}

// M is exported.
func (t T) M() int {}

// m isn't, but the comment is preserved for notes.
func (T) m() {}

// U is an exported method of an unexported type.
// Its doc is not shown, unless t is embedded
// in an exported type. We don't remove it to
// be safe.
func (t) U() {}
`
	////////////////
	const want = `// Package-level comment.
package p

// const C
const C = 1

// leave unexported consts
const c = 1

// var V
var V int

// leave unexported vars
var v int

// type T
type T int

// leave unexported types
type t int

// Exp is exported.
func Exp()

// unexp is not exported, but the comment is preserved for notes.

// M is exported.
func (t T) M() int

// m isn't, but the comment is preserved for notes.

// U is an exported method of an unexported type.
// Its doc is not shown, unless t is embedded
// in an exported type. We don't remove it to
// be safe.
func (t) U()
`
	////////////////

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "tst.go", file, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	removeUnusedASTNodes(astFile)
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, astFile); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestDecodeBasicLit(t *testing.T) {
	// byte slice representing an encoded ast.BasicLit from Go 1.25.
	golden := []byte{
		0xf6, 0x1, 0xf7, 0xd, 0x2a, 0x61, 0x73, 0x74, 0x2e, 0x42, 0x61, 0x73, 0x69,
		0x63, 0x4c, 0x69, 0x74, 0xf6, 0x2, 0x0, 0xf4, 0x1, 0xa, 0x2, 0xf7, 0x3,
		0x31, 0x32, 0x33, 0xf3,
	}

	d := codec.NewDecoder(golden)
	val, err := d.Decode()
	if err != nil {
		t.Fatalf("Decode() failed: %v", err)
	}

	got, ok := val.(*ast.BasicLit)
	if !ok {
		t.Fatalf("decoded value is not an ast.BasicLit, got %T", val)
	}

	want := ast.BasicLit{
		Kind:  token.INT,
		Value: "123",
	}

	if got.Kind != want.Kind || got.Value != want.Value {
		t.Errorf("Decode(...) = %+v, want %+v", got, want)
	}
}
