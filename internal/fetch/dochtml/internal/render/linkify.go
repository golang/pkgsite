// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/scanner"
	"go/token"
	"html/template"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/safehtml"
	safetemplate "github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/fetch/internal/doc"
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

	// Regexp for Go identifiers.
	identRx     = `[\pL_][\pL_0-9]*`
	qualIdentRx = identRx + `(\.` + identRx + `)*`

	// Regexp for RFCs.
	rfcRx = `RFC\s+(\d{3,5})(,?\s+[Ss]ection\s+(\d+(\.\d+)*))?`
)

var (
	matchRx     = regexp.MustCompile(urlRx + `|` + rfcRx + `|` + qualIdentRx)
	badAnchorRx = regexp.MustCompile(`[^a-zA-Z0-9]`)
)

func (r *Renderer) declHTML(doc string, decl ast.Decl) (out struct{ Doc, Decl template.HTML }) {
	dids := newDeclIDs(decl)
	idr := &identifierResolver{r.pids, dids, r.packageURL}
	if doc != "" {
		var b bytes.Buffer
		for _, blk := range docToBlocks(doc) {
			switch blk := blk.(type) {
			case *paragraph:
				b.WriteString("<p>\n")
				for _, line := range blk.lines {
					r.formatLineHTML(&b, line, idr)
					b.WriteString("\n")
				}
				b.WriteString("</p>\n")
			case *preformat:
				b.WriteString("<pre>\n")
				for _, line := range blk.lines {
					r.formatLineHTML(&b, line, nil)
					b.WriteString("\n")
				}
				b.WriteString("</pre>\n")
			case *heading:
				id := badAnchorRx.ReplaceAllString(blk.title, "_")
				sid := safehtml.IdentifierFromConstantPrefix("hdr", id)
				b.WriteString(`<h3 id="` + sid.String() + `">`)
				b.WriteString(template.HTMLEscapeString(blk.title))
				if !r.disablePermalinks {
					b.WriteString(` <a href="#` + sid.String() + `">¶</a>`)
				}
				b.WriteString("</h3>\n")
			}
		}
		out.Doc = template.HTML(b.String())
	}
	if decl != nil {
		var b bytes.Buffer
		b.WriteString("<pre>\n")
		r.formatDeclHTML(&b, decl, idr)
		b.WriteString("</pre>\n")
		out.Decl = template.HTML(b.String())
	}
	return out
}

func (r *Renderer) codeHTML(code interface{}) safehtml.HTML {
	// TODO: Should we perform hotlinking for comments and code?
	if code == nil {
		return safehtml.HTML{}
	}

	var b bytes.Buffer
	p := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	p.Fprint(&b, r.fset, code)
	return codeHTML(b.String())
}

type codeElement struct {
	Text    string
	Comment bool
}

var codeTmpl = safetemplate.Must(safetemplate.New("").Parse(`
<pre>
{{range .}}
  {{- if .Comment -}}
    <span class="comment">{{.Text}}</span>
  {{- else -}}
    {{.Text}}
  {{- end -}}
{{end}}
</pre>
`))

func codeHTML(src string) safehtml.HTML {
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
			if exampleOutputRx.MatchString(lit) && outputOffset == -1 {
				outputOffset = len(els)
			}
			lit = strings.Replace(lit, indent, "\n", -1)
			els = append(els, codeElement{lit, true})
			lastOffset += len(lit)
		case token.STRING:
			// Avoid replacing indents in multi-line string literals.
			outputOffset = -1
			els = append(els, codeElement{lit, false})
			lastOffset += len(lit)
		default:
			outputOffset = -1
		}
	}

	if outputOffset >= 0 {
		els = els[:outputOffset]
	}
	// Trim trailing newlines.
	if len(els) > 0 {
		els[len(els)-1].Text = strings.TrimRight(els[len(els)-1].Text, "\n")
	}
	h, err := codeTmpl.ExecuteToHTML(els)
	if err != nil {
		h = safehtml.HTMLEscaped("[" + err.Error() + "]")
	}
	return h
}

// formatLineHTML formats the line as HTML-annotated text.
// URLs and Go identifiers are linked to corresponding declarations.
func (r *Renderer) formatLineHTML(w io.Writer, line string, idr *identifierResolver) {
	var lastChar, nextChar byte
	var numQuotes int
	for len(line) > 0 {
		m0, m1 := len(line), len(line)
		if m := matchRx.FindStringIndex(line); m != nil {
			m0, m1 = m[0], m[1]
		}
		if m0 > 0 {
			nonWord := line[:m0]
			io.WriteString(w, template.HTMLEscapeString(nonWord))
			lastChar = nonWord[len(nonWord)-1]
			numQuotes += countQuotes(nonWord)
		}
		if m1 > m0 {
			word := line[m0:m1]
			nextChar = 0
			if m1 < len(line) {
				nextChar = line[m1]
			}

			// Reduce false-positives by having a list of allowed
			// characters preceding and succeeding an identifier.
			// Also, forbid ID linking within unbalanced quotes on same line.
			validPrefix := strings.IndexByte("\x00 \t()[]*\n", lastChar) >= 0
			validSuffix := strings.IndexByte("\x00 \t()[]:;,.'\n", nextChar) >= 0
			forbidLinking := !validPrefix || !validSuffix || numQuotes%2 != 0

			// TODO: Should we provide hotlinks for related packages?

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

				word := template.HTMLEscapeString(word)
				fmt.Fprintf(w, `<a href="%s">%s</a>`, word, word)
			// Match "RFC ..." to link RFCs.
			case strings.HasPrefix(word, "RFC") && len(word) > 3 && unicode.IsSpace(rune(word[3])):
				// Strip all characters except for letters, numbers, and '.' to
				// obtain RFC fields.
				rfcFields := strings.FieldsFunc(word, func(c rune) bool {
					return !unicode.IsLetter(c) && !unicode.IsNumber(c) && c != '.'
				})
				word := template.HTMLEscapeString(word)
				if len(rfcFields) >= 4 {
					// RFC x Section y
					fmt.Fprintf(w, `<a href="https://rfc-editor.org/rfc/rfc%s.html#section-%s">%s</a>`,
						rfcFields[1], rfcFields[3], word)
				} else if len(rfcFields) >= 2 {
					// RFC x
					fmt.Fprintf(w, `<a href="https://rfc-editor.org/rfc/rfc%s.html">%s</a>`,
						rfcFields[1], word)
				}
			case !forbidLinking && !r.disableHotlinking && idr != nil: // && numQuotes%2 == 0:
				io.WriteString(w, idr.toHTML(word).String())
			default:
				io.WriteString(w, template.HTMLEscapeString(word))
			}
			numQuotes += countQuotes(word)
		}
		line = line[m1:]
	}
}

func countQuotes(s string) int {
	n := -1 // loop always iterates at least once
	for i := len(s); i >= 0; i = strings.LastIndexAny(s[:i], `"“”`) {
		n++
	}
	return n
}

// formatDeclHTML formats the decl as HTML-annotated source code for the
// provided decl. Type identifiers are linked to corresponding declarations.
func (r *Renderer) formatDeclHTML(w io.Writer, decl ast.Decl, idr *identifierResolver) {
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

	// Trim large string literals and slices.
	v := &declVisitor{}
	ast.Walk(v, decl)

	// Format decl as Go source code file.
	var b bytes.Buffer
	p := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	p.Fprint(&b, r.fset, &printer.CommentedNode{Node: decl, Comments: v.Comments})
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

	// Scan through the source code, appropriately annotating it with HTML spans
	// for comments, and HTML links and anchors for relevant identifiers.
	var bb bytes.Buffer // temporary output buffer
	var idIdx int       // current index in anchorPoints and anchorLinks
	var lastOffset int  // last src offset copied to output buffer
	var s scanner.Scanner
	s.Init(file, src, nil, scanner.ScanComments)
scan:
	for {
		p, tok, lit := s.Scan()
		line := file.Line(p) - 1 // current 0-indexed line number
		offset := file.Offset(p) // current offset into source file
		tokType := codeType      // current token type (assume source code)

		template.HTMLEscape(&bb, src[lastOffset:offset])
		lastOffset = offset
		switch tok {
		case token.EOF:
			break scan
		case token.COMMENT:
			tokType = commentType
			bb.WriteString(`<span class="comment">`)
			r.formatLineHTML(&bb, lit, idr)
			bb.WriteString(`</span>`)
			lastOffset += len(lit)
		case token.IDENT:
			if idIdx < len(anchorPoints) && anchorPoints[idIdx].id != "" {
				anchorLines[line] = append(anchorLines[line], anchorPoints[idIdx])
			}
			if idIdx < len(anchorLinks) && anchorLinks[idIdx] != "" {
				u := template.HTMLEscapeString(anchorLinks[idIdx])
				s := template.HTMLEscapeString(lit)
				fmt.Fprintf(&bb, `<a href="%s">%s</a>`, u, s)
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
	for _, iks := range anchorLines {
		for _, ik := range iks {
			// Attributes for types and functions are handled in the template
			// that generates the full documentation HTML.
			if ik.kind == "function" || ik.kind == "type" {
				continue
			}
			// Top-level methods are handled in the template, but interface methods
			// are handled here.
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil {
				continue
			}
			fmt.Fprintf(w, `<span id="%s" data-kind="%s"></span>`,
				template.HTMLEscapeString(ik.id), ik.kind)
		}
		b, _ := bb.ReadBytes('\n')
		w.Write(b) // write remainder of line (contains newline)
	}
}

// declVisitor is used to walk over the AST and trim large string
// literals and arrays before package documentation is rendered.
// Comments are added to Comments to indicate that a part of the
// original code is not displayed.
type declVisitor struct {
	Comments []*ast.CommentGroup
}

// Visit implements ast.Visitor.
func (v *declVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.BasicLit:
		if n.Kind == token.STRING && len(n.Value) > 128 {
			v.Comments = append(v.Comments,
				&ast.CommentGroup{List: []*ast.Comment{{
					Slash: n.Pos(),
					Text:  stringBasicLitSize(n.Value),
				}}})
			n.Value = `""`
		}
	case *ast.CompositeLit:
		if len(n.Elts) > 100 {
			v.Comments = append(v.Comments,
				&ast.CommentGroup{List: []*ast.Comment{{
					Slash: n.Lbrace,
					Text:  fmt.Sprintf("/* %d elements not displayed */", len(n.Elts)),
				}}})
			n.Elts = n.Elts[:0]
		}
	}
	return v
}

// stringBasicLitSize computes the number of bytes in the given string basic literal.
//
// See noder.basicLit and syntax.StringLit cases in cmd/compile/internal/gc/noder.go.
func stringBasicLitSize(s string) string {
	if len(s) > 0 && s[0] == '`' {
		// strip carriage returns from raw string
		s = strings.ReplaceAll(s, "\r", "")
	}
	u, err := strconv.Unquote(s)
	if err != nil {
		return fmt.Sprintf("/* invalid %d byte string literal not displayed */", len(s))
	}
	return fmt.Sprintf("/* %d byte string literal not displayed */", len(u))
}

// An idKind holds an anchor ID and the kind of the identifier being anchored.
// The valid kinds are: "constant", "variable", "type", "function", "method" and "field".
type idKind struct {
	id, kind string
}

// generateAnchorPoints returns a mapping of *ast.Ident objects to the
// qualified ID that should be set as an anchor point, as well as the kind
// of identifer, used in the data-kind attribute.
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
					m[name] = idKind{name.Name, kind}
				}
			case token.TYPE:
				ts := sp.(*ast.TypeSpec)
				m[ts.Name] = idKind{ts.Name.Name, "type"}

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
						m[id] = idKind{ts.Name.String() + "." + id.String(), kind}
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
						m[id] = idKind{ts.Name.String() + "." + typeName, kind}
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
		m[decl.Name] = idKind{anchorID, kind}
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
