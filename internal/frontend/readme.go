// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"github.com/google/safehtml/uncheckedconversions"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// ReadmeHTML sanitizes readmeContents based on bluemondy.UGCPolicy and returns
// a safehtml.HTML. If readmeFilePath indicates that this is a markdown file,
// it will also render the markdown contents using goldmark.
//
// This function is exported for use in an external tool that uses this package to
// compare readme files to see how changes in processing will affect them.
func ReadmeHTML(ctx context.Context, mi *internal.ModuleInfo, readme *internal.Readme) (_ safehtml.HTML, err error) {
	defer derrors.Wrap(&err, "ReadmeHTML(%s@%s)", mi.ModulePath, mi.Version)
	if readme == nil || readme.Contents == "" {
		return safehtml.HTML{}, nil
	}
	if !isMarkdown(readme.Filepath) {
		t := template.Must(template.New("").Parse(`<pre class="readme">{{.}}</pre>`))
		h, err := t.ExecuteToHTML(readme.Contents)
		if err != nil {
			return safehtml.HTML{}, err
		}
		return h, nil
	}

	// Sets priority value so that we always use our custom transformer
	// instead of the default ones. The default values are in:
	// https://github.com/yuin/goldmark/blob/7b90f04af43131db79ec320be0bd4744079b346f/parser/parser.go#L567
	const ASTTransformerPriority = 10000
	gdMarkdown := goldmark.New(
		goldmark.WithParserOptions(
			// WithHeadingAttribute allows us to include other attributes in
			// heading tags. This is useful for our aria-level implementation of
			// increasing heading rankings.
			parser.WithHeadingAttribute(),
			// Generates an id in every heading tag. This is used in github in
			// order to generate a link with a hash that a user would scroll to
			// <h1 id="goldmark">goldmark</h1> => github.com/yuin/goldmark#goldmark
			parser.WithAutoHeadingID(),
			// Include custom ASTTransformer using the readme and module info to
			// use translateRelativeLink and translateHTML to modify the AST
			// before it is rendered.
			parser.WithASTTransformers(util.Prioritized(&ASTTransformer{
				info:   mi.SourceInfo,
				readme: readme,
			}, ASTTransformerPriority)),
		),
		// These extensions lets users write HTML code in the README. This is
		// fine since we process the contents using bluemonday after.
		goldmark.WithRendererOptions(goldmarkHtml.WithUnsafe(), goldmarkHtml.WithXHTML()),
		goldmark.WithExtensions(
			extension.GFM, // Support Github Flavored Markdown.
			emoji.Emoji,   // Support Github markdown emoji markup.
		),
	)
	gdMarkdown.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(NewHTMLRenderer(mi.SourceInfo, readme), 100),
		),
	)

	var b bytes.Buffer
	contents := []byte(readme.Contents)
	gdRenderer := gdMarkdown.Renderer()
	gdParser := gdMarkdown.Parser()

	reader := text.NewReader(contents)
	doc := gdParser.Parse(reader)

	if err := gdRenderer.Render(&b, contents, doc); err != nil {
		return safehtml.HTML{}, nil
	}
	return sanitizeGoldmarkHTML(&b), nil
}

// sanitizeGoldmarkHTML sanitizes HTML from a bytes.Buffer so that it is safe.
func sanitizeGoldmarkHTML(b *bytes.Buffer) safehtml.HTML {
	p := bluemonday.UGCPolicy()

	p.AllowAttrs("width", "align").OnElements("img")
	p.AllowAttrs("width", "align").OnElements("div")
	p.AllowAttrs("width", "align").OnElements("p")
	// Allow accessible headings (i.e <div role="heading" aria-level="7">).
	p.AllowAttrs("width", "align", "role", "aria-level").OnElements("div")
	for _, h := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
		// Needed to preserve github styles heading font-sizes
		p.AllowAttrs("class").OnElements(h)
	}

	s := string(p.SanitizeBytes(b.Bytes()))
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(s)
}
