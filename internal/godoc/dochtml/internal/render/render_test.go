// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"

	"golang.org/x/pkgsite/internal/godoc/importer"
)

//lint:file-ignore SA1019 We only need the syntax tree.
// ast.NewPackage is deprecated in favor of go/types, but we don't want or need full
// type information here, just the syntax tree. We are only rendering documentation.

var pkgTime, fsetTime = mustLoadPackage("time")

func mustLoadPackage(path string) (*doc.Package, *token.FileSet) {
	srcName := filepath.Base(path) + ".go"
	code, err := os.ReadFile(filepath.Join("testdata", srcName))
	if err != nil {
		panic(err)
	}

	fset := token.NewFileSet()
	pkgFiles := make(map[string]*ast.File)
	astFile, _ := parser.ParseFile(fset, srcName, code, parser.ParseComments)
	pkgFiles[srcName] = astFile
	astPkg, _ := ast.NewPackage(fset, pkgFiles, importer.SimpleImporter, nil)
	return doc.New(astPkg, path, 0), fset
}
