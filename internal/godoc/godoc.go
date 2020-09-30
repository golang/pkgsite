// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package godoc is for rendering Go documentation.
package godoc

import (
	"go/ast"
	"go/token"

	"golang.org/x/pkgsite/internal/godoc/dochtml"
)

var ErrTooLarge = dochtml.ErrTooLarge

type ModuleInfo = dochtml.ModuleInfo

// A package contains everything needed to render Go documentation for a package.
type Package struct {
	Fset *token.FileSet
	gobPackage
}

type gobPackage struct { // fields that can be directly gob-encoded
	Files              []*File
	ModulePackagePaths map[string]bool
}

// A File contains everything needed about a source file to render documentation.
type File struct {
	Name string // full file pathname relative to zip content directory
	AST  *ast.File
}

// NewPackage returns a new Package with the given fset and set of module package paths.
func NewPackage(fset *token.FileSet, modPaths map[string]bool) *Package {
	return &Package{
		Fset: fset,
		gobPackage: gobPackage{
			ModulePackagePaths: modPaths,
		},
	}
}

// AddFile adds a file to the Package. After it returns, the contents of the ast.File
// are unsuitable for anything other than the methods of this package.
func (p *Package) AddFile(f *ast.File, removeNodes bool) {
	if removeNodes {
		removeUnusedASTNodes(f)
	}
	removeCycles(f)
	p.Files = append(p.Files, &File{
		Name: p.Fset.Position(f.Package).Filename,
		AST:  f,
	})
}

// removeUnusedASTNodes removes parts of the AST not needed for documentation.
// It doesn't remove unexported consts, vars or types, although it probably could.
func removeUnusedASTNodes(pf *ast.File) {
	var decls []ast.Decl
	for _, d := range pf.Decls {
		if f, ok := d.(*ast.FuncDecl); ok {
			// Remove all unexported functions and function bodies.
			if f.Name == nil || !ast.IsExported(f.Name.Name) {
				continue
			}
			f.Body = nil
		}
		decls = append(decls, d)
	}
	pf.Comments = nil
	pf.Decls = decls
}
