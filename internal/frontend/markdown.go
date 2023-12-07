// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/source"
	"rsc.io/markdown"
)

// ProcessReadmeMarkdown processes the README of unit u, if it has one.
// Processing includes rendering and sanitizing the HTML or Markdown,
// and extracting headings and links.
//
// Headings are prefixed with "readme-" and heading levels are adjusted to start
// at h3 in order to nest them properly within the rest of the page. The
// readme's original styling is preserved in the html by giving headings a css
// class styled identical to their original heading level.
//
// The extracted links are for display outside of the readme contents.
//
// This function is exported for use by external tools.
func ProcessReadmeMarkdown(ctx context.Context, u *internal.Unit) (_ *Readme, err error) {
	defer derrors.WrapAndReport(&err, "ProcessReadmeMarkdown(%q, %q, %q)", u.Path, u.ModulePath, u.Version)
	return processReadmeMarkdown(ctx, u.Readme, u.SourceInfo)
}

func processReadmeMarkdown(ctx context.Context, readme *internal.Readme, info *source.Info) (frontendReadme *Readme, err error) {
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

	var p markdown.Parser
	doc := p.Parse(readme.Contents)
	(&linkRewriter{info, readme}).rewriteLinks(doc)
	rewriteImgSrc(doc, info, readme)
	rewriteHeadingIDs(doc) // rewrite heading ids before extractTOC extracts them
	et := &extractTOC{ctx: ctx, removeTitle: true}
	et.extract(doc)
	el := &extractLinks{ctx: ctx}
	el.extract(doc)
	transformHeadingsToHTML(doc)
	var buf bytes.Buffer
	doc.PrintHTML(&buf)
	return &Readme{
		HTML:    sanitizeHTML(&buf),
		Outline: et.Headings,
		Links:   el.links,
	}, nil
}

// rewriteImgSrc rewrites the HTML in the markdown document to replace img
// src keys with a value that properly represents the source of the image
// from the repo.
func rewriteImgSrc(doc *markdown.Document, info *source.Info, readme *internal.Readme) {
	walkBlocks(doc.Blocks, func(b markdown.Block) error {
		switch x := b.(type) {
		case *markdown.HTMLBlock:
			htmlBlock := x
			for i := range htmlBlock.Text {
				translated, err := translateHTML([]byte(htmlBlock.Text[i]), info, readme)
				if err != nil {
					continue
				}
				htmlBlock.Text[i] = string(translated)
			}
		case *markdown.Text:
			rewriteHtmlInline(x.Inline, info, readme)
		}
		return nil
	})
}

func rewriteHtmlInline(inlines []markdown.Inline, info *source.Info, readme *internal.Readme) {
	for _, inl := range inlines {
		if htmlTag, ok := inl.(*markdown.HTMLTag); ok {
			translated, err := translateHTML([]byte(htmlTag.Text), info, readme)
			if err != nil {
				continue
			}
			htmlTag.Text = string(translated)
		}
	}
}

var errSkipChildren = errors.New("skip children")

// walkBlocks calls walkFunc on all the blocks in the markdown document. If the
// walkFunc returns the errSkipChildren error the children of that block will be skipped.
func walkBlocks(blocks []markdown.Block, walkFunc func(b markdown.Block) error) error {
	for _, b := range blocks {
		err := walkFunc(b)
		if err == errSkipChildren {
			continue
		} else if err != nil {
			return err
		}

		err = nil
		switch x := b.(type) {
		case *markdown.Document:
			err = walkBlocks(x.Blocks, walkFunc)
		case *markdown.Text:
		case *markdown.Paragraph:
			err = walkBlocks([]markdown.Block{x.Text}, walkFunc)
		case *markdown.Heading:
			err = walkBlocks([]markdown.Block{x.Text}, walkFunc)
		case *markdown.List:
			err = walkBlocks(x.Items, walkFunc)
		case *markdown.Item:
			err = walkBlocks(x.Blocks, walkFunc)
		case *markdown.Quote:
			err = walkBlocks(x.Blocks, walkFunc)
		case *markdown.HTMLBlock:
		case *markdown.CodeBlock:
		case *markdown.Empty:
		case *markdown.ThematicBreak:
		default:
			return fmt.Errorf("unhandled block type %T", x)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *extractTOC) extract(doc *markdown.Document) {
	var headings []*Heading
	err := walkBlocks(doc.Blocks, func(b markdown.Block) error {
		if heading, ok := b.(*markdown.Heading); ok {
			var textbuf bytes.Buffer
			for _, t := range heading.Text.Inline {
				t.PrintText(&textbuf)
			}
			section := &Heading{
				Level: heading.Level,
				Text:  textbuf.String(),
			}
			section.ID = heading.ID
			headings = append(headings, section)
			return errSkipChildren
		}
		return nil
	})
	if err != nil {
		log.Errorf(e.ctx, "extractTOC.extract: %v", err)
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

func (e *extractLinks) extract(doc *markdown.Document) {
	var seenLinksHeading bool
	err := walkBlocks(doc.Blocks, func(b markdown.Block) error {
		switch x := b.(type) {
		case *markdown.Heading:
			// We are in the links heading from the point we see a heading with
			// linkHeadingText until the point we see the next heading.
			if e.inLinksHeading {
				e.inLinksHeading = false
			}
			var headingText bytes.Buffer
			for _, t := range x.Text.Inline {
				t.PrintText(&headingText)
			}
			if !seenLinksHeading && bytes.Equal(headingText.Bytes(), linkHeadingBytes) {
				seenLinksHeading = true
				e.inLinksHeading = true
			}
		case *markdown.Item:
			// When in the links heading, extract links from list items.
			if !e.inLinksHeading {
				return errSkipChildren
			}
			// We expect the pattern: ListItem -> TextBlock -> Link, with no
			// other children.
			if len(x.Blocks) == 0 {
				return errSkipChildren
			}
			if tb, ok := x.Blocks[0].(*markdown.Text); ok {
				if len(tb.Inline) != 1 {
					return errSkipChildren
				}
				if l, ok := tb.Inline[0].(*markdown.Link); ok {
					// Record the link.
					var linkText bytes.Buffer
					for _, t := range l.Inner {
						t.PrintText(&linkText)
					}
					e.links = append(e.links, link{
						Href: l.URL,
						Body: linkText.String(),
					})
				}
			}
			return errSkipChildren
		}
		return nil
	})
	if err != nil {
		log.Errorf(e.ctx, "extractLinks.extract: %v", err)
	}
}

// linkRewriter rewrites links and image targets in a markdown document
// using translateLink.
type linkRewriter struct {
	info   *source.Info
	readme *internal.Readme
}

func (g *linkRewriter) rewriteLinks(doc *markdown.Document) {
	walkBlocks(doc.Blocks, func(b markdown.Block) error {
		if text, ok := b.(*markdown.Text); ok {
			g.rewriteLinksInline(text.Inline)
		}
		return nil
	})
}

func (g *linkRewriter) rewriteLinksInline(inlines []markdown.Inline) {
	for _, inl := range inlines {
		switch x := inl.(type) {
		case *markdown.Link:
			g.rewriteLinksInline(x.Inner)
			if d := translateLink(x.URL, g.info, false, g.readme); d != "" {
				x.URL = d
			}
		case *markdown.Image:
			g.rewriteLinksInline(x.Inner)
			if d := translateLink(x.URL, g.info, true, g.readme); d != "" {
				x.URL = d
			}
		case *markdown.Emph:
			g.rewriteLinksInline(x.Inner)
		case *markdown.Strong:
			g.rewriteLinksInline(x.Inner)
		}

	}
}

// transformHeadingsToHTML replaces heading blocks with rendered html
// blocks for the heading. It converts heading levels above 6 to divs
// with the h[level] class set on them.
func transformHeadingsToHTML(doc *markdown.Document) {
	firstHeading := true
	offset := 0
	var rewriteHeadingsBlocks func([]markdown.Block)
	rewriteHeadingsBlocks = func(blocks []markdown.Block) {
		for i, b := range blocks {
			switch x := b.(type) {
			case *markdown.Paragraph:
				rewriteHeadingsBlocks([]markdown.Block{x.Text})
			case *markdown.List:
				rewriteHeadingsBlocks(x.Items)
			case *markdown.Item:
				rewriteHeadingsBlocks(x.Blocks)
			case *markdown.Quote:
				rewriteHeadingsBlocks(x.Blocks)
			case *markdown.Heading:
				heading := x
				if firstHeading {
					// The offset ensures the first heading is always an <h3>.
					offset = 3 - heading.Level
					firstHeading = false
				}
				newLevel := heading.Level + offset

				htmltag := &markdown.HTMLBlock{}
				var buf bytes.Buffer
				// TODO(matloob): Do we want the div and h elements to have analogous classes?
				// Currently we're using newLevel for the div's class but n.Level for the h element's
				// class.
				if newLevel > 6 {
					fmt.Fprintf(&buf, `<div class="h%d" role="heading" aria-level="%d"`, newLevel, heading.Level)
				} else {
					fmt.Fprintf(&buf, `<h%d class="h%d"`, newLevel, heading.Level)
				}
				if heading.ID != "" {
					fmt.Fprintf(&buf, ` id="%s"`, htmlQuoteEscaper.Replace(heading.ID))
				}
				buf.WriteByte('>')
				heading.Text.PrintHTML(&buf)
				if newLevel > 6 {
					_, _ = buf.WriteString("</div>")
				} else {
					fmt.Fprintf(&buf, "</h%d>", newLevel)
				}
				htmltag.Text = append(htmltag.Text, buf.String())
				blocks[i] = htmltag
			}
		}
	}
	rewriteHeadingsBlocks(doc.Blocks)
}

var htmlQuoteEscaper = strings.NewReplacer(
	"\"", "&quot;",
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

// rewriteHeadingIDs generates ids based on the body of the heading.
// The original code uses the raw markdown as the input to the ids.Generate
// function, but we don't have the raw markdown anymore, so we use the
// text instead.
func rewriteHeadingIDs(doc *markdown.Document) {
	ids := &ids{
		values: map[string]bool{},
	}
	walkBlocks(doc.Blocks, func(b markdown.Block) error {
		if heading, ok := b.(*markdown.Heading); ok {
			id := ids.generateID(heading, "heading")
			heading.ID = string(id)
		}
		return nil
	})
}
