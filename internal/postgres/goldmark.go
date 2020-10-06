/*
* Copyright 2020 The Go Authors. All rights reserved.
* Use of this source code is governed by a BSD-style
* license that can be found in the LICENSE file.
 */

package postgres

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// ASTTransformer is a default transformer of the goldmark tree.
type ASTTransformer struct{}

// HTMLRenderer is a renderer.NodeRenderer implementation that renders
// pkg.go.dev readme features.
type HTMLRenderer struct {
	html.Config
}

// Transform transforms the given AST tree to remove an unnecessary child
// node from the image node. This is so that the summary generated doesn't
// the text content of an image block.
func (g *ASTTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Image:
			// remove KindText childnode from image
			v.RemoveChild(v, v.FirstChild())
		}
		return ast.WalkContinue, nil
	})
}

// NewHTMLRenderer creates a new HTMLRenderer for a readme.
func NewHTMLRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &HTMLRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *HTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// skip rendering for everything except for KindText
	for _, kind := range []ast.NodeKind{
		ast.KindAutoLink, ast.KindBlockquote, ast.KindCodeBlock, ast.KindCodeSpan,
		ast.KindDocument, ast.KindEmphasis, ast.KindFencedCodeBlock, ast.KindHTMLBlock,
		ast.KindHeading, ast.KindLink, ast.KindList, ast.KindListItem,
		ast.KindParagraph, ast.KindRawHTML, ast.KindString, ast.KindTextBlock,
		ast.KindThematicBreak,
	} {
		reg.Register(kind, r.skipNode)
	}
	reg.Register(ast.KindImage, r.skipImage)
}

func (r *HTMLRenderer) skipImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		w.WriteString(" ")
	}
	return ast.WalkContinue, nil
}

func (r *HTMLRenderer) skipNode(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}
