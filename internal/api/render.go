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

	// emitExample prints an example.
	emitExample(ex *doc.Example)
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
		if _, err := r.w.Write(pkg.Text(pkg.Doc)); err != nil {
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
		if _, err = r.w.Write(formatted); err != nil {
			r.err = err
			return
		}
	}
	r.printf("\n")
}

func (r *textRenderer) emitExample(ex *doc.Example) {
	if r.err != nil {
		return
	}
	r.printf("Example")
	if ex.Suffix != "" {
		r.printf(" (%s)", ex.Suffix)
	}
	r.printf(":\n")
	if ex.Doc != "" {
		formatted := r.printer.Text(r.parser.Parse(ex.Doc))
		if len(formatted) > 0 {
			if _, err := r.w.Write(formatted); err != nil {
				r.err = err
				return
			}
			r.printf("\n")
		}
	}
	var buf strings.Builder
	if err := format.Node(&buf, r.fset, ex.Code); err != nil {
		r.err = err
		return
	}
	// Indent the code and output.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, line := range lines {
		// Omit blank line before close brace.
		if i == len(lines)-2 && line == "" {
			continue
		}
		r.printf("\t%s\n", line)
	}
	if ex.Output != "" {
		r.printf("\n\tOutput:\n")
		for _, line := range strings.Split(strings.TrimSpace(ex.Output), "\n") {
			r.printf("\t%s\n", line)
		}
	}
	r.printf("\n")
}

// TODO(jba): consolidate this function to avoid duplication.
func (r *textRenderer) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	if _, err := fmt.Fprintf(r.w, format, args...); err != nil {
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
		if _, err := r.w.Write(r.printer.Markdown(r.parser.Parse(pkg.Doc))); err != nil {
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
	r.printf("```go\n")
	err := format.Node(r.w, r.fset, node)
	if err != nil {
		r.err = err
		return
	}
	r.printf("\n```\n")
	formatted := r.printer.Markdown(r.parser.Parse(comment))
	if len(formatted) > 0 {
		if _, err = r.w.Write(formatted); err != nil {
			r.err = err
			return
		}
	}
	r.printf("\n")
}

func (r *markdownRenderer) emitExample(ex *doc.Example) {
	if r.err != nil {
		return
	}
	r.printf("#### Example")
	if ex.Suffix != "" {
		r.printf(" (%s)", ex.Suffix)
	}
	r.printf("\n\n")
	if ex.Doc != "" {
		if _, err := r.w.Write(r.printer.Markdown(r.parser.Parse(ex.Doc))); err != nil {
			r.err = err
			return
		}
		r.printf("\n")
	}
	r.printf("```go\n")
	err := format.Node(r.w, r.fset, ex.Code)
	if err != nil {
		r.err = err
		return
	}
	r.printf("\n```\n")
	if ex.Output != "" {
		r.printf("Output:\n\n```\n%s\n```\n", ex.Output)
	}
	r.printf("\n")
}

func (r *markdownRenderer) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	if _, err := fmt.Fprintf(r.w, format, args...); err != nil {
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
	buf     strings.Builder
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
		if _, err := r.w.Write(r.printer.HTML(r.parser.Parse(pkg.Doc))); err != nil {
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
	r.buf.Reset()
	err := format.Node(&r.buf, r.fset, node)
	if err != nil {
		r.err = err
		return
	}
	r.printf("<pre><code>%s</code></pre>\n", html.EscapeString(r.buf.String()))
	formatted := r.printer.HTML(r.parser.Parse(comment))
	if len(formatted) > 0 {
		if _, err = r.w.Write(formatted); err != nil {
			r.err = err
			return
		}
	}
	r.printf("\n")
}

func (r *htmlRenderer) emitExample(ex *doc.Example) {
	if r.err != nil {
		return
	}
	r.printf("<h4>Example")
	if ex.Suffix != "" {
		r.printf(" (%s)", ex.Suffix)
	}
	r.printf("</h4>\n")
	r.printf("\n")
	if ex.Doc != "" {
		if _, err := r.w.Write(r.printer.Markdown(r.parser.Parse(ex.Doc))); err != nil {
			r.err = err
			return
		}
		r.printf("\n")
	}
	r.printf("<pre><code>\n")
	err := format.Node(r.w, r.fset, ex.Code)
	if err != nil {
		r.err = err
		return
	}
	r.printf("\n</code></pre>\n")
	if ex.Output != "" {
		r.printf("Output:\n\n<pre><code>\n%s\n</code></pre>\n", html.EscapeString(ex.Output))
	}
	r.printf("\n")
}

func (r *htmlRenderer) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	if _, err := fmt.Fprintf(r.w, format, args...); err != nil {
		r.err = err
	}
}

// renderDoc renders the documentation for dpkg using the given renderer.
func renderDoc(dpkg *doc.Package, r renderer, examples bool) error {
	r.start(dpkg)
	if examples {
		for _, ex := range dpkg.Examples {
			r.emitExample(ex)
		}
	}

	renderValues(dpkg.Consts, r, "constants")
	renderValues(dpkg.Vars, r, "variables")
	renderFuncs(dpkg.Funcs, r, "functions", examples)

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
		if examples {
			for _, ex := range t.Examples {
				r.emitExample(ex)
			}
		}
		renderValues(t.Consts, r, "")
		renderValues(t.Vars, r, "")
		renderFuncs(t.Funcs, r, "", examples)
		renderFuncs(t.Methods, r, "", examples)
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

func renderFuncs(funcs []*doc.Func, r renderer, section string, examples bool) {
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
		if examples {
			for _, ex := range f.Examples {
				r.emitExample(ex)
			}
		}
	}
	if started && section != "" {
		r.endSection()
	}
}
