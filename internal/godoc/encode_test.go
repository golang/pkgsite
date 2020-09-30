// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestEncodeDecodeASTFiles(t *testing.T) {
	// Verify that we can encode and decode the Go files in this directory.
	p, err := packageForDir(".", true)
	if err != nil {
		t.Fatal(err)
	}

	data, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	p2, err := DecodePackage(data)
	if err != nil {
		t.Fatal(err)
	}
	data2, err := p2.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, data2) {
		t.Fatal("datas unequal")
	}
}

func TestObjectIdentity(t *testing.T) {
	// Check that encoding and decoding preserves object identity.
	const file = `
package p
var a int
func main() { a = 1 }
`

	compareObjs := func(f *ast.File) {
		t.Helper()
		// We know (from hand-inspecting the output of ast.Fprintf) that these two
		// objects are identical in the above program.
		o1 := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Names[0].Obj
		o2 := f.Decls[1].(*ast.FuncDecl).Body.List[0].(*ast.AssignStmt).Lhs[0].(*ast.Ident).Obj
		if o1 != o2 {
			t.Fatal("objects not identical")
		}
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", file, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	compareObjs(f)

	p := NewPackage(fset)
	p.AddFile(f, false)
	data, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	p, err = DecodePackage(data)
	if err != nil {
		t.Fatal(err)
	}
	compareObjs(p.Files[0].AST)
}

func packageForDir(dir string, removeNodes bool) (*Package, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	p := NewPackage(fset)
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			p.AddFile(f, removeNodes)
		}
	}
	return p, nil
}

// Compare the time to decode AST files with and without
// removing parts of the AST not relevant to documentation.
//
// Run on a cloudtop 9/29/2020:
// - data size is 3.5x smaller
// - decode time is 4.5x faster
func BenchmarkRemovingAST(b *testing.B) {
	for _, removeNodes := range []bool{false, true} {
		b.Run(fmt.Sprintf("removeNodes=%t", removeNodes), func(b *testing.B) {
			p, err := packageForDir(".", removeNodes)
			if err != nil {
				b.Fatal(err)
			}
			data, err := p.Encode()
			if err != nil {
				b.Fatal(err)
			}
			b.Logf("len(data) = %d", len(data))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := DecodePackage(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
