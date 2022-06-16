// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"fmt"
	"go/doc/comment"
	"regexp"
	"strings"
	"unicode"

	safe "github.com/google/safehtml"
	"github.com/google/safehtml/template"
)

type link struct {
	Class string
	Href  string
	Text  any // string or safe.HTML
}

type heading struct {
	ID    safe.Identifier
	Title safe.HTML
}

var (
	// tocTemplate expects a []heading.
	tocTemplate = template.Must(template.New("toc").Parse(`<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc{{if gt (len .) 5}} Documentation-toc-columns{{end}}">
    {{range . -}}
      <li class="Documentation-tocItem">
        <a href="#{{.ID}}">{{.Title}}</a>
      </li>
    {{end -}}
  </ul>
</div>
`))

	italicTemplate = template.Must(template.New("italics").Parse(`<i>{{.}}</i>`))

	codeTemplate = template.Must(template.New("code").Parse(`<pre>{{.}}</pre>`))

	paraTemplate = template.Must(template.New("para").Parse("<p>{{.}}\n</p>"))

	headingTemplate = template.Must(template.New("heading").Parse(
		`<h4 id="{{.ID}}">{{.Title}} <a class="Documentation-idLink" href="#{{.ID}}">¶</a></h4>`))

	linkTemplate = template.Must(template.New("link").Parse(
		`<a{{with .Class}}class="{{.}}" {{end}} href="{{.Href}}">{{.Text}}</a>`))

	uListTemplate = template.Must(template.New("ulist").Parse(
		`<ul>
{{- range .}}
  {{.}}
{{- end}}
</ul>`))

	oListTemplate = template.Must(template.New("olist").Parse(
		`<ol>
		   {{range .}}
		     {{.}}
           {{end}}
         </ol>`))

	listItemTemplate = template.Must(template.New("li").Parse(
		`<li{{with .Number}}value="{{.}}" {{end}}>{{.Content}}</li>`))
)

func (r *Renderer) formatDocHTML(text string, extractLinks bool) safe.HTML {
	p := comment.Parser{}
	doc := p.Parse(text)
	if extractLinks {
		r.removeLinks(doc)
	}
	var headings []heading
	for _, b := range doc.Content {
		if h, ok := b.(*comment.Heading); ok {
			headings = append(headings, r.newHeading(h))
		}
	}
	h := r.blocksToHTML(doc.Content, true, extractLinks)
	if r.enableCommandTOC && len(headings) > 0 {
		h = safe.HTMLConcat(ExecuteToHTML(tocTemplate, headings), h)
	}
	return h
}

func (r *Renderer) removeLinks(doc *comment.Doc) {
	var bs []comment.Block
	inLinks := false
	for _, b := range doc.Content {
		switch b := b.(type) {
		case *comment.Heading:
			if textsToString(b.Text) == "Links" {
				inLinks = true
			} else {
				inLinks = false
				bs = append(bs, b)
			}
		case *comment.List:
			if inLinks {
				for _, item := range b.Items {
					fmt.Println("    ", item)
					if link, ok := itemLink(item); ok {
						r.links = append(r.links, link)
					}
				}
			} else {
				bs = append(bs, b)
			}
		case *comment.Paragraph:
			if inLinks {
				// Links section doesn't require leading whitespace, so
				// the link may be in a paragraph.
				s := textsToString(b.Text)
				r.links = append(r.links, parseLinks(strings.Split(s, "\n"))...)
			} else {
				bs = append(bs, b)
			}

		default:
			if !inLinks {
				bs = append(bs, b)
			}
		}
	}
	doc.Content = bs
}

func itemLink(item *comment.ListItem) (l Link, ok bool) {
	// Should be a single Paragraph.
	if len(item.Content) != 1 {
		return l, false
	}
	p, ok := item.Content[0].(*comment.Paragraph)
	if !ok {
		return l, false
	}
	// TODO: clean up.
	if lp := parseLink("- " + textsToString(p.Text)); lp != nil {
		return *lp, true
	}
	return l, false
}

func (r *Renderer) blocksToHTML(bs []comment.Block, useParagraph, extractLinks bool) safe.HTML {
	return concatHTML(bs, func(b comment.Block) safe.HTML {
		return r.blockToHTML(b, useParagraph, extractLinks)
	})
}

func (r *Renderer) blockToHTML(b comment.Block, useParagraph, extractLinks bool) safe.HTML {
	switch b := b.(type) {
	case *comment.Paragraph:
		th := r.textsToHTML(b.Text)
		if useParagraph {
			return ExecuteToHTML(paraTemplate, th)
		}
		return th

	case *comment.Code:
		return ExecuteToHTML(codeTemplate, b.Text)

	case *comment.Heading:
		return ExecuteToHTML(headingTemplate, r.newHeading(b))

	case *comment.List:
		var items []safe.HTML
		useParagraph = b.BlankBetween()
		for _, item := range b.Items {
			items = append(items, ExecuteToHTML(listItemTemplate, struct {
				Number  string
				Content safe.HTML
			}{item.Number, r.blocksToHTML(item.Content, useParagraph, false)}))
		}
		t := oListTemplate
		if b.Items[0].Number == "" {
			t = uListTemplate
		}
		return ExecuteToHTML(t, items)
	default:
		return badType(b)
	}
}

func (r *Renderer) newHeading(h *comment.Heading) heading {
	return heading{headingID(h), r.textsToHTML(h.Text)}
}

func (r *Renderer) textsToHTML(ts []comment.Text) safe.HTML {
	return concatHTML(ts, r.textToHTML)
}

func (r *Renderer) textToHTML(t comment.Text) safe.HTML {
	switch t := t.(type) {
	case comment.Plain:
		// Don't auto-link URLs. The doc/comment package already does that.
		return linkRFCs(string(t))
	case comment.Italic:
		return ExecuteToHTML(italicTemplate, t)
	case *comment.Link:
		return ExecuteToHTML(linkTemplate, link{"", t.URL, r.textsToHTML(t.Text)})
	case *comment.DocLink:
		url := r.docLinkURL(t)
		return ExecuteToHTML(linkTemplate, link{"", url, r.textsToHTML(t.Text)})
	default:
		return badType(t)
	}
}

func (r *Renderer) docLinkURL(dl *comment.DocLink) string {
	var url string
	if dl.ImportPath != "" {
		url = "/" + dl.ImportPath
		if r.packageURL != nil {
			url = r.packageURL(dl.ImportPath)
		}
	}
	id := dl.Name
	if dl.Recv != "" {
		id = dl.Recv + "." + id
	}
	if id != "" {
		url += "#" + id
	}
	return url
}

// TODO: any -> *comment.Text | *comment.Block
func concatHTML[T any](xs []T, toHTML func(T) safe.HTML) safe.HTML {
	var hs []safe.HTML
	for _, x := range xs {
		hs = append(hs, toHTML(x))
	}
	return safe.HTMLConcat(hs...)
}

func badType(x interface{}) safe.HTML {
	return safe.HTMLEscaped(fmt.Sprintf("bad type %T", x))
}

func headingID(h *comment.Heading) safe.Identifier {
	s := textsToString(h.Text)
	id := badAnchorRx.ReplaceAllString(s, "_")
	return safe.IdentifierFromConstantPrefix("hdr", id)
}

func textsToString(ts []comment.Text) string {
	var b strings.Builder
	for _, t := range ts {
		switch t := t.(type) {
		case comment.Plain:
			b.WriteString(string(t))
		case comment.Italic:
			b.WriteString(string(t))
		case *comment.Link:
			b.WriteString(textsToString(t.Text))
		case *comment.DocLink:
			b.WriteString(textsToString(t.Text))
		default:
			fmt.Fprintf(&b, "bad text type %T", t)
		}
	}
	return b.String()
}

var rfcRegexp = regexp.MustCompile(rfcRx)

// TODO: merge/replace Renderer.formatLineHTML.
// TODO: make more efficient.
func linkRFCs(s string) safe.HTML {
	var hs []safe.HTML
	for len(s) > 0 {
		m0, m1 := len(s), len(s)
		if m := rfcRegexp.FindStringIndex(s); m != nil {
			m0, m1 = m[0], m[1]
		}
		if m0 > 0 {
			hs = append(hs, safe.HTMLEscaped(s[:m0]))
		}
		if m1 > m0 {
			word := s[m0:m1]
			// Strip all characters except for letters, numbers, and '.' to
			// obtain RFC fields.
			rfcFields := strings.FieldsFunc(word, func(c rune) bool {
				return !unicode.IsLetter(c) && !unicode.IsNumber(c) && c != '.'
			})
			var url string
			if len(rfcFields) >= 4 {
				// RFC x Section y
				url = fmt.Sprintf("https://rfc-editor.org/rfc/rfc%s.html#section-%s",
					rfcFields[1], rfcFields[3])
			} else if len(rfcFields) >= 2 {
				url = fmt.Sprintf("https://rfc-editor.org/rfc/rfc%s.html", rfcFields[1])
			}
			if url != "" {
				hs = append(hs, ExecuteToHTML(linkTemplate, link{"", url, word}))
			}
		}
		s = s[m1:]
	}
	return safe.HTMLConcat(hs...)
}
