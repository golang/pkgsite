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
	"strings"
)

var pkgTime, fsetTime = mustLoadPackage("time")

func mustLoadPackage(path string) (*doc.Package, *token.FileSet) {
	// simpleImporter is used by ast.NewPackage.
	simpleImporter := func(imports map[string]*ast.Object, pkgPath string) (*ast.Object, error) {
		pkg := imports[pkgPath]
		if pkg == nil {
			pkgName := pkgPath[strings.LastIndex(pkgPath, "/")+1:]
			pkg = ast.NewObj(ast.Pkg, pkgName)
			pkg.Data = ast.NewScope(nil) // required for or dot-imports
			imports[pkgPath] = pkg
		}
		return pkg, nil
	}

	srcName := filepath.Base(path) + ".go"
	code, err := os.ReadFile(filepath.Join("testdata", srcName))
	if err != nil {
		panic(err)
	}

	fset := token.NewFileSet()
	pkgFiles := make(map[string]*ast.File)
	astFile, _ := parser.ParseFile(fset, srcName, code, parser.ParseComments)
	pkgFiles[srcName] = astFile
	astPkg, _ := ast.NewPackage(fset, pkgFiles, simpleImporter, nil)
	return doc.New(astPkg, path, 0), fset
}
