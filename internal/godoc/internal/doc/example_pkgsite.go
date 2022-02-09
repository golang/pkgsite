// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Extract example functions from file ASTs.

package doc

import (
	"go/ast"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Examples returns the examples found in testFiles, sorted by Name field.
// The Order fields record the order in which the examples were encountered.
// The Suffix field is not populated when Examples is called directly, it is
// only populated by NewFromFiles for examples it finds in _test.go files.
//
// Playable Examples must be in a package whose name ends in "_test".
// An Example is "playable" (the Play field is non-nil) in either of these
// circumstances:
//   - The example function is self-contained: the function references only
//     identifiers from other packages (or predeclared identifiers, such as
//     "int") and the test file does not include a dot import.
//   - The entire test file is the example: the file contains exactly one
//     example function, zero test, fuzz test, or benchmark function, and at
//     least one top-level function, type, variable, or constant declaration
//     other than the example function.
func Examples2(fset *token.FileSet, testFiles ...*ast.File) []*Example {
	var list []*Example
	for _, file := range testFiles {
		hasTests := false // file contains tests, fuzz test, or benchmarks
		numDecl := 0      // number of non-import declarations in the file
		var flist []*Example
		for _, decl := range file.Decls {
			if g, ok := decl.(*ast.GenDecl); ok && g.Tok != token.IMPORT {
				numDecl++
				continue
			}
			f, ok := decl.(*ast.FuncDecl)
			if !ok || f.Recv != nil {
				continue
			}
			numDecl++
			name := f.Name.Name
			if isTest(name, "Test") || isTest(name, "Benchmark") || isTest(name, "Fuzz") {
				hasTests = true
				continue
			}
			if !isTest(name, "Example") {
				continue
			}
			if params := f.Type.Params; len(params.List) != 0 {
				continue // function has params; not a valid example
			}
			if f.Body == nil { // ast.File.Body nil dereference (see issue 28044)
				continue
			}
			var doc string
			if f.Doc != nil {
				doc = f.Doc.Text()
			}
			output, unordered, hasOutput := exampleOutput(f.Body, file.Comments)
			flist = append(flist, &Example{
				Name:        name[len("Example"):],
				Doc:         doc,
				Code:        f.Body,
				Play:        playExample2(fset, file, f),
				Comments:    file.Comments,
				Output:      output,
				Unordered:   unordered,
				EmptyOutput: output == "" && hasOutput,
				Order:       len(flist),
			})
		}
		if !hasTests && numDecl > 1 && len(flist) == 1 {
			// If this file only has one example function, some
			// other top-level declarations, and no tests or
			// benchmarks, use the whole file as the example.
			flist[0].Code = file
			flist[0].Play = playExampleFile(file)
		}
		list = append(list, flist...)
	}
	// sort by name
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// playExample synthesizes a new *ast.File based on the provided
// file with the provided function body as the body of main.
func playExample2(fset *token.FileSet, file *ast.File, f *ast.FuncDecl) *ast.File {
	body := f.Body
	tokenFile := fset.File(file.Package)
	if !strings.HasSuffix(file.Name.Name, "_test") {
		// We don't support examples that are part of the
		// greater package (yet).
		return nil
	}

	// Collect top-level declarations in the file.
	topDecls := make(map[*ast.Object]ast.Decl)
	typMethods := make(map[string][]ast.Decl)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				topDecls[d.Name.Obj] = d
			} else {
				if len(d.Recv.List) == 1 {
					t := d.Recv.List[0].Type
					tname, _ := baseTypeName(t)
					typMethods[tname] = append(typMethods[tname], d)
				}
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					topDecls[s.Name.Obj] = d
				case *ast.ValueSpec:
					for _, name := range s.Names {
						topDecls[name.Obj] = d
					}
				}
			}
		}
	}

	// Find unresolved identifiers and uses of top-level declarations.
	depDecls, unresolved := findDeclsAndUnresolved(body, topDecls, typMethods)

	// Remove predeclared identifiers from unresolved list.
	for n := range unresolved {
		if predeclaredTypes[n] || predeclaredConstants[n] || predeclaredFuncs[n] {
			delete(unresolved, n)
		}
	}

	// Use unresolved identifiers to determine the imports used by this
	// example. The heuristic assumes package names match base import
	// paths for imports w/o renames (should be good enough most of the time).
	namedImports := make(map[string]string) // [name]path
	var blankImports []ast.Spec             // _ imports
	for _, s := range file.Imports {
		p, err := strconv.Unquote(s.Path.Value)
		if err != nil {
			continue
		}
		if p == "syscall/js" {
			// We don't support examples that import syscall/js,
			// because the package syscall/js is not available in the playground.
			return nil
		}
		n := path.Base(p)
		if s.Name != nil {
			n = s.Name.Name
			switch n {
			case "_":
				blankImports = append(blankImports, s)
				continue
			case ".":
				// We can't resolve dot imports (yet).
				return nil
			}
		}
		if unresolved[n] {
			namedImports[n] = p
			delete(unresolved, n)
		}
	}

	// If there are other unresolved identifiers, give up because this
	// synthesized file is not going to build.
	if len(unresolved) > 0 {
		return nil
	}

	// Include documentation belonging to blank imports.
	var comments []*ast.CommentGroup
	for _, s := range blankImports {
		if c := s.(*ast.ImportSpec).Doc; c != nil {
			comments = append(comments, c)
		}
	}

	// Include comments that are inside the function body.
	for _, c := range file.Comments {
		if body.Pos() <= c.Pos() && c.End() <= body.End() {
			comments = append(comments, c)
		}
	}

	// Strip the "Output:" or "Unordered output:" comment and adjust body
	// end position.
	body, comments = stripOutputComment(body, comments)

	// Include documentation belonging to dependent declarations.
	for _, d := range depDecls {
		switch d := d.(type) {
		case *ast.GenDecl:
			if d.Doc != nil {
				comments = append(comments, d.Doc)
			}
		case *ast.FuncDecl:
			if d.Doc != nil {
				comments = append(comments, d.Doc)
			}
		}
	}

	importDecl := synthesizeImportDecl(namedImports, blankImports, tokenFile)

	// Synthesize main function.
	funcDecl := &ast.FuncDecl{
		Name: ast.NewIdent("main"),
		Type: f.Type,
		Body: body,
	}

	decls := make([]ast.Decl, 0, 2+len(depDecls))
	decls = append(decls, importDecl)
	decls = append(decls, depDecls...)
	decls = append(decls, funcDecl)

	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Pos() < decls[j].Pos()
	})

	sort.Slice(comments, func(i, j int) bool {
		return comments[i].Pos() < comments[j].Pos()
	})

	// Synthesize file.
	return &ast.File{
		Name:     ast.NewIdent("main"),
		Decls:    decls,
		Comments: comments,
	}
}

func findDeclsAndUnresolved(body ast.Node, topDecls map[*ast.Object]ast.Decl, typMethods map[string][]ast.Decl) ([]ast.Decl, map[string]bool) {
	var depDecls []ast.Decl
	unresolved := make(map[string]bool)
	hasDepDecls := make(map[ast.Decl]bool)
	objs := map[*ast.Object]bool{}

	var inspectFunc func(ast.Node) bool
	inspectFunc = func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.Ident:
			if e.Obj == nil && e.Name != "_" {
				unresolved[e.Name] = true
			} else if d := topDecls[e.Obj]; d != nil {
				objs[e.Obj] = true
				if !hasDepDecls[d] {
					hasDepDecls[d] = true
					depDecls = append(depDecls, d)
				}
			}
			return true
		case *ast.SelectorExpr:
			// For selector expressions, only inspect the left hand side.
			// (For an expression like fmt.Println, only add "fmt" to the
			// set of unresolved names, not "Println".)
			ast.Inspect(e.X, inspectFunc)
			return false
		case *ast.KeyValueExpr:
			// For key value expressions, only inspect the value
			// as the key should be resolved by the type of the
			// composite literal.
			ast.Inspect(e.Value, inspectFunc)
			return false
		}
		return true
	}
	ast.Inspect(body, inspectFunc)
	for i := 0; i < len(depDecls); i++ {
		switch d := depDecls[i].(type) {
		case *ast.FuncDecl:
			// Inspect types of parameters and results. See #28492.
			if d.Type.Params != nil {
				for _, p := range d.Type.Params.List {
					ast.Inspect(p.Type, inspectFunc)
				}
			}
			if d.Type.Results != nil {
				for _, r := range d.Type.Results.List {
					ast.Inspect(r.Type, inspectFunc)
				}
			}

			ast.Inspect(d.Body, inspectFunc)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					ast.Inspect(s.Type, inspectFunc)
					depDecls = append(depDecls, typMethods[s.Name.Name]...)
				case *ast.ValueSpec:
					if s.Type != nil {
						ast.Inspect(s.Type, inspectFunc)
					}
					for _, val := range s.Values {
						ast.Inspect(val, inspectFunc)
					}
				}
			}
		}
	}
	// Some decls include multiple specs, such as a variable declaration with
	// multiple variables on the same line, or a parenthesized declaration. Trim
	// the declarations to include only the specs that are actually mentioned.
	// However, if there is a constant group with iota, leave it all: later
	// constant declarations in the group may have no value and so cannot stand
	// on their own, and furthermore, removing any constant from the group could
	// change the values of subsequent ones.
	// See testdata/examples/iota.go for a minimal example.
	ds := depDecls[:0]
	for _, d := range depDecls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			ds = append(ds, d)
		case *ast.GenDecl:
			// Collect all Specs that were mentioned in the example.
			var specs []ast.Spec
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					if objs[s.Name.Obj] {
						specs = append(specs, s)
					}
				case *ast.ValueSpec:
					// A ValueSpec may have multiple names (e.g. "var a, b int").
					// Keep only the names that were mentioned in the example.
					// Exception: the multiple names have a single initializer (which
					// would be a function call with multiple return values). In that
					// case, keep everything.
					if len(s.Names) > 1 && len(s.Values) == 1 {
						specs = append(specs, s)
						continue
					}
					ns := *s
					ns.Names = nil
					ns.Values = nil
					for i, n := range s.Names {
						if objs[n.Obj] {
							ns.Names = append(ns.Names, n)
							if s.Values != nil {
								ns.Values = append(ns.Values, s.Values[i])
							}
						}
					}
					if len(ns.Names) > 0 {
						specs = append(specs, &ns)
					}
				}
			}
			if len(specs) > 0 {
				// Constant with iota? Keep it all.
				if d.Tok == token.CONST && hasIota(d.Specs[0]) {
					ds = append(ds, d)
				} else {
					// Synthesize a GenDecl with just the Specs we need.
					nd := *d // copy the GenDecl
					nd.Specs = specs
					if len(specs) == 1 {
						// Remove grouping parens if there is only one spec.
						nd.Lparen = 0
					}
					ds = append(ds, &nd)
				}
			}
		}
	}
	return ds, unresolved
}

func hasIota(s ast.Spec) bool {
	has := false
	ast.Inspect(s, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "iota" {
			has = true
			return false
		}
		return true
	})
	return has
}

// synthesizeImportDecl creates the imports for the example. We want the imports
// divided into two groups, one for the standard library and one for all others.
// To get ast.SortImports (called by the formatter) to do that, we must assign
// file positions to the import specs so that there is a blank line between the
// two groups. The exact positions don't matter, and they don't have to be
// distinct within a group; ast.SortImports just looks for a gap of more than
// one line between specs.
func synthesizeImportDecl(namedImports map[string]string, blankImports []ast.Spec, tfile *token.File) *ast.GenDecl {
	importDecl := &ast.GenDecl{
		Tok:    token.IMPORT,
		Lparen: 1, // Need non-zero Lparen and Rparen so that printer
		Rparen: 1, // treats this as a factored import.
	}
	var stds, others []ast.Spec
	var stdPos, otherPos token.Pos
	if tfile.LineCount() >= 3 {
		stdPos = tfile.LineStart(1)
		otherPos = tfile.LineStart(3)
	}
	for n, p := range namedImports {
		var (
			pos   token.Pos
			specs *[]ast.Spec
		)
		if !strings.ContainsRune(p, '.') {
			pos = stdPos
			specs = &stds
		} else {
			pos = otherPos
			specs = &others
		}
		s := &ast.ImportSpec{
			Path:   &ast.BasicLit{Value: strconv.Quote(p), Kind: token.STRING, ValuePos: pos},
			EndPos: pos,
		}
		if path.Base(p) != n {
			s.Name = ast.NewIdent(n)
			s.Name.NamePos = pos
		}
		*specs = append(*specs, s)
	}
	importDecl.Specs = append(stds, others...)
	importDecl.Specs = append(importDecl.Specs, blankImports...)

	return importDecl
}
