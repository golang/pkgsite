// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"context"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"github.com/google/safehtml/testconversions"
)

func TestFormatDocHTML(t *testing.T) {
	linksDoc := `Documentation.

The Go Project

Go is an open source project.


Links

- title1, url1

  -		title2 , url2


Header

More doc.
`

	for _, test := range []struct {
		name         string
		doc          string
		extractLinks []bool // nil means both
		want         string
		wantLinks    []Link
	}{
		{
			name: "short documentation is rendered",
			doc:  "The Go Project",
			want: "<p>The Go Project\n</p>",
		},
		{
			name: "regular documentation is rendered",
			doc: `The Go programming language is an open source project to make programmers more productive.

Go is expressive, concise, clean, and efficient. Its concurrency mechanisms make it easy to write programs that
get the most out of multicore and networked machines, while its novel type system enables flexible and modular
program construction.`,
			want: `<p>The Go programming language is an open source project to make programmers more productive.
</p><p>Go is expressive, concise, clean, and efficient. Its concurrency mechanisms make it easy to write programs that
get the most out of multicore and networked machines, while its novel type system enables flexible and modular
program construction.
</p>`,
		},
		{
			name: "header gets linked",
			doc: `The Go Project

Go is an open source project.`,
			want: `<p>The Go Project
</p><p>Go is an open source project.
</p>`,
		},
		{
			name: "header gets linked 2",
			doc: `Documentation.

The Go Project

Go is an open source project.`,
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-The_Go_Project">The Go Project</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-The_Go_Project">The Go Project <a class="Documentation-idLink" href="#hdr-The_Go_Project" aria-label="Go to The Go Project">¶</a></h4><p>Go is an open source project.
</p>`,
		},
		{
			name: "urls become links",
			doc: `Go is an open source project developed by a team at https://google.com and many
https://www.golang.org/CONTRIBUTORS from the open source community.

Go is distributed under a https://golang.org/LICENSE.`,
			want: `<p>Go is an open source project developed by a team at <a href="https://google.com">https://google.com</a> and many
<a href="https://www.golang.org/CONTRIBUTORS">https://www.golang.org/CONTRIBUTORS</a> from the open source community.
</p><p>Go is distributed under a <a href="https://golang.org/LICENSE">https://golang.org/LICENSE</a>.
</p>`,
		},
		{
			name: "RFCs get linked",
			doc: `Package tls partially implements TLS 1.2, as specified in RFC 5246, and TLS 1.3, as specified in RFC 8446.

In TLS 1.3, this type is called NamedGroup, but at this time this library only supports Elliptic Curve based groups. See RFC 8446, Section 4.2.7.

TLSUnique contains the tls-unique channel binding value (see RFC
5929, section 3). The newline-separated RFC should be linked, but the words RFC and RFCs should not be.
`,
			want: `<p>Package tls partially implements TLS 1.2, as specified in <a href="https://rfc-editor.org/rfc/rfc5246.html">RFC 5246</a>, and TLS 1.3, as specified in <a href="https://rfc-editor.org/rfc/rfc8446.html">RFC 8446</a>.
</p><p>In TLS 1.3, this type is called NamedGroup, but at this time this library only supports Elliptic Curve based groups. See <a href="https://rfc-editor.org/rfc/rfc8446.html#section-4.2.7">RFC 8446, Section 4.2.7</a>.
</p><p>TLSUnique contains the tls-unique channel binding value (see <a href="https://rfc-editor.org/rfc/rfc5929.html#section-3">RFC
5929, section 3</a>). The newline-separated RFC should be linked, but the words RFC and RFCs should not be.
</p>`,
		},
		{
			name: "quoted strings",
			doc:  `Bar returns the string "bar".`,
			want: `<p>Bar returns the string &#34;bar&#34;.
</p>`,
		},
		{
			name: "text is escaped",
			doc:  `link http://foo"><script>evil</script>`,
			want: `<p>link <a href="http://foo">http://foo</a>&#34;&gt;&lt;script&gt;evil&lt;/script&gt;
</p>`,
		},
		{
			name: "ulist",
			doc: `
			Here is a list:
				- a
				- b`,
			want: `<p>Here is a list:
</p><ul class="Documentation-bulletList">
  <li>a</li>
  <li>b</li>
</ul>`,
		},
		{
			name: "olist",
			doc: `
			Here is a list:
				1. a
				2. b`,
			want: `<p>Here is a list:
</p><ol class="Documentation-numberList">
  <li value="1">a</li>
  <li value="2">b</li>
</ol>`,
		},
		{
			name:         "Links section is not extracted",
			extractLinks: []bool{false},
			doc:          linksDoc,
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-The_Go_Project">The Go Project</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-Links">Links</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-Header">Header</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-The_Go_Project">The Go Project <a class="Documentation-idLink" href="#hdr-The_Go_Project" aria-label="Go to The Go Project">¶</a></h4><p>Go is an open source project.
</p><h4 id="hdr-Links">Links <a class="Documentation-idLink" href="#hdr-Links" aria-label="Go to Links">¶</a></h4><p>- title1, url1
</p><ul class="Documentation-bulletList">
  <li>title2 , url2</li>
</ul><h4 id="hdr-Header">Header <a class="Documentation-idLink" href="#hdr-Header" aria-label="Go to Header">¶</a></h4><p>More doc.
</p>`,
		},
		{
			name:         "Links section is extracted",
			extractLinks: []bool{true},
			doc:          linksDoc,
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-The_Go_Project">The Go Project</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-Header">Header</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-The_Go_Project">The Go Project <a class="Documentation-idLink" href="#hdr-The_Go_Project" aria-label="Go to The Go Project">¶</a></h4><p>Go is an open source project.
</p><h4 id="hdr-Header">Header <a class="Documentation-idLink" href="#hdr-Header" aria-label="Go to Header">¶</a></h4><p>More doc.
</p>`,
			wantLinks: []Link{
				{Text: "title1", Href: "url1"},
				{Text: "title2", Href: "url2"},
			},
		},
		{
			name: "escape back ticks in quotes",
			doc:  "For more detail, run ``go help test'' and ``go help testflag''",
			want: `<p>For more detail, run “go help test” and “go help testflag”` + "\n" + "</p>",
		},
		{
			name: "symbol links",
			doc:  "Links to [Month] and [Time.After].",
			want: `<p>Links to <a href="#Month">Month</a> and <a href="#Time.After">Time.After</a>.
</p>`,
		},
		{
			name: "package links",
			doc:  "Links to [time] and [github.com/a/b].",
			want: `<p>Links to <a href="">time</a> and <a href="/github.com/a/b">github.com/a/b</a>.
</p>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			extractLinks := test.extractLinks
			if extractLinks == nil {
				extractLinks = []bool{false, true}
			}
			for _, el := range extractLinks {
				t.Run(fmt.Sprintf("extractLinks=%t", el), func(t *testing.T) {
					r := New(context.Background(), nil, pkgTime, nil)
					got := r.formatDocHTML(test.doc, el)
					want := testconversions.MakeHTMLForTest(test.want)
					if diff := cmp.Diff(want, got, cmp.AllowUnexported(safehtml.HTML{})); diff != "" {
						t.Errorf("doc mismatch (-want +got)\n%s", diff)
					}
					if diff := cmp.Diff(test.wantLinks, r.Links()); diff != "" {
						t.Errorf("r.Links() mismatch (-want +got)\n%s", diff)
					}
				})
			}
		})
	}
}

func TestDeclHTML(t *testing.T) {
	for _, test := range []struct {
		name   string
		symbol string
		want   string
	}{
		{
			name:   "const",
			symbol: "Nanosecond",
			want: `const (
<span id="Nanosecond" data-kind="constant">	Nanosecond  <a href="#Duration">Duration</a> = 1
</span><span id="Microsecond" data-kind="constant">	Microsecond          = 1000 * <a href="#Nanosecond">Nanosecond</a>
</span><span id="Millisecond" data-kind="constant">	Millisecond          = 1000 * <a href="#Microsecond">Microsecond</a> <span class="comment">// comment</span>
</span><span id="Second" data-kind="constant">	Second               = 1000 * <a href="#Millisecond">Millisecond</a> <span class="comment">/* multi
	line
	comment */</span></span>
<span id="Minute" data-kind="constant">	Minute = 60 * <a href="#Second">Second</a>
</span><span id="Hour" data-kind="constant">	Hour   = 60 * <a href="#Minute">Minute</a>
</span>)`,
		},
		{
			name:   "var",
			symbol: "UTC",
			want:   `<span id="UTC" data-kind="variable">var UTC *<a href="#Location">Location</a> = &amp;utcLoc</span>`,
		},
		{
			name:   "type",
			symbol: "Ticker",
			want: `type Ticker struct {
<span id="Ticker.C" data-kind="field">	C &lt;-chan <a href="#Time">Time</a> <span class="comment">// The channel on which the ticks are delivered.</span>
</span>	<span class="comment">// contains filtered or unexported fields</span>
}`,
		},
		{
			name:   "func",
			symbol: "Sleep",
			want:   `func Sleep(d <a href="#Duration">Duration</a>)`,
		},
		{
			name:   "method",
			symbol: "After",
			want:   `func After(d <a href="#Duration">Duration</a>) &lt;-chan <a href="#Time">Time</a>`,
		},
		{
			name:   "interface",
			symbol: "Iface",
			want: `type Iface interface {
<span id="Iface.M" data-kind="method">	<span class="comment">// Method comment.</span>
</span>	M()
	<span class="comment">// contains filtered or unexported methods</span>
}`,
		},
		{
			name:   "long literal",
			symbol: "TooLongLiteral",
			want: `type TooLongLiteral struct {
<span id="TooLongLiteral.Name" data-kind="field">	<span class="comment">// The name.</span>
</span>	Name <a href="/builtin#string">string</a>

<span id="TooLongLiteral.Labels" data-kind="field">	<span class="comment">// The labels.</span>
</span>	Labels <a href="/builtin#int">int</a> ` + "``" + ` <span class="comment">/* 137-byte string literal not displayed */</span>
	<span class="comment">// contains filtered or unexported fields</span>
}`,
		},
		{
			name:   "filtered comment",
			symbol: "FieldTagFiltered",
			want: `type FieldTagFiltered struct {
<span id="FieldTagFiltered.Name" data-kind="field">	Name <a href="/builtin#string">string</a> ` + "`tag`" + `
</span>	<span class="comment">// contains filtered or unexported fields</span>
}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			decl := declForName(t, pkgTime, test.symbol)
			r := New(context.Background(), fsetTime, pkgTime, nil)
			got := r.DeclHTML("", decl).Decl.String()
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got)\n%s", diff)
			}
		})
	}
}

func declForName(t *testing.T, pkg *doc.Package, symbol string) ast.Decl {

	inVals := func(vals []*doc.Value) ast.Decl {
		for _, v := range vals {
			for _, n := range v.Names {
				if n == symbol {
					return v.Decl
				}
			}
		}
		return nil
	}

	if d := inVals(pkg.Consts); d != nil {
		return d
	}
	if d := inVals(pkg.Vars); d != nil {
		return d
	}
	for _, t := range pkg.Types {
		if t.Name == symbol {
			return t.Decl
		}
		if d := inVals(t.Consts); d != nil {
			return d
		}
		if d := inVals(t.Vars); d != nil {
			return d
		}
	}
	for _, f := range pkg.Funcs {
		if f.Name == symbol {
			return f.Decl
		}
	}
	t.Fatalf("no symbol %q in package %s", symbol, pkg.Name)
	return nil
}

func TestCodeHTML(t *testing.T) {
	for _, test := range []struct {
		name, in, want string
	}{
		{
			"basic",
			`a := 1
// a comment
b := 2 /* another comment */
`,
			`
<pre class="Documentation-exampleCode">
a := 1
// a comment
b := 2 /* another comment */
</pre>
`,
		},
		{
			"trailing newlines",
			`a := 1


`,
			`
<pre class="Documentation-exampleCode">
a := 1
</pre>
`,
		},
		{
			"stripped output comment",
			`a := 1
// Output:
b := 1
// Output:
// removed
`,
			`
<pre class="Documentation-exampleCode">
a := 1
// Output:
b := 1
</pre>
`,
		},
		{
			"stripped output comment and trailing code",
			`a := 1
// Output:
b := 1

// Output:
// removed
cleanup()
`,
			`
<pre class="Documentation-exampleCode">
a := 1
// Output:
b := 1
</pre>
`,
		},
	} {
		out := codeHTML(test.in, exampleTmpl)
		got := strings.TrimSpace(string(out.String()))
		want := strings.TrimSpace(test.want)
		if got != want {
			t.Errorf("%s:\ngot:\n%s\nwant:\n%s", test.name, got, want)
		}
	}
}

func mustParse(t *testing.T, fset *token.FileSet, filename, src string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestExampleCode(t *testing.T) {
	fset := token.NewFileSet()
	for _, test := range []struct {
		name    string
		example doc.Example
		want    string
	}{
		{
			name: "formats example code by fixing new lines and spacing",
			example: doc.Example{
				Code: mustParse(t, fset, "example_test.go", "package main"),
				Play: mustParse(t, fset, "example_hook_test.go", `package main
import    "fmt"
func main()    {
	fmt.Println("hello world")
	} `),
			},
			want: `package main

import "fmt"

func main() {
	fmt.Println("hello world")
}
`,
		},
		{
			name: "converts playable playground example to string",
			example: doc.Example{
				Code: mustParse(t, fset, "example_hook_test.go", "package main"),
				Play: mustParse(t, fset, "example_hook_test.go", `package main
import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Replacing a Logger's core can alter fundamental behaviors.
	// For example, it can convert a Logger to a no-op.
	nop := zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewNopCore()
	})

	logger := zap.NewExample()
	defer logger.Sync()

	logger.Info("working")
	logger.WithOptions(nop).Info("no-op")
	logger.Info("original logger still works")
}`),
			},
			want: `package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Replacing a Logger's core can alter fundamental behaviors.
	// For example, it can convert a Logger to a no-op.
	nop := zap.WrapCore(func(zapcore.Core) zapcore.Core {
		return zapcore.NewNopCore()
	})

	logger := zap.NewExample()
	defer logger.Sync()

	logger.Info("working")
	logger.WithOptions(nop).Info("no-op")
	logger.Info("original logger still works")
}
`,
		},
		{
			name: "converts non playable playground example to string",
			example: doc.Example{
				Code: mustParse(t, fset, "example_hook_test.go", `package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.Compare("a", "b"))
	fmt.Println(strings.Compare("a", "a"))
	fmt.Println(strings.Compare("b", "a"))
}
`),
				Play: nil,
			},
			want: `package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.Compare("a", "b"))
	fmt.Println(strings.Compare("a", "a"))
	fmt.Println(strings.Compare("b", "a"))
}
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			r := New(ctx, fset, pkgTime, nil)
			got, err := r.codeString(&test.example)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):%s", diff)
			}
		})
	}
}

func TestParseLink(t *testing.T) {
	for _, test := range []struct {
		line string
		want *Link
	}{
		{"", nil},
		{"foo", nil},
		{"- a b", nil},
		{"- a, b", &Link{Text: "a", Href: "b"}},
		{"- a, b, c", &Link{Text: "a", Href: "b, c"}},
		{"- a \t, https://b.com?x=1&y=2  ", &Link{Text: "a", Href: "https://b.com?x=1&y=2"}},
	} {
		got := parseLink(test.line)
		if !cmp.Equal(got, test.want) {
			t.Errorf("%q: got %+v, want %+v\n", test.line, got, test.want)
		}
	}
}

func TestCommentEscape(t *testing.T) {
	commentTests := []struct {
		in, out string
	}{
		{"typically invoked as ``go tool asm'',", `typically invoked as “go tool asm”,`},
		{"For more detail, run ``go help test'' and ``go help testflag''", `For more detail, run “go help test” and “go help testflag”`},
	}
	for i, test := range commentTests {
		out := convertQuotes(test.in)
		if out != test.out {
			t.Errorf("#%d: mismatch\nhave: %q\nwant: %q", i, out, test.out)
		}
	}
}

func TestTOC(t *testing.T) {
	doc := `
Documentation.

The Go Project

Go is an open source project.

Heading 2

More text.`

	want := testconversions.MakeHTMLForTest(`<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-The_Go_Project">The Go Project</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-Heading_2">Heading 2</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-The_Go_Project">The Go Project <a class="Documentation-idLink" href="#hdr-The_Go_Project" aria-label="Go to The Go Project">¶</a></h4><p>Go is an open source project.
</p><h4 id="hdr-Heading_2">Heading 2 <a class="Documentation-idLink" href="#hdr-Heading_2" aria-label="Go to Heading 2">¶</a></h4><p>More text.
</p>`)

	r := New(context.Background(), nil, pkgTime, nil)
	got := r.declHTML(doc, nil, false).Doc
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(safehtml.HTML{})); diff != "" {
		t.Errorf("r.declHTML() mismatch (-want +got)\n%s", diff)
	}
}
