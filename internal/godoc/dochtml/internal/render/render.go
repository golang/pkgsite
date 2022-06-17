// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package render formats Go documentation as HTML.
// It is an internal component that powers dochtml.
package render

import (
	"context"
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
)

var (
	// Regexp for example outputs.
	exampleOutputRx = regexp.MustCompile(`(?i)//[[:space:]]*(unordered )?output:`)
)

type Renderer struct {
	fset             *token.FileSet
	pids             *packageIDs
	packageURL       func(string) string
	enableCommandTOC bool
	ctx              context.Context
	docTmpl          *template.Template
	exampleTmpl      *template.Template
	links            []Link // Links removed from package overview to be displayed elsewhere.
}

type Options struct {
	// RelatedPackages is a list of related packages to use for hotlinking.
	// A recommended heuristic is to include all packages imported by the
	// given package, its tests, and its example tests.
	//
	// Only relevant for HTML formatting.
	RelatedPackages []*doc.Package

	// PackageURL is a function that given a package path,
	// returns a URL for navigating to the godoc for that package.
	//
	// Only relevant for HTML formatting.
	PackageURL func(pkgPath string) (url string)

	// EnableCommandTOC turns on the table of contents for the overview section
	// of command pages.
	//
	// Only relevant for HTML formatting.
	EnableCommandTOC bool
}

// docDataTmpl renders documentation. It expects a docData.
var docDataTmpl = template.Must(template.New("").Parse(`
{{- if and .EnableCommandTOC .Headings -}}
  <div role="navigation" aria-label="Table of Contents">
    <ul class="Documentation-toc{{if gt (len .Headings) 5}} Documentation-toc-columns{{end}}">
      {{range .Headings -}}
        <li class="Documentation-tocItem">
          <a href="#{{.ID}}">{{.Title}}</a>
        </li>
      {{end -}}
    </ul>
  </div>
{{end -}}
{{- range .Elements -}}
  {{- if .IsHeading -}}
    <h4 id="{{.ID}}">{{.Title}} <a class="Documentation-idLink" href="#{{.ID}}">¶</a></h4>
  {{- else if .IsPreformat -}}
    <pre>{{.Body}}</pre>
  {{- else -}}
    <p>{{.Body}}</p>
  {{- end -}}
{{- end -}}`))

// exampleTmpl renders code for an example. It expect an Example.
var exampleTmpl = template.Must(template.New("").Parse(`
<pre class="Documentation-exampleCode">
{{range .}}
	{{- .Text -}}
{{end}}
</pre>
`))

func New(ctx context.Context, fset *token.FileSet, pkg *doc.Package, opts *Options) *Renderer {
	var others []*doc.Package
	var packageURL func(string) string
	var enableCommandTOC bool
	if opts != nil {
		if len(opts.RelatedPackages) > 0 {
			others = opts.RelatedPackages
		}
		if opts.PackageURL != nil {
			packageURL = opts.PackageURL
		}
		enableCommandTOC = opts.EnableCommandTOC
	}
	pids := newPackageIDs(pkg, others...)

	return &Renderer{
		fset:             fset,
		pids:             pids,
		packageURL:       packageURL,
		enableCommandTOC: enableCommandTOC,
		docTmpl:          docDataTmpl,
		exampleTmpl:      exampleTmpl,
		ctx:              ctx,
	}
}

const maxSynopsisNodeDepth = 10

// ShortSynopsis returns a very short, one-line summary of the given input node.
// It currently only supports *ast.FuncDecl nodes and will return a non-nil
// error otherwise.
func (r *Renderer) ShortSynopsis(n ast.Node) (string, error) {
	return shortOneLineNodeDepth(r.fset, n, 0)
}

// Synopsis returns a one-line summary of the given input node.
func (r *Renderer) Synopsis(n ast.Node) string {
	return OneLineNodeDepth(r.fset, n, 0)
}

// DocHTML formats documentation text as HTML.
//
// Each span of unindented non-blank lines is converted into a single paragraph.
// There is one exception to the rule: a span that consists of a
// single line, is followed by another paragraph span, begins with a capital
// letter, and contains no punctuation is formatted as a heading.
//
// A span of indented lines is converted into a <pre> block, with the common
// indent prefix removed.
//
// URLs in the comment text are converted into links. Any word that matches
// an exported top-level identifier in the package is automatically converted
// into a hyperlink to the declaration of that identifier.
//
// This returns formatted HTML with:
//
//	<p>                elements for plain documentation text
//	<pre>              elements for preformatted text
//	<h3 id="hdr-XXX">  elements for headings with the "id" attribute
//	<a href="XXX">     elements for URL hyperlinks
//
// DocHTML is intended for documentation for the package and examples.
func (r *Renderer) DocHTML(doc string) safehtml.HTML {
	return r.declHTML(doc, nil, false).Doc
}

// DocHTMLExtractLinks is like DocHTML, but as a side-effect, the "Links"
// heading of doc is removed and its links are extracted.
func (r *Renderer) DocHTMLExtractLinks(doc string) safehtml.HTML {
	return r.declHTML(doc, nil, true).Doc
}

// Links returns the links extracted by DocHTMLExtractLinks.
func (r *Renderer) Links() []Link {
	return r.links
}

// DeclHTML formats the doc and decl and returns a tuple of
// strings corresponding to each input argument.
//
// This formats documentation HTML according to the same rules as DocHTML.
//
// This formats declaration HTML with:
//
//	<pre>                       element wrapping the entire declaration
//	<span id="X" data-kind="K"> elements for many top-level declarations
//	<span class="comment">      elements for every Go comment
//	<a href="XXX">              elements for URL hyperlinks
//
// DeclHTML is intended for top-level package declarations.
func (r *Renderer) DeclHTML(doc string, decl ast.Decl) (out struct{ Doc, Decl safehtml.HTML }) {
	// This returns an anonymous struct instead of multiple return values since
	// the template package only allows single return values.
	return r.declHTML(doc, decl, false)
}

// CodeHTML formats example code. If the code is a single block statement,
// the outer braces are stripped and the code unindented. If the example code
// contains an output comment, that will stripped as well.
//
// The code type must be *ast.File, *CommentedNode, []ast.Decl, []ast.Stmt
// or assignment-compatible to ast.Expr, ast.Decl, ast.Spec, or ast.Stmt.
//
// This returns formatted HTML with:
//
//	<pre>                   element wrapping entire block
//	<span class="comment">  elements for every Go comment
//
// CodeHTML is intended for use with example code snippets.
func (r *Renderer) CodeHTML(ex *doc.Example) safehtml.HTML {
	return r.codeHTML(ex)
}

// block is (*heading | *paragraph | *preformat).
type block interface{}

type (
	lines   []string
	heading struct {
		title string
	}
	paragraph struct {
		lines lines
	}
	preformat struct {
		lines lines
	}
)

// docToBlocks converts doc string into list of blocks.
//
// Heading block is a non-blank line, surrounded by blank lines
// and the next non-blank line is not indented.
//
// Preformat block contains single line or consecutive lines which have indent greater than 0.
//
// Paragraph block is a default block type if a block does not fall into heading and preformat.
func docToBlocks(doc string) []block {
	docLines := unindent(strings.Split(strings.Trim(doc, "\n"), "\n"))

	// Group the lines based on indentation and blank lines.
	var groups [][]string
	var lastGroup []string
	var wasInCode bool
	for _, line := range docLines {
		hasIndent := indentLength(line) > 0
		nowInCode := hasIndent || (wasInCode && line == "")
		newGroup := wasInCode != nowInCode || (!nowInCode && line == "")
		wasInCode = nowInCode
		if newGroup && len(lastGroup) > 0 {
			groups = append(groups, lastGroup)
			lastGroup = nil
		}
		if line != "" || nowInCode {
			lastGroup = append(lastGroup, line)
		}
	}
	if len(lastGroup) > 0 {
		groups = append(groups, lastGroup)
	}

	// Classify groups of lines as individual blocks.
	var blks []block
	var lastBlk block
	for i, group := range groups {
		willParagraph := i+1 < len(groups) && indentLength(groups[i+1][0]) == 0
		for len(group) > 0 && group[len(group)-1] == "" {
			group = group[:len(group)-1] // remove trailing empty lines
		}
		_, wasHeading := lastBlk.(*heading)
		switch {
		case indentLength(group[0]) > 0:
			blks = append(blks, &preformat{unindent(group)})
		case i != 0 && !wasHeading && len(group) == 1 && isHeading(group[0]) && willParagraph:
			blks = append(blks, &heading{group[0]})
		default:
			blks = append(blks, &paragraph{group})
		}
		lastBlk = blks[len(blks)-1]
	}
	return blks
}

func indentLength(s string) int {
	return len(s) - len(trimIndent(s))
}

func trimIndent(s string) string {
	return strings.TrimLeft(s, " \t")
}

func commonPrefixLength(a, b string) (n int) {
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func unindent(lines []string) []string {
	if len(lines) > 0 {
		npre := indentLength(lines[0])
		for _, line := range lines {
			if line != "" {
				npre = commonPrefixLength(lines[0][:npre], line)
			}
		}
		for i, line := range lines {
			if line != "" {
				lines[i] = line[npre:]
			}
		}
	}
	return lines
}

// isHeading returns bool of if it passes as a section heading or not.
// This is the copy of the heading function from the standard library.
// https://go.googlesource.com/go/+/refs/tags/go1.16/src/go/doc/comment.go#212
func isHeading(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return false
	}

	// a heading must start with an uppercase letter
	r, _ := utf8.DecodeRuneInString(line)
	if !unicode.IsLetter(r) || !unicode.IsUpper(r) {
		return false
	}

	// it must end in a letter or digit:
	r, _ = utf8.DecodeLastRuneInString(line)
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return false
	}

	// exclude lines with illegal characters. we allow "(),"
	if strings.ContainsAny(line, ";:!?+*/=[]{}_^°&§~%#@<\">\\") {
		return false
	}

	// allow "'" for possessive "'s" only
	for b := line; ; {
		i := strings.IndexRune(b, '\'')
		if i < 0 {
			break
		}
		if i+1 >= len(b) || b[i+1] != 's' || (i+2 < len(b) && b[i+2] != ' ') {
			return false // not followed by "s "
		}
		b = b[i+2:]
	}

	// allow "." when followed by non-space
	for b := line; ; {
		i := strings.IndexRune(b, '.')
		if i < 0 {
			break
		}
		if i+1 >= len(b) || b[i+1] == ' ' {
			return false // not followed by non-space
		}
		b = b[i+1:]
	}

	return true
}
