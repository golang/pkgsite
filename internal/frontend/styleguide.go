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
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/frontend/page"
	"golang.org/x/pkgsite/internal/frontend/serrors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"rsc.io/markdown"
)

// serveStyleGuide serves the styleguide page, the content of which is
// generated from the markdown files in static/shared.
func (s *Server) serveStyleGuide(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	ctx := r.Context()
	if !experiment.IsActive(ctx, internal.ExperimentStyleGuide) {
		return &serrors.ServerError{Status: http.StatusNotFound}
	}
	page, err := styleGuide(ctx, s.staticFS)
	page.BasePage = s.newBasePage(r, "Style Guide")
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
	page.BasePage
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

	p := markdown.Parser{
		HeadingIDs:         true,
		Strikethrough:      true,
		TaskListItems:      true,
		AutoLinkText:       true,
		AutoLinkAssumeHTTP: true,
		Table:              true,
		Emoji:              true,
	}
	doc := p.Parse(string(source))
	addMissingHeadingIDs(doc) // extractTOC is going to use these ids, so do this first
	et := &extractTOC{ctx: ctx}
	et.extract(doc)
	renderCodeBlocks(doc)
	doc.PrintHTML(&buf)

	id := strings.TrimSuffix(filepath.Base(filename), ".md")
	return &StyleSection{
		ID:      id,
		Title:   camelCase(id),
		Content: uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(buf.String()),
		Outline: et.Headings,
	}, nil
}

func writeLines(w *bytes.Buffer, lines []string) {
	for _, l := range lines {
		w.WriteString(l)
		w.WriteString("\n")
	}
}

func writeEscapedLines(w *bytes.Buffer, lines []string) {
	for _, l := range lines {
		w.WriteString(html.EscapeString(l))
		w.WriteString("\n")
	}
}

// renderFencedCodeBlock writes html code snippets twice, once as actual
// html for the page and again as a code snippet.
func renderCodeBlocks(doc *markdown.Document) {
	var rewriteBlocks func([]markdown.Block)
	rewriteBlocks = func(blocks []markdown.Block) {
		for i, b := range blocks {
			switch x := b.(type) {
			case *markdown.Text:
			case *markdown.HTMLBlock:
			case *markdown.Table:
			case *markdown.Empty:
			case *markdown.ThematicBreak:
			case *markdown.Paragraph:
				rewriteBlocks([]markdown.Block{x.Text})
			case *markdown.List:
				rewriteBlocks(x.Items)
			case *markdown.Item:
				rewriteBlocks(x.Blocks)
			case *markdown.Quote:
				rewriteBlocks(x.Blocks)
			case *markdown.Heading:
			case *markdown.CodeBlock:
				htmltag := &markdown.HTMLBlock{}
				var buf bytes.Buffer
				buf.WriteString("<span>\n")
				writeLines(&buf, x.Text)
				buf.WriteString("</span>\n")
				buf.WriteString("<pre class=\"StringifyElement-markup js-clipboard\">\n")
				writeEscapedLines(&buf, x.Text)
				buf.WriteString("</pre>")
				htmltag.Text = append(htmltag.Text, buf.String())
				blocks[i] = htmltag
			}
		}
	}
	rewriteBlocks(doc.Blocks)
}

func addMissingHeadingIDs(doc *markdown.Document) {
	walkBlocks(doc.Blocks, func(b markdown.Block) error {
		if heading, ok := b.(*markdown.Heading); ok {
			if heading.ID != "" {
				return nil
			}
			var buf bytes.Buffer
			for _, inl := range heading.Text.Inline {
				inl.PrintText(&buf)
			}
			f := func(c rune) bool {
				return !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9')
			}
			heading.ID = strings.ToLower(strings.Join(strings.FieldsFunc(buf.String(), f), "-"))
		}
		return nil
	})
}

// markdownFiles walks the shared directory of fsys and collects
// the paths to markdown files.
func markdownFiles(fsys fs.FS) ([]string, error) {
	var matches []string
	err := fs.WalkDir(fsys, "shared", func(filepath string, _ fs.DirEntry, err error) error {
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
