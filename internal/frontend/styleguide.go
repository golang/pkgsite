// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"html"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/safehtml"
	"github.com/google/safehtml/uncheckedconversions"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// serveStyleGuide serves the styleguide page, the content of which is
// generated from the markdown files in static/shared.
func (s *Server) serveStyleGuide(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	ctx := r.Context()
	if !experiment.IsActive(ctx, internal.ExperimentStyleGuide) {
		return &serverError{status: http.StatusNotFound}
	}
	page, err := styleGuide(ctx, s.staticFS)
	page.basePage = s.newBasePage(r, "")
	page.AllowWideContent = true
	page.UseResponsiveLayout = true
	page.Title = "Style Guide"
	if err != nil {
		return err
	}
	s.servePage(ctx, w, "styleguide", page)
	return nil
}

type styleGuidePage struct {
	basePage
	Title    string
	Sections []*StyleSection
	Outline  []*Heading
}

// styleGuide collects the paths to the markdown files in staticFS,
// renders them into sections for the styleguide, and merges the document
// outlines into a single page outline.
func styleGuide(ctx context.Context, staticFS fs.FS) (_ *styleGuidePage, err error) {
	defer derrors.WrapStack(&err, "styleGuide)")
	files, err := markdownFiles(staticFS)
	if err != nil {
		return nil, err
	}
	var sections []*StyleSection
	for _, f := range files {
		doc, err := styleSection(ctx, staticFS, f)
		if err != nil {
			return nil, err
		}
		sections = append(sections, doc)
	}
	var outline []*Heading
	for _, s := range sections {
		outline = append(outline, s.Outline...)
	}
	return &styleGuidePage{
		Sections: sections,
		Outline:  outline,
	}, nil
}

// StyleSection represents a section on the styleguide page.
type StyleSection struct {
	// ID is the ID for the header element of the section.
	ID string

	// Title is the title of the section, taken from the name
	// of the markdown file.
	Title string

	// Content is the HTML rendered from the parsed markdown file.
	Content safehtml.HTML

	// Outline is a collection of headings used in the navigation.
	Outline []*Heading
}

// styleSection uses goldmark to parse a markdown file and render
// a section of the styleguide.
func styleSection(ctx context.Context, fsys fs.FS, filename string) (_ *StyleSection, err error) {
	defer derrors.WrapStack(&err, "styleSection(%q)", filename)
	var buf bytes.Buffer
	source, err := fs.ReadFile(fsys, filename)
	if err != nil {
		return nil, err
	}

	// We set priority values so that we always use our custom transformer
	// instead of the default ones. The default values are in:
	// https://github.com/yuin/goldmark/blob/7b90f04af43131db79ec320be0bd4744079b346f/parser/parser.go#L567
	const (
		astTransformerPriority = 10000
		nodeRenderersPriority  = 100
	)
	et := &extractTOC{ctx: ctx}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithAttribute(),
			parser.WithASTTransformers(
				util.Prioritized(et, astTransformerPriority),
			),
		),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(
				util.Prioritized(&guideRenderer{}, nodeRenderersPriority),
			),
			ghtml.WithUnsafe(),
			ghtml.WithXHTML(),
		),
	)

	if err := md.Convert(source, &buf); err != nil {
		return nil, err
	}

	id := strings.TrimSuffix(filepath.Base(filename), ".md")
	return &StyleSection{
		ID:      id,
		Title:   camelCase(id),
		Content: uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(buf.String()),
		Outline: et.Headings,
	}, nil
}

// guideRenderer is a renderer.NodeRenderer implementation that renders
// styleguide sections.
type guideRenderer struct {
	ghtml.Config
}

func (r *guideRenderer) writeLines(w util.BufWriter, source []byte, n ast.Node) {
	l := n.Lines().Len()
	for i := 0; i < l; i++ {
		line := n.Lines().At(i)
		w.Write(line.Value(source))
	}
}

func (r *guideRenderer) writeEscapedLines(w util.BufWriter, source []byte, n ast.Node) {
	l := n.Lines().Len()
	for i := 0; i < l; i++ {
		line := n.Lines().At(i)
		w.Write([]byte(html.EscapeString(string(line.Value(source)))))
	}
}

// renderFencedCodeBlock writes html code snippets twice, once as actual
// html for the page and again as a code snippet.
func (r *guideRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	w.WriteString("<span>\n")
	r.writeLines(w, source, n)
	w.WriteString("</span>\n")
	w.WriteString("<pre class=\"StringifyElement-markup js-clipboard\">\n")
	r.writeEscapedLines(w, source, n)
	w.WriteString("</pre>\n")
	return ast.WalkContinue, nil
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *guideRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

// markdownFiles walks the shared directory of fsys and collects
// the paths to markdown files.
func markdownFiles(fsys fs.FS) ([]string, error) {
	var matches []string
	err := fs.WalkDir(fsys, "shared", func(filepath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path.Ext(filepath) == ".md" {
			matches = append(matches, filepath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// camelCase turns a snake cased string into a camel case string.
// For example, hello-world becomes HelloWorld. This function is
// used to ensure proper casing in the classnames of the style
// sections.
func camelCase(s string) string {
	p := strings.Split(s, "-")
	var o []string
	for _, v := range p {
		o = append(o, cases.Title(language.Und).String(v))
	}
	return strings.Join(o, "")
}
