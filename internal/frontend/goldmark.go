/*
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

package frontend

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/source"
)

// astTransformer is a default transformer of the goldmark tree. We pass in
// readme information to use for the link transformations.
type astTransformer struct {
	info   *source.Info
	readme *internal.Readme
}

// Transform transforms the given AST tree.
func (g *astTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Image:
			if d := translateLink(string(v.Destination), g.info, true, g.readme); d != "" {
				v.Destination = []byte(d)
			}
		case *ast.Link:
			if d := translateLink(string(v.Destination), g.info, false, g.readme); d != "" {
				v.Destination = []byte(d)
			}
		}
		return ast.WalkContinue, nil
	})
}

// htmlRenderer is a renderer.NodeRenderer implementation that renders
// pkg.go.dev readme features.
type htmlRenderer struct {
	html.Config
	info   *source.Info
	readme *internal.Readme
	// firstHeading and offset are used to calculate the first heading tag's level in a readme.
	firstHeading bool
	offset       int
}

// newHTMLRenderer creates a new HTMLRenderer for a readme.
func newHTMLRenderer(info *source.Info, readme *internal.Readme, opts ...html.Option) renderer.NodeRenderer {
	r := &htmlRenderer{
		info:         info,
		readme:       readme,
		Config:       html.NewConfig(),
		firstHeading: true,
		offset:       0,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *htmlRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
}

func (r *htmlRenderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if r.firstHeading {
		// The offset ensures the first heading is always an <h3>.
		r.offset = 3 - n.Level
		r.firstHeading = false
	}
	newLevel := n.Level + r.offset
	if entering {
		// TODO(matloob): Do we want the div and h elements to have analagous classes?
		// Currently we're using newLevel for the div's class but n.Level for the h element's
		// class.
		if newLevel > 6 {
			_, _ = w.WriteString(fmt.Sprintf(`<div class="h%d" role="heading" aria-level="%d"`, newLevel, n.Level))
		} else {
			_, _ = w.WriteString(fmt.Sprintf(`<h%d class="h%d"`, newLevel, n.Level))
		}
		if n.Attributes() != nil {
			html.RenderAttributes(w, node, html.HeadingAttributeFilter)
		}
		_ = w.WriteByte('>')
	} else {
		if newLevel > 6 {
			_, _ = w.WriteString("</div>\n")
		} else {
			_, _ = w.WriteString(fmt.Sprintf("</h%d>\n", newLevel))
		}
	}
	return ast.WalkContinue, nil
}

// renderHTMLBlock is copied directly from the goldmark source code and
// modified to call translateHTML in every block
func (r *htmlRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.HTMLBlock)
	if entering {
		if r.Unsafe {
			l := n.Lines().Len()
			for i := 0; i < l; i++ {
				line := n.Lines().At(i)
				d, err := translateHTML(line.Value(source), r.info, r.readme)
				if err != nil {
					return ast.WalkStop, err
				}
				_, _ = w.Write(d)
			}
		} else {
			_, _ = w.WriteString("<!-- raw HTML omitted -->\n")
		}
	} else {
		if n.HasClosure() {
			if r.Unsafe {
				closure := n.ClosureLine
				_, _ = w.Write(closure.Value(source))
			} else {
				_, _ = w.WriteString("<!-- raw HTML omitted -->\n")
			}
		}
	}
	return ast.WalkContinue, nil
}

func (r *htmlRenderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	if r.Unsafe {
		n := node.(*ast.RawHTML)
		for i := 0; i < n.Segments.Len(); i++ {
			segment := n.Segments.At(i)
			d, err := translateHTML(segment.Value(source), r.info, r.readme)
			if err != nil {
				return ast.WalkStop, err
			}
			_, _ = w.Write(d)
		}
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString("<!-- raw HTML omitted -->")
	return ast.WalkSkipChildren, nil
}

// ids is a collection of element ids in document.
type ids struct {
	values map[string]bool
}

// newIDs creates a collection of element ids in a document.
func newIDs() parser.IDs {
	return &ids{
		values: map[string]bool{},
	}
}

// Generate turns heading content from a markdown document into a heading id.
// First HTML markup and markdown images are stripped then ASCII letters
// and numbers are used to generate the final result. Finally, all heading ids
// are prefixed with "readme-" to avoid name collisions with other ids on the
// unit page. Duplicated heading ids are given an incremental suffix. See
// readme_test.go for examples.
func (s *ids) Generate(value []byte, kind ast.NodeKind) []byte {
	// Matches strings like `<tag attr="value">Text</tag>` or `[![Text](file.svg)](link.html)`.
	r := regexp.MustCompile(`(<[^<>]+>|\[\!\[[^\]]+]\([^\)]+\)\]\([^\)]+\))`)
	str := r.ReplaceAllString(string(value), "")
	f := func(c rune) bool {
		return !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9')
	}
	str = strings.Join(strings.FieldsFunc(str, f), "-")
	str = strings.ToLower(str)
	if len(str) == 0 {
		if kind == ast.KindHeading {
			str = "heading"
		} else {
			str = "id"
		}
	}
	key := str
	for i := 1; ; i++ {
		if _, ok := s.values[key]; !ok {
			s.values[key] = true
			break
		}
		key = fmt.Sprintf("%s-%d", str, i)
	}
	return []byte("readme-" + key)
}

// Put implements Put from the goldmark parser IDs interface.
func (s *ids) Put(value []byte) {
	s.values[string(value)] = true
}

type extractLinks struct {
	ctx            context.Context
	inLinksHeading bool
	links          []link
}

// The name of the heading from which we extract links.
const linkHeadingText = "Links"

var linkHeadingBytes = []byte(linkHeadingText) // for faster comparison to node contents

// Transform extracts links from the "Links" section of a README.
func (e *extractLinks) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	err := ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {

		case *ast.Heading:
			// We are in the links heading from the point we see a heading with
			// linkHeadingText until the point we see the next heading.
			if e.inLinksHeading {
				return ast.WalkStop, nil
			}
			if bytes.Equal(n.Text(reader.Source()), linkHeadingBytes) {
				e.inLinksHeading = true
			}

		case *ast.ListItem:
			// When in the links heading, extract links from list items.
			if !e.inLinksHeading {
				return ast.WalkSkipChildren, nil
			}
			// We expect the pattern: ListItem -> TextBlock -> Link, with no
			// other children.
			if tb, ok := n.FirstChild().(*ast.TextBlock); ok {
				if l, ok := tb.FirstChild().(*ast.Link); ok && l.NextSibling() == nil {
					// Record the link.
					e.links = append(e.links, link{
						Href: string(l.Destination),
						Body: string(l.Text(reader.Source())),
					})
				}
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		log.Errorf(e.ctx, "extractLinks.Transform: %v", err)
	}
}

type extractTOC struct {
	ctx         context.Context
	Headings    []*Heading
	removeTitle bool // omit title from TOC
}

// Transform collects the headings from a readme into an outline
// of the document. It nests the headings based on the h-level hierarchy.
// See tests for heading levels in TestReadme for behavior.
func (e *extractTOC) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	var headings []*Heading
	err := ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if n.Kind() == ast.KindHeading && entering {
			heading := n.(*ast.Heading)
			section := &Heading{
				Level: heading.Level,
				Text:  string(n.Text(reader.Source())),
			}
			if id, ok := heading.AttributeString("id"); ok {
				section.ID = string(id.([]byte))
			}
			headings = append(headings, section)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		log.Errorf(e.ctx, "extractTOC.Transform: %v", err)
	}

	// We nest the headings by walking through the list we extracted and
	// establishing parent child relationships based on heading levels.
	var nested []*Heading
	for i, h := range headings {
		if i == 0 {
			nested = append(nested, h)
			continue
		}
		parent := headings[i-1]
		for parent != nil && parent.Level >= h.Level {
			parent = parent.parent
		}
		if parent == nil {
			nested = append(nested, h)
		} else {
			h.parent = parent
			parent.Children = append(parent.Children, h)
		}
	}
	if e.removeTitle {
		// If there is only one top tevel heading with 1 or more children we
		// assume it is the title of the document and remove it from the TOC.
		if len(nested) == 1 && len(nested[0].Children) > 0 {
			nested = nested[0].Children
		}
	}
	e.Headings = nested
}
