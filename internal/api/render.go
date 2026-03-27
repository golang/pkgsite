// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO(jba)? Consider rendering notes separately. Now they appear
// with the doc for each symbol, which is probably better for LLMs,
// but be open to evidence to the contrary.

package api

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/format"
	"go/token"
	"html"
	"io"
	"slices"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// A renderer prints symbol documentation for a package.
// An error that occurs during rendering is saved and returned
// by the end method.
type renderer interface {
	start(*doc.Package)
	end() error
	// startSection starts a section, like the one for functions.
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

// TODO(jba): consolidate this function to avoid duplication.
func (r *textRenderer) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	_, err := fmt.Fprintf(r.w, format, args...)
	if err != nil {
		r.err = err
	}
}

type markdownRenderer struct {
	fset    *token.FileSet
	w       io.Writer
	pkg     *doc.Package
	parser  *comment.Parser
	printer *comment.Printer
	caser   cases.Caser
	err     error
}

func (r *markdownRenderer) start(pkg *doc.Package) {
	r.pkg = pkg
	r.parser = pkg.Parser()
	r.printer = pkg.Printer()
	r.printer.HeadingLevel = 3
	r.caser = cases.Title(language.English)

	r.printf("# package %s\n", pkg.Name)
	if pkg.Doc != "" {
		r.printf("\n")
		_, err := r.w.Write(r.printer.Markdown(r.parser.Parse(pkg.Doc)))
		if err != nil {
			r.err = err
		}
	}
	r.printf("\n")
}

func (r *markdownRenderer) end() error { return r.err }

func (r *markdownRenderer) startSection(name string) {
	if name == "" {
		return
	}
	r.printf("## %s\n\n", r.caser.String(name))
}

func (r *markdownRenderer) endSection() {}

func (r *markdownRenderer) emit(comment string, node ast.Node) {
	if r.err != nil {
		return
	}
	r.printf("```\n")
	err := format.Node(r.w, r.fset, node)
	if err != nil {
		r.err = err
		return
	}
	r.printf("\n```\n")
	formatted := r.printer.Markdown(r.parser.Parse(comment))
	if len(formatted) > 0 {
		_, err = r.w.Write(formatted)
		if err != nil {
			r.err = err
			return
		}
	}
	r.printf("\n")
}

func (r *markdownRenderer) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	_, err := fmt.Fprintf(r.w, format, args...)
	if err != nil {
		r.err = err
	}
}

type htmlRenderer struct {
	fset    *token.FileSet
	w       io.Writer
	pkg     *doc.Package
	parser  *comment.Parser
	printer *comment.Printer
	caser   cases.Caser
	err     error
}

func (r *htmlRenderer) start(pkg *doc.Package) {
	r.pkg = pkg
	r.parser = pkg.Parser()
	r.printer = pkg.Printer()
	r.printer.HeadingLevel = 3
	r.caser = cases.Title(language.English)

	r.printf("<h1>package %s</h1>\n", pkg.Name)
	if pkg.Doc != "" {
		r.printf("\n")
		_, err := r.w.Write(r.printer.HTML(r.parser.Parse(pkg.Doc)))
		if err != nil {
			r.err = err
		}
	}
	r.printf("\n")
}

func (r *htmlRenderer) end() error { return r.err }

func (r *htmlRenderer) startSection(name string) {
	if name == "" {
		return
	}
	r.printf("<h2>%s</h2>\n\n", r.caser.String(name))
}

func (r *htmlRenderer) endSection() {}

func (r *htmlRenderer) emit(comment string, node ast.Node) {
	if r.err != nil {
		return
	}
	var buf strings.Builder
	err := format.Node(&buf, r.fset, node)
	if err != nil {
		r.err = err
		return
	}
	r.printf("<pre><code>%s</code></pre>\n", html.EscapeString(buf.String()))
	formatted := r.printer.HTML(r.parser.Parse(comment))
	if len(formatted) > 0 {
		_, err = r.w.Write(formatted)
		if err != nil {
			r.err = err
			return
		}
	}
	r.printf("\n")
}

func (r *htmlRenderer) printf(format string, args ...any) {
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
