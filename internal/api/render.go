// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/format"
	"go/token"
	"io"
	"slices"
	"strings"
)

// A renderer prints symbol documentation for a package.
// An error that occurs during rendering is saved and returned
// by the end method.
type renderer interface {
	start(*doc.Package)
	end() error
	// startSection start a section, like the one for functions.
	startSection(name string)
	endSection()

	// emit prints documentation for particular node, like a const
	// or function.
	emit(comment string, node ast.Node)
	// TODO(jba): support examples
}

type textRenderer struct {
	fset    *token.FileSet
	w       io.Writer
	pkg     *doc.Package
	parser  *comment.Parser
	printer *comment.Printer
	err     error
}

func (r *textRenderer) start(pkg *doc.Package) {
	r.pkg = pkg
	r.parser = pkg.Parser()
	// Configure the printer for symbol comments here,
	// so we only do it once.
	r.printer = pkg.Printer()
	r.printer.TextPrefix = "\t"
	r.printer.TextCodePrefix = "\t\t"

	r.printf("package %s\n", pkg.Name)
	if pkg.Doc != "" {
		r.printf("\n")
		// The package doc is not indented, so don't use r.printer.
		_, err := r.w.Write(pkg.Text(pkg.Doc))
		if err != nil {
			r.err = err
		}
	}
	r.printf("\n")
}

func (r *textRenderer) end() error { return r.err }

func (r *textRenderer) startSection(name string) {
	r.printf("%s\n\n", strings.ToUpper(name))
}

func (r *textRenderer) endSection() {}

func (r *textRenderer) emit(comment string, node ast.Node) {
	if r.err != nil {
		return
	}
	err := format.Node(r.w, r.fset, node)
	if err != nil {
		r.err = err
		return
	}
	r.printf("\n")
	formatted := r.printer.Text(r.parser.Parse(comment))
	if len(formatted) > 0 {
		_, err = r.w.Write(formatted)
		if err != nil {
			r.err = err
			return
		}
	}
	r.printf("\n")
}

func (r *textRenderer) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	_, err := fmt.Fprintf(r.w, format, args...)
	if err != nil {
		r.err = err
	}
}

// renderDoc renders the documentation for dpkg using the given renderer.
// TODO(jba): support examples.
func renderDoc(dpkg *doc.Package, r renderer) error {
	r.start(dpkg)

	renderValues(dpkg.Consts, r, "constants")
	renderValues(dpkg.Vars, r, "variables")
	renderFuncs(dpkg.Funcs, r, "functions")

	started := false
	for _, t := range dpkg.Types {
		if !ast.IsExported(t.Name) {
			continue
		}
		if !started {
			r.startSection("types")
			started = true
		}
		r.emit(t.Doc, t.Decl)
		renderValues(t.Consts, r, "")
		renderValues(t.Vars, r, "")
		renderFuncs(t.Funcs, r, "")
		renderFuncs(t.Methods, r, "")
	}
	if started {
		r.endSection()
	}
	return r.end()
}

func renderValues(vals []*doc.Value, r renderer, section string) {
	started := false
	for _, v := range vals {
		// Render a group if at least one is exported.
		if slices.IndexFunc(v.Names, ast.IsExported) >= 0 {
			if !started {
				if section != "" {
					r.startSection(section)
				}
				started = true
			}
			r.emit(v.Doc, v.Decl)
		}
	}
	if started && section != "" {
		r.endSection()
	}
}

func renderFuncs(funcs []*doc.Func, r renderer, section string) {
	started := false
	for _, f := range funcs {
		if !ast.IsExported(f.Name) {
			continue
		}
		if !started {
			if section != "" {
				r.startSection(section)
			}
			started = true
		}
		r.emit(f.Doc, f.Decl)
	}
	if started && section != "" {
		r.endSection()
	}
}
