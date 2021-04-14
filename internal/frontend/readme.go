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
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/source"
)

// Heading holds data about a heading and nested headings within a readme.
// This data is used in the sidebar template to render the readme outline.
type Heading struct {
	// Level is the original level of the heading.
	Level int
	// Text is the content from the readme contained within a heading.
	Text string
	// ID corresponds to the ID attribute for a heading element
	// and is also used in an href to the corresponding section
	// within the readme outline. All ids are prefixed with readme-
	// to avoid name collisions.
	ID string
	// Children are nested headings.
	Children []*Heading
	// parent is the heading this heading is nested within. Nil for top
	// level headings.
	parent *Heading
}

// Readme holds the result of processing a REAME file.
type Readme struct {
	HTML    safehtml.HTML // rendered HTML
	Outline []*Heading    // document headings
	Links   []link        // links from the "Links" section
}

// ProcessReadme processes the README of unit u, if it has one.
// Processing includes rendering and sanitizing the HTML or Markdown,
// and extracting headings and links.
//
// Headings are prefixed with "readme-" and heading levels are adjusted to start
// at h3 in order to nest them properly within the rest of the page. The
// readme's original styling is preserved in the html by giving headings a css
// class styled identical to their original heading level.
//
//  The extracted links are for display outside of the readme contents.
//
// This function is exported for use by external tools.
func ProcessReadme(ctx context.Context, u *internal.Unit) (_ *Readme, err error) {
	defer derrors.WrapAndReport(&err, "ProcessReadme(%q, %q, %q)", u.Path, u.ModulePath, u.Version)
	return processReadme(ctx, u.Readme, u.SourceInfo)
}

func processReadme(ctx context.Context, readme *internal.Readme, sourceInfo *source.Info) (_ *Readme, err error) {
	if readme == nil || readme.Contents == "" {
		return &Readme{}, nil
	}
	if !isMarkdown(readme.Filepath) {
		t := template.Must(template.New("").Parse(`<pre class="readme">{{.}}</pre>`))
		h, err := t.ExecuteToHTML(readme.Contents)
		if err != nil {
			return nil, err
		}
		return &Readme{HTML: h}, nil
	}

	// Sets priority value so that we always use our custom transformer
	// instead of the default ones. The default values are in:
	// https://github.com/yuin/goldmark/blob/7b90f04af43131db79ec320be0bd4744079b346f/parser/parser.go#L567
	const astTransformerPriority = 10000
	el := &extractLinks{ctx: ctx}
	et := &extractTOC{ctx: ctx}
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
			parser.WithASTTransformers(
				util.Prioritized(&astTransformer{
					info:   sourceInfo,
					readme: readme,
				}, astTransformerPriority),
				// Extract links after we have transformed the URLs.
				util.Prioritized(el, astTransformerPriority+1),
				util.Prioritized(et, astTransformerPriority+1),
			),
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
			util.Prioritized(newHTMLRenderer(sourceInfo, readme), 100),
		),
	)
	contents := []byte(readme.Contents)
	gdParser := gdMarkdown.Parser()
	reader := gmtext.NewReader(contents)
	pctx := parser.NewContext(parser.WithIDs(newIDs()))
	doc := gdParser.Parse(reader, parser.WithContext(pctx))
	gdRenderer := gdMarkdown.Renderer()

	var b bytes.Buffer
	if err := gdRenderer.Render(&b, contents, doc); err != nil {
		return &Readme{}, nil
	}
	return &Readme{
		HTML:    sanitizeHTML(&b),
		Outline: et.headings,
		Links:   el.links,
	}, nil
}

// sanitizeHTML sanitizes HTML from a bytes.Buffer so that it is safe.
func sanitizeHTML(b *bytes.Buffer) safehtml.HTML {
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
