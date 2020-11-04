/*
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

package frontend

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/source"
)

// ASTTransformer is a default transformer of the goldmark tree. We pass in
// readme information to use for the link transformations.
type ASTTransformer struct {
	info   *source.Info
	readme *internal.Readme
}

// Transform transforms the given AST tree.
func (g *ASTTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Image:
			if d := translateRelativeLink(string(v.Destination), g.info, true, g.readme); d != "" {
				v.Destination = []byte(d)
			}
		case *ast.Link:
			if d := translateRelativeLink(string(v.Destination), g.info, false, g.readme); d != "" {
				v.Destination = []byte(d)
			}
		case *ast.Heading:
			if id, ok := v.AttributeString("id"); ok {
				v.SetAttributeString("id", append([]byte("readme-"), id.([]byte)...))
			}
		}
		return ast.WalkContinue, nil
	})
}

// HTMLRenderer is a renderer.NodeRenderer implementation that renders
// pkg.go.dev readme features.
type HTMLRenderer struct {
	html.Config
	info   *source.Info
	readme *internal.Readme
	// firstHeading and offset are used to calculate the first heading tag's level in a readme.
	firstHeading bool
	offset       int
}

// NewHTMLRenderer creates a new HTMLRenderer for a readme.
func NewHTMLRenderer(info *source.Info, readme *internal.Readme, opts ...html.Option) renderer.NodeRenderer {
	r := &HTMLRenderer{
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
func (r *HTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)
}

func (r *HTMLRenderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if r.firstHeading {
		// The offset ensures the first heading is always an <h3>.
		r.offset = 3 - n.Level
		r.firstHeading = false
	}
	newLevel := n.Level + r.offset
	if entering {
		if n.Level > 6 {
			_, _ = w.WriteString(fmt.Sprintf(`<div class="h%d" role="heading" aria-level="%d"`, newLevel, n.Level))
		} else {
			_, _ = w.WriteString(fmt.Sprintf(`<h%d class="h%d"`, newLevel, n.Level))
		}
		if n.Attributes() != nil {
			html.RenderAttributes(w, node, html.HeadingAttributeFilter)
		}
		_ = w.WriteByte('>')
	} else {
		if n.Level > 6 {
			_, _ = w.WriteString("</div>\n")
		} else {
			_, _ = w.WriteString(fmt.Sprintf("</h%d>\n", newLevel))
		}
	}
	return ast.WalkContinue, nil
}

// renderHTMLBlock is copied directly from the goldmark source code and
// modified to call translateHTML in every block
func (r *HTMLRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
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

func (r *HTMLRenderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
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
