// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"

	"github.com/shurcooL/htmlg"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/tools/go/packages"
)

// computeDoc computes the package documentation for p.
func computeDoc(p *packages.Package) (*token.FileSet, *doc.Package, error) {
	var (
		fset  = token.NewFileSet()
		files = make(map[string]*ast.File)
	)
	for _, name := range p.GoFiles {
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			return nil, nil, err
		}
		files[name] = f
	}
	apkg := &ast.Package{
		Name:  p.Name,
		Files: files,
	}
	return fset, doc.New(apkg, p.PkgPath, 0), nil
}

// renderDocHTML renders package documentation HTML for the
// provided file set and package.
func renderDocHTML(fset *token.FileSet, p *doc.Package) ([]byte, error) {
	var buf bytes.Buffer
	err := htmlg.RenderComponents(&buf, godocComponent{
		Fset:    fset,
		Package: p,
	})
	return buf.Bytes(), err
}

// godocComponent is an htmlg.Component that renders package
// documentation HTML for the given file set and doc.Package.
//
// This is a temporary approach for rendering; it's likely to
// be replaced by a html/template-based approach. This is a
// part of unified documentation rendering work that's underway.
type godocComponent struct {
	Fset *token.FileSet
	*doc.Package
}

// Render implements the htmlg.Component interface.
func (p godocComponent) Render() []*html.Node {
	ns := []*html.Node{
		htmlg.P(
			parseHTML(docHTML(p.Doc)),
		),
		htmlg.H1(htmlg.Text("Index")),
		htmlg.P(htmlg.Text("<TODO>")), // Issue b/131827600.
	}

	// Constants.
	if len(p.Consts) > 0 {
		ns = append(ns, htmlg.H2(htmlg.Text("Constants")))
	}
	for _, c := range p.Consts {
		ns = append(ns,
			htmlg.Pre(
				htmlg.Text(printASTNode(p.Fset, c.Decl)),
			),
		)
	}

	// Variables.
	if len(p.Vars) > 0 {
		ns = append(ns, htmlg.H2(htmlg.Text("Variables")))
	}
	for _, v := range p.Vars {
		ns = append(ns,
			htmlg.Pre(
				htmlg.Text(printASTNode(p.Fset, v.Decl)),
			),
			htmlg.P(
				parseHTML(docHTML(v.Doc)),
			),
		)
	}

	// Functions.
	for _, f := range p.Funcs {
		heading := htmlg.H2(htmlg.Text("func "+f.Name+" "), htmlg.A("¶", "#"+f.Name))
		heading.Attr = append(heading.Attr, html.Attribute{
			Key: atom.Id.String(), Val: f.Name,
		})
		ns = append(ns,
			heading,
			htmlg.Pre(
				htmlg.Text(printASTNode(p.Fset, f.Decl)),
			),
			htmlg.P(
				parseHTML(docHTML(f.Doc)),
			),
		)
	}

	// Types.
	for _, t := range p.Types {
		ns = append(ns,
			htmlg.H2(htmlg.Text("type "+t.Name)),
			htmlg.Pre(
				htmlg.Text(printASTNode(p.Fset, t.Decl)),
			),
			htmlg.P(
				parseHTML(docHTML(t.Doc)),
			),
		)
		for _, c := range t.Consts {
			ns = append(ns,
				htmlg.Pre(
					htmlg.Text(printASTNode(p.Fset, c.Decl)),
				),
				htmlg.P(
					parseHTML(docHTML(c.Doc)),
				),
			)
		}
		for _, m := range t.Methods {
			ns = append(ns,
				htmlg.H3(htmlg.Text("func ("+m.Recv+") "+m.Name)),
				htmlg.Pre(
					htmlg.Text(printASTNode(p.Fset, m.Decl)),
				),
				htmlg.P(
					parseHTML(docHTML(m.Doc)),
				),
			)
		}
	}

	return ns
}

// printASTNode prints node, using fset, and returns it as string.
func printASTNode(fset *token.FileSet, node interface{}) string {
	var buf bytes.Buffer
	gofmtConfig.Fprint(&buf, fset, node)
	return buf.String()
}

// gofmtConfig is consistent with the default gofmt behavior.
var gofmtConfig = printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}

// docHTML returns documentation comment text converted to formatted HTML.
func docHTML(text string) string {
	var buf bytes.Buffer
	doc.ToHTML(&buf, text, nil)
	return buf.String()
}

// TODO: skip this unneccessary round-trip
func parseHTML(s string) *html.Node {
	n, err := html.Parse(strings.NewReader(s))
	if err != nil {
		panic(fmt.Errorf("internal error: html.Parse failed to parse our own HTML: %v", err))
	}
	return n
}
