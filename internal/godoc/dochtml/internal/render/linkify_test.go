// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"context"
	"fmt"
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"github.com/google/safehtml/testconversions"
	"golang.org/x/tools/txtar"
)

func TestFormatDocHTML(t *testing.T) {
	files, err := filepath.Glob(filepath.FromSlash("testdata/formatDocHTML/*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no files")
	}
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".txt"), func(t *testing.T) {
			// See testdata/formatDocHTML/README.md for how these txtar files represent
			// test cases.
			ar, err := txtar.ParseFile(file)
			if err != nil {
				t.Fatal(err)
			}
			content := map[string][]byte{}
			for _, f := range ar.Files {
				content[f.Name] = f.Data
			}

			getContent := func(name string) string {
				return strings.TrimSpace(string(content[name]))
			}

			mustContent := func(t *testing.T, name string) string {
				if c := getContent(name); c != "" {
					return c
				}
				t.Fatalf("txtar file %s missing section %q", file, name)
				return ""
			}

			doc := string(mustContent(t, "doc"))
			wantNoExtract := mustContent(t, "want")
			var decl ast.Decl
			if d := getContent("decl"); d != "" {
				decl = parseDecl(t, d)
			}
			for _, extractLinks := range []bool{false, true} {
				t.Run(fmt.Sprintf("extractLinks=%t", extractLinks), func(t *testing.T) {
					r := New(context.Background(), nil, pkgTime, nil)
					got := r.formatDocHTML(doc, decl, extractLinks).String()
					want := wantNoExtract
					wantLinks := ""
					if extractLinks {
						// Use "want:links" if present.
						if w := getContent("want:links"); w != "" {
							want = w
						}
						wantLinks = getContent("links")
					}
					if diff := cmp.Diff(want, got); diff != "" {
						t.Errorf("doc mismatch (-want +got)\n%s", diff)
						t.Logf("want: %s", want)
						t.Logf("got: %s", got)
					}
					var b strings.Builder
					for _, l := range r.Links() {
						b.WriteString(l.Text + " " + l.Href + "\n")
					}
					if diff := cmp.Diff(wantLinks, strings.TrimSpace(b.String())); diff != "" {
						t.Errorf("links mismatch (-want +got)\n%s", diff)
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
		{
			"indented multi-line comment block statement",
			`{
	/*
		This is a multi
		line comment.
	*/
}`,
			`
<pre class="Documentation-exampleCode">
/*
	This is a multi
	line comment.
*/
</pre>
`,
		},
		{
			"An Output comment must appear at the start of the line.",
			`_ = true
// This comment containing "// Output:" is not treated specially.
`,
			`
<pre class="Documentation-exampleCode">
_ = true
// This comment containing &#34;// Output:&#34; is not treated specially.
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
      <li class="Documentation-tocItem"><a href="#hdr-The_Go_Project">The Go Project</a></li>
      <li class="Documentation-tocItem"><a href="#hdr-Heading_2">Heading 2</a></li>
  </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-The_Go_Project">The Go Project <a class="Documentation-idLink" href="#hdr-The_Go_Project" title="Go to The Go Project" aria-label="Go to The Go Project">¶</a></h4><p>Go is an open source project.
</p><h4 id="hdr-Heading_2">Heading 2 <a class="Documentation-idLink" href="#hdr-Heading_2" title="Go to Heading 2" aria-label="Go to Heading 2">¶</a></h4><p>More text.
</p>`)

	r := New(context.Background(), nil, pkgTime, nil)
	got := r.declHTML(doc, nil, false).Doc
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(safehtml.HTML{})); diff != "" {
		t.Errorf("r.declHTML() mismatch (-want +got)\n%s", diff)
	}
}

func TestHeadingIDSuffix(t *testing.T) {
	for _, test := range []struct {
		decl    string
		want    string
		wantIDs bool
	}{
		{"", "", true},
		{"func Foo(){}", "Foo", true},
		{"func (x T) Run(){}", "T_Run", true},
		{"func (x *T) Run(){}", "T_Run", true},
		{"func (x T[A]) Run(){}", "T_Run", true},
		{"func (x *T[A]) Run(){}", "T_Run", true},
		{"const C = 1", "C", true},
		{"var V int", "V", true},
		{"var V, W int", "", false},
		{"var x, y, V int", "", false},
		{"type T int", "T", true},
		{"type T_Run[X any] int", "T_Run", true},
		{"const (a = 1; b = 2; C = 3)", "", false},
		{"var (a = 1; b = 2; C = 3)", "", false},
		{"type (a int; b int; C int)", "", false},
	} {
		var decl ast.Decl
		if test.decl != "" {
			decl = parseDecl(t, test.decl)
		}
		got, gotIDs := headingIDSuffix(decl)
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.decl, got, test.want)
		}
		if gotIDs != test.wantIDs {
			t.Errorf("%q createIDs: got %t, want %t", test.decl, gotIDs, test.wantIDs)
		}
	}
}

func parseDecl(t *testing.T, decl string) ast.Decl {
	prog := "package p\n" + decl
	f, err := parser.ParseFile(token.NewFileSet(), "", prog, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f.Decls[0]
}

func TestAddHeading(t *testing.T) {
	// This test checks that the generated IDs are unique and the headings are saved.
	// It doesn't care about the HTML.
	var html safehtml.HTML

	check := func(hs *headingScope, ids ...string) {
		t.Helper()
		var want []heading
		for _, id := range ids {
			want = append(want, heading{safehtml.IdentifierFromConstantPrefix("hdr", id), html})
		}
		if !slices.Equal(hs.headings, want) {
			t.Errorf("\ngot  %v\nwant %v", hs.headings, want)
		}
	}

	addHeading := func(hs *headingScope, heading string) {
		hs.addHeading(&comment.Heading{
			Text: []comment.Text{comment.Plain(heading)},
		}, html)
	}

	hs := newHeadingScope("T", true)
	addHeading(hs, "heading")
	addHeading(hs, "heading 2")
	addHeading(hs, "heading")
	addHeading(hs, "heading")
	addHeading(hs, "heading.2")
	check(hs, "heading-T", "heading_2-T", "heading-T-1", "heading-T-2", "heading_2-T-1")

	// Check empty suffix.
	hs = newHeadingScope("", true)
	addHeading(hs, "h")
	addHeading(hs, "h")
	check(hs, "h", "h-1")

	// Check that invalid ID characters are removed from both suffix and input.
	hs = newHeadingScope("a.b𝜽", true)
	addHeading(hs, "h.i𝜽")
	check(hs, "h_i_-a_b_")

	// Check no link (empty ID).
	hs = newHeadingScope("", false)
	addHeading(hs, "h")
	want := []heading{{Title: html}}
	if !slices.Equal(hs.headings, want) {
		t.Errorf("got %v, want %v", hs.headings, want)
	}
}
