// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/doc/comment"
	"go/format"
	"go/printer"
	"go/scanner"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	safe "github.com/google/safehtml"
	"github.com/google/safehtml/legacyconversions"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
	"golang.org/x/pkgsite/internal/log"
)

/*
This logic is responsible for converting documentation comments and AST nodes
into formatted HTML. This relies on identifierResolver.toHTML to do the work
of converting words into links.
*/

// TODO(golang.org/issue/17056): Support hiding deprecated declarations.

const (
	// Regexp for URLs.
	// Match any ".,:;?!" within path, but not at end (see #18139, #16565).
	// This excludes some rare yet valid URLs ending in common punctuation
	// in order to allow sentences ending in URLs.
	urlRx = protoPart + `://` + hostPart + pathPart

	// Protocol (e.g. "http").
	protoPart = `(https?|s?ftps?|file|gopher|mailto|nntp)`
	// Host (e.g. "www.example.com" or "[::1]:8080").
	hostPart = `([a-zA-Z0-9_@\-.\[\]:]+)`
	// Optional path, query, fragment (e.g. "/path/index.html?q=foo#bar").
	pathPart = `([.,:;?!]*[a-zA-Z0-9$'()*+&#=@~_/\-\[\]%])*`

	// Regexp for RFCs.
	rfcRx = `RFC\s+(\d{3,5})(,?\s+[Ss]ection\s+(\d+(\.\d+)*))?`
)

var (
	matchRx     = regexp.MustCompile(urlRx + `|` + rfcRx)
	badAnchorRx = regexp.MustCompile(`[^a-zA-Z0-9]`)
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

// parseLinks extracts links from lines.
func parseLinks(lines []string) []Link {
	var links []Link
	for _, l := range lines {
		if link := parseLink(l); link != nil {
			links = append(links, *link)
		}
	}
	return links
}

// If line is of the form "- title, url", then parseLink returns
// a Link with the title and url. Otherwise it returns nil.
// The line already has leading whitespace trimmed.
func parseLink(line string) *Link {
	if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "-\t") {
		return nil
	}
	text, href, found := strings.Cut(line[2:], ",")
	if !found {
		return nil
	}
	return &Link{
		Text: strings.TrimSpace(text),
		Href: strings.TrimSpace(href),
	}
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

func (r *Renderer) declHTML(doc string, decl ast.Decl, extractLinks bool) (out struct{ Doc, Decl safe.HTML }) {
	if doc != "" {
		out.Doc = r.formatDocHTML(doc, extractLinks)
	}
	if decl != nil {
		idr := &identifierResolver{r.pids, newDeclIDs(decl), r.packageURL}
		out.Decl = r.formatDeclHTML(decl, idr)
	}
	return out
}

func (r *Renderer) codeString(ex *doc.Example) (string, error) {
	if ex == nil || ex.Code == nil {
		return "", errors.New("please include an example with code")
	}
	var buf bytes.Buffer

	if ex.Play != nil {
		if err := format.Node(&buf, r.fset, ex.Play); err != nil {
			return "", err
		}
	} else {
		n := &printer.CommentedNode{
			Node:     ex.Code,
			Comments: ex.Comments,
		}
		if err := format.Node(&buf, r.fset, n); err != nil {
			return "", err
		}
	}

	return buf.String(), nil
}

func (r *Renderer) codeHTML(ex *doc.Example) safe.HTML {
	codeStr, err := r.codeString(ex)
	if err != nil {
		log.Errorf(r.ctx, "Error converting *doc.Example into string: %v", err)
		return template.MustParseAndExecuteToHTML(`<pre class="Documentation-exampleCode">Error rendering example code.</pre>`)
	}
	return codeHTML(codeStr, r.exampleTmpl)
}

type codeElement struct {
	Text    string
	Comment bool
}

func codeHTML(src string, codeTmpl *template.Template) safe.HTML {
	var els []codeElement
	// If code is an *ast.BlockStmt, then trim the braces.
	var indent string
	if len(src) >= 4 && strings.HasPrefix(src, "{\n") && strings.HasSuffix(src, "\n}") {
		src = strings.Trim(src[2:len(src)-2], "\n")
		indent = src[:indentLength(src)]
		if len(indent) > 0 {
			src = strings.TrimPrefix(src, indent) // handle remaining indents later
		}
	}

	// Scan through the source code, adding comment spans for comments,
	// and stripping the trailing example output.
	var lastOffset int        // last src offset copied to output buffer
	var outputOffset int = -1 // index in els of last output comment
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	s.Init(file, []byte(src), nil, scanner.ScanComments)
	indent = "\n" + indent // prepend newline for easier search-and-replace.
scan:
	for {
		p, tok, lit := s.Scan()
		offset := file.Offset(p) // current offset into source file
		prev := src[lastOffset:offset]
		prev = strings.Replace(prev, indent, "\n", -1)
		els = append(els, codeElement{prev, false})
		lastOffset = offset
		switch tok {
		case token.EOF:
			break scan
		case token.COMMENT:
			if exampleOutputRx.MatchString(lit) {
				outputOffset = len(els)
			}
			lit = strings.Replace(lit, indent, "\n", -1)
			els = append(els, codeElement{lit, true})
			lastOffset += len(lit)
		case token.STRING:
			// Avoid replacing indents in multi-line string literals.
			els = append(els, codeElement{lit, false})
			lastOffset += len(lit)
		}
	}

	if outputOffset >= 0 {
		els = els[:outputOffset]
	}
	// Trim trailing newlines.
	if len(els) > 0 {
		els[len(els)-1].Text = strings.TrimRight(els[len(els)-1].Text, "\n")
	}
	return ExecuteToHTML(codeTmpl, els)
}

// formatLineHTML formats the line as HTML-annotated text.
// URLs and Go identifiers are linked to corresponding declarations.
// If pre is true no conversion of doubled ` and ' to “ and ” is performed.
func (r *Renderer) formatLineHTML(line string, pre bool) safe.HTML {
	var htmls []safe.HTML
	addLink := func(href, text string) {
		htmls = append(htmls, ExecuteToHTML(LinkTemplate, Link{Href: href, Text: text}))
	}

	if !pre {
		line = convertQuotes(line)
	}
	for len(line) > 0 {
		m0, m1 := len(line), len(line)
		if m := matchRx.FindStringIndex(line); m != nil {
			m0, m1 = m[0], m[1]
		}
		if m0 > 0 {
			nonWord := line[:m0]
			htmls = append(htmls, safe.HTMLEscaped(nonWord))
		}
		if m1 > m0 {
			word := line[m0:m1]
			switch {
			case strings.Contains(word, "://"):
				// Forbid closing brackets without prior opening brackets.
				// See https://golang.org/issue/22285.
				if i := strings.IndexByte(word, ')'); i >= 0 && i < strings.IndexByte(word, '(') {
					m1 = m0 + i
					word = line[m0:m1]
				}
				if i := strings.IndexByte(word, ']'); i >= 0 && i < strings.IndexByte(word, '[') {
					m1 = m0 + i
					word = line[m0:m1]
				}

				// Require balanced pairs of parentheses.
				// See https://golang.org/issue/5043.
				for i := 0; strings.Count(word, "(") != strings.Count(word, ")") && i < 10; i++ {
					m1 = strings.LastIndexAny(line[:m1], "()")
					word = line[m0:m1]
				}
				for i := 0; strings.Count(word, "[") != strings.Count(word, "]") && i < 10; i++ {
					m1 = strings.LastIndexAny(line[:m1], "[]")
					word = line[m0:m1]
				}

				addLink(word, word)
			// Match "RFC ..." to link RFCs.
			case strings.HasPrefix(word, "RFC") && len(word) > 3 && unicode.IsSpace(rune(word[3])):
				// Strip all characters except for letters, numbers, and '.' to
				// obtain RFC fields.
				rfcFields := strings.FieldsFunc(word, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsNumber(c) && c != '.'
				})
				if len(rfcFields) >= 4 {
					// RFC x Section y
					addLink(fmt.Sprintf("https://rfc-editor.org/rfc/rfc%s.html#section-%s", rfcFields[1], rfcFields[3]), word)
				} else if len(rfcFields) >= 2 {
					// RFC x
					addLink(fmt.Sprintf("https://rfc-editor.org/rfc/rfc%s.html", rfcFields[1]), word)
				}
			default:
				htmls = append(htmls, safe.HTMLEscaped(word))
			}
		}
		line = line[m1:]
	}
	return safe.HTMLConcat(htmls...)
}

func ExecuteToHTML(tmpl *template.Template, data interface{}) safe.HTML {
	h, err := tmpl.ExecuteToHTML(data)
	if err != nil {
		return safe.HTMLEscaped("[" + err.Error() + "]")
	}
	return h
}

// formatDeclHTML formats the decl as HTML-annotated source code for the
// provided decl. Type identifiers are linked to corresponding declarations.
func (r *Renderer) formatDeclHTML(decl ast.Decl, idr *identifierResolver) safe.HTML {
	// Generate all anchor points and links for the given decl.
	anchorPointsMap := generateAnchorPoints(decl)
	anchorLinksMap := generateAnchorLinks(idr, decl)

	// Convert the maps (keyed by *ast.Ident) to slices of idKinds or URLs.
	//
	// This relies on the ast.Inspect and scanner.Scanner both
	// visiting *ast.Ident and token.IDENT nodes in the same order.
	var anchorPoints []idKind
	var anchorLinks []string
	ast.Inspect(decl, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			anchorPoints = append(anchorPoints, anchorPointsMap[id])
			anchorLinks = append(anchorLinks, anchorLinksMap[id])
		}
		return true
	})

	// Trim large string literals and composite literals.
	const (
		maxStringSize = 125
		maxElements   = 100
	)
	decl = rewriteDecl(decl, maxStringSize, maxElements)
	// Format decl as Go source code file.
	p := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	var b bytes.Buffer
	p.Fprint(&b, r.fset, decl)
	src := b.Bytes()
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), b.Len())

	// anchorLines is a list of anchor IDs that should be placed for each line.
	// lineTypes is a list of the type (e.g., comment or code) of each line.
	type lineType byte
	const codeType, commentType lineType = 1 << 0, 1 << 1 // may OR together
	numLines := bytes.Count(src, []byte("\n")) + 1
	anchorLines := make([][]idKind, numLines)
	lineTypes := make([]lineType, numLines)
	htmlLines := make([][]safe.HTML, numLines)

	// Scan through the source code, appropriately annotating it with HTML spans
	// for comments, and HTML links and anchors for relevant identifiers.
	var idIdx int      // current index in anchorPoints and anchorLinks
	var lastOffset int // last src offset copied to output buffer
	var s scanner.Scanner
	s.Init(file, src, nil, scanner.ScanComments)
scan:
	for {
		p, tok, lit := s.Scan()
		line := file.Line(p) - 1 // current 0-indexed line number
		offset := file.Offset(p) // current offset into source file
		tokType := codeType      // current token type (assume source code)

		// Add traversed bytes from src to the appropriate line.
		prevLines := strings.SplitAfter(string(src[lastOffset:offset]), "\n")
		for i, ln := range prevLines {
			n := line - len(prevLines) + i + 1
			if n < 0 { // possible at EOF
				n = 0
			}
			htmlLines[n] = append(htmlLines[n], safe.HTMLEscaped(ln))
		}

		lastOffset = offset
		switch tok {
		case token.EOF:
			break scan
		case token.COMMENT:
			tokType = commentType
			htmlLines[line] = append(htmlLines[line],
				template.MustParseAndExecuteToHTML(`<span class="comment">`),
				r.formatLineHTML(lit, false),
				template.MustParseAndExecuteToHTML(`</span>`))
			lastOffset += len(lit)
		case token.IDENT:
			if idIdx < len(anchorPoints) && anchorPoints[idIdx].ID.String() != "" {
				anchorLines[line] = append(anchorLines[line], anchorPoints[idIdx])
			}
			if idIdx < len(anchorLinks) && anchorLinks[idIdx] != "" {
				htmlLines[line] = append(htmlLines[line], ExecuteToHTML(LinkTemplate, Link{Href: anchorLinks[idIdx], Text: lit}))
				lastOffset += len(lit)
			}
			idIdx++
		}
		for i := strings.Count(strings.TrimSuffix(lit, "\n"), "\n"); i >= 0; i-- {
			lineTypes[line+i] |= tokType
		}
	}

	// Move anchor points up to the start of a comment
	// if the next line has no anchors.
	for i := range anchorLines {
		if i+1 == len(anchorLines) || len(anchorLines[i+1]) == 0 {
			j := i
			for j > 0 && lineTypes[j-1] == commentType {
				j--
			}
			anchorLines[i], anchorLines[j] = anchorLines[j], anchorLines[i]
		}
	}

	// Emit anchor IDs and data-kind attributes for each relevant line.
	var htmls []safe.HTML
	for line, iks := range anchorLines {
		inAnchor := false
		for _, ik := range iks {
			// Attributes for types and functions are handled in the template
			// that generates the full documentation HTML.
			if ik.Kind == "function" || ik.Kind == "type" {
				continue
			}
			// Top-level methods are handled in the template, but interface methods
			// are handled here.
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil {
				continue
			}
			htmls = append(htmls, ExecuteToHTML(anchorTemplate, ik))
			inAnchor = true
		}
		htmls = append(htmls, htmlLines[line]...)
		if inAnchor {
			htmls = append(htmls, template.MustParseAndExecuteToHTML("</span>"))
		}
	}
	return safe.HTMLConcat(htmls...)
}

var anchorTemplate = template.Must(template.New("anchor").Parse(`<span id="{{.ID}}" data-kind="{{.Kind}}">`))

// rewriteDecl rewrites n by removing strings longer than maxStringSize and
// composite literals longer than maxElements.
func rewriteDecl(n ast.Decl, maxStringSize, maxElements int) ast.Decl {
	v := &rewriteVisitor{maxStringSize, maxElements}
	ast.Walk(v, n)
	return n
}

type rewriteVisitor struct {
	maxStringSize, maxElements int
}

func (v *rewriteVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.ValueSpec:
		for _, val := range n.Values {
			v.rewriteLongValue(val, &n.Comment)
		}
	case *ast.Field:
		if n.Tag != nil {
			v.rewriteLongValue(n.Tag, &n.Comment)
		}
	}
	return v
}

func (v *rewriteVisitor) rewriteLongValue(n ast.Node, pcg **ast.CommentGroup) {
	switch n := n.(type) {
	case *ast.BasicLit:
		if n.Kind != token.STRING {
			return
		}
		size := len(n.Value) - 2 // subtract quotation marks
		if size <= v.maxStringSize {
			return
		}
		addComment(pcg, n.ValuePos, fmt.Sprintf("/* %d-byte string literal not displayed */", size))
		if len(n.Value) == 0 {
			// Impossible, but avoid the panic just in case.
			return
		}
		if quote := n.Value[0]; quote == '`' {
			n.Value = "``"
		} else {
			n.Value = `""`
		}
	case *ast.CompositeLit:
		if len(n.Elts) > v.maxElements {
			addComment(pcg, n.Lbrace, fmt.Sprintf("/* %d elements not displayed */", len(n.Elts)))
			n.Elts = n.Elts[:0]
		}
	}
}

func addComment(cg **ast.CommentGroup, pos token.Pos, text string) {
	if *cg == nil {
		*cg = &ast.CommentGroup{}
	}
	(*cg).List = append((*cg).List, &ast.Comment{Slash: pos, Text: text})
}

// An idKind holds an anchor ID and the kind of the identifier being anchored.
// The valid kinds are: "constant", "variable", "type", "function", "method" and "field".
type idKind struct {
	ID   safe.Identifier
	Kind string
}

// SafeGoID constructs a safe identifier from a Go symbol or dotted concatenation of symbols
// (e.g. "Time.Equal").
func SafeGoID(s string) safe.Identifier {
	ValidateGoDottedExpr(s)
	return legacyconversions.RiskilyAssumeIdentifier(s)
}

var badIDRx = regexp.MustCompile(`[^_\pL\pN.]`)

// ValidateGoDottedExpr panics if s contains characters other than '.' plus the valid Go identifier characters.
func ValidateGoDottedExpr(s string) {
	if badIDRx.MatchString(s) {
		panic(fmt.Sprintf("invalid identifier characters: %q", s))
	}
}

// generateAnchorPoints returns a mapping of *ast.Ident objects to the
// qualified ID that should be set as an anchor point, as well as the kind
// of identifier, used in the data-kind attribute.
func generateAnchorPoints(decl ast.Decl) map[*ast.Ident]idKind {
	m := map[*ast.Ident]idKind{}
	switch decl := decl.(type) {
	case *ast.GenDecl:
		for _, sp := range decl.Specs {
			switch decl.Tok {
			case token.CONST, token.VAR:
				kind := "constant"
				if decl.Tok == token.VAR {
					kind = "variable"
				}
				for _, name := range sp.(*ast.ValueSpec).Names {
					m[name] = idKind{SafeGoID(name.Name), kind}
				}
			case token.TYPE:
				ts := sp.(*ast.TypeSpec)
				m[ts.Name] = idKind{SafeGoID(ts.Name.Name), "type"}

				var fs []*ast.Field
				var kind string
				switch tx := ts.Type.(type) {
				case *ast.StructType:
					fs = tx.Fields.List
					kind = "field"
				case *ast.InterfaceType:
					fs = tx.Methods.List
					kind = "method"
				}
				for _, f := range fs {
					for _, id := range f.Names {
						m[id] = idKind{SafeGoID(ts.Name.String() + "." + id.String()), kind}
					}
					// if f.Names == nil, we have an embedded struct field or embedded
					// interface.
					//
					// Don't generate anchor points for embedded interfaces. They
					// aren't interesting in and of themselves; they just represent an
					// additional list of methods added to the interface.
					//
					// Do generate anchor points for embedded fields: they are
					// interesting, because their names can be used in selector
					// expressions and struct literals.
					if f.Names == nil && kind == "field" {
						// The name of an embedded field is the type name.
						typeName, id := nodeName(f.Type)
						typeName = typeName[strings.LastIndexByte(typeName, '.')+1:]
						m[id] = idKind{SafeGoID(ts.Name.String() + "." + typeName), kind}
					}
				}
			}
		}
	case *ast.FuncDecl:
		anchorID := decl.Name.Name
		kind := "function"
		if decl.Recv != nil && len(decl.Recv.List) > 0 {
			recvName, _ := nodeName(decl.Recv.List[0].Type)
			recvName = recvName[strings.LastIndexByte(recvName, '.')+1:]
			anchorID = recvName + "." + anchorID
			kind = "method"
		}
		m[decl.Name] = idKind{SafeGoID(anchorID), kind}
	}
	return m
}

// generateAnchorLinks returns a mapping of *ast.Ident objects to the URL
// that the identifier should link to.
func generateAnchorLinks(idr *identifierResolver, decl ast.Decl) map[*ast.Ident]string {
	m := map[*ast.Ident]string{}
	ignore := map[ast.Node]bool{}
	ast.Inspect(decl, func(node ast.Node) bool {
		if ignore[node] {
			return false
		}
		switch node := node.(type) {
		case *ast.SelectorExpr:
			// Package qualified identifier (e.g., "io.EOF").
			if prefix, _ := node.X.(*ast.Ident); prefix != nil {
				if obj := prefix.Obj; obj != nil && obj.Kind == ast.Pkg {
					if spec, _ := obj.Decl.(*ast.ImportSpec); spec != nil {
						if path, err := strconv.Unquote(spec.Path.Value); err == nil {
							// Register two links, one for the package
							// and one for the qualified identifier.
							m[prefix] = idr.toURL(path, "")
							m[node.Sel] = idr.toURL(path, node.Sel.Name)
							return false
						}
					}
				}
			}
		case *ast.Ident:
			if node.Obj == nil && doc.IsPredeclared(node.Name) {
				m[node] = idr.toURL("builtin", node.Name)
			} else if node.Obj != nil && idr.topLevelDecls[node.Obj.Decl] {
				m[node] = "#" + node.Name
			}
		case *ast.FuncDecl:
			ignore[node.Name] = true // E.g., "func NoLink() int"
		case *ast.TypeSpec:
			ignore[node.Name] = true // E.g., "type NoLink int"
		case *ast.ValueSpec:
			for _, n := range node.Names {
				ignore[n] = true // E.g., "var NoLink1, NoLink2 int"
			}
		case *ast.AssignStmt:
			for _, n := range node.Lhs {
				ignore[n] = true // E.g., "NoLink1, NoLink2 := 0, 1"
			}
		}
		return true
	})
	return m
}

const (
	ulquo = "“"
	urquo = "”"
)

var unicodeQuoteReplacer = strings.NewReplacer("``", ulquo, "''", urquo)

// convertQuotes turns doubled ` and ' into “ and ”.
func convertQuotes(text string) string {
	return unicodeQuoteReplacer.Replace(text)
}
