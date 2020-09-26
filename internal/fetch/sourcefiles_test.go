// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestEncodeDecodeASTFiles(t *testing.T) {
	// Verify that we can encode and decode the Go files in this directory.
	files, fset, err := astFilesForDir(".")
	if err != nil {
		t.Fatal(err)
	}

	data, err := EncodeASTFiles(fset, files)
	if err != nil {
		t.Fatal(err)
	}
	gotFset, gotFiles, err := DecodeASTFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	data2, err := EncodeASTFiles(gotFset, gotFiles)
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

	data, err := EncodeASTFiles(fset, []*ast.File{f})
	if err != nil {
		t.Fatal(err)
	}
	_, files, err := DecodeASTFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	compareObjs(files[0])
}

// Compare the time to decode AST files with and without
// removing parts of the AST not relevant to documentation.
//
// Run on a cloudtop 9/29/2020:
// - data size is 3x smaller
// - decode time is 3.5x faster
func BenchmarkRemovingAST(b *testing.B) {
	files, fset, err := astFilesForDir(".")
	if err != nil {
		b.Fatal(err)
	}

	run := func(name string) {
		data, err := EncodeASTFiles(fset, files)
		if err != nil {
			b.Fatal(err)
		}
		b.Logf("len(data) = %d", len(data))
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := DecodeASTFiles(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	run("not removed")

	for _, f := range files {
		removeUnusedASTNodes(f)
	}
	run("removed")
}

func astFilesForDir(dir string) ([]*ast.File, *token.FileSet, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	var files []*ast.File
	for _, p := range pkgs {
		for _, f := range p.Files {
			files = append(files, f)
		}
	}
	return files, fset, nil
}
