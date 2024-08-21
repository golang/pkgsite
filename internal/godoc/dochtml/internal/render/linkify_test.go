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
	"path/filepath"
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
			for _, extractLinks := range []bool{false, true} {
				t.Run(fmt.Sprintf("extractLinks=%t", extractLinks), func(t *testing.T) {
					r := New(context.Background(), nil, pkgTime, nil)
					got := r.formatDocHTML(doc, nil, extractLinks).String()
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

func TestFormatDocHTMLDecl(t *testing.T) {
	duplicateHeadersDoc := `Documentation.

Information

This is some information.

Information

This is some other information.
`
	// typeWithFieldsDecl is declared as:
	// 	type I2 interface {
	// 		I1
	// 		M2()
	// 	}
	typeWithFieldsDecl := &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: ast.NewIdent("I2"),
				Type: &ast.InterfaceType{
					Methods: &ast.FieldList{
						List: []*ast.Field{
							{Type: ast.NewIdent("I1")},
							{Type: &ast.FuncType{}, Names: []*ast.Ident{ast.NewIdent("M2")}},
						},
					},
				},
			},
		},
	}

	for _, test := range []struct {
		name         string
		doc          string
		decl         ast.Decl
		extractLinks []bool // nil means both
		want         string
		wantLinks    []Link
	}{
		{
			name: "unique header ids in constants section for grouped constants",
			doc:  duplicateHeadersDoc,
			decl: &ast.GenDecl{
				Tok:   token.CONST,
				Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{{}}}, &ast.ValueSpec{Names: []*ast.Ident{{}}}},
			},
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-constant_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-constant_Information">Information <a class="Documentation-idLink" href="#hdr-constant_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
</p>`,
		},
		{
			name: "unique header ids in variables section for grouped variables",
			doc:  duplicateHeadersDoc,
			decl: &ast.GenDecl{
				Tok:   token.VAR,
				Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{{}}}, &ast.ValueSpec{Names: []*ast.Ident{{}}}},
			},
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-variable_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-variable_Information">Information <a class="Documentation-idLink" href="#hdr-variable_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
</p>`,
		},
		{
			name: "unique header ids in functions section",
			doc:  duplicateHeadersDoc,
			decl: &ast.FuncDecl{Name: ast.NewIdent("FooFunc")},
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-FooFunc_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-FooFunc_Information">Information <a class="Documentation-idLink" href="#hdr-FooFunc_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
</p>`,
		},
		{
			name: "unique header ids in functions section for method",
			doc:  duplicateHeadersDoc,
			decl: &ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("Bar")}}},
				Name: ast.NewIdent("Func"),
			},
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-Bar_Func_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-Bar_Func_Information">Information <a class="Documentation-idLink" href="#hdr-Bar_Func_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
</p>`,
		},
		{
			name: "unique header ids in types section",
			doc:  duplicateHeadersDoc,
			decl: &ast.GenDecl{
				Tok:   token.TYPE,
				Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent("Duration")}},
			},
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-Duration_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-Duration_Information">Information <a class="Documentation-idLink" href="#hdr-Duration_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
</p>`,
		},
		{
			name: "unique header ids in types section for types with fields",
			doc:  duplicateHeadersDoc,
			decl: typeWithFieldsDecl,
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-I2_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-I2_Information">Information <a class="Documentation-idLink" href="#hdr-I2_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
</p>`,
		},
		{
			name: "unique header ids in types section for typed variable",
			doc:  duplicateHeadersDoc,
			decl: &ast.GenDecl{
				Tok:   token.VAR,
				Specs: []ast.Spec{&ast.ValueSpec{Type: &ast.StarExpr{X: ast.NewIdent("Location")}, Names: []*ast.Ident{ast.NewIdent("UTC")}}},
			},
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-UTC_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-UTC_Information">Information <a class="Documentation-idLink" href="#hdr-UTC_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
</p>`,
		},
		{
			name: "unique header ids in types section for typed constant",
			doc:  duplicateHeadersDoc,
			decl: &ast.GenDecl{
				Tok:   token.CONST,
				Specs: []ast.Spec{&ast.ValueSpec{Type: ast.NewIdent("T"), Names: []*ast.Ident{ast.NewIdent("C")}}},
			},
			want: `<div role="navigation" aria-label="Table of Contents">
  <ul class="Documentation-toc">
    <li class="Documentation-tocItem">
        <a href="#hdr-Information">Information</a>
      </li>
    <li class="Documentation-tocItem">
        <a href="#hdr-C_Information">Information</a>
      </li>
    </ul>
</div>
<p>Documentation.
</p><h4 id="hdr-Information">Information <a class="Documentation-idLink" href="#hdr-Information" aria-label="Go to Information">¶</a></h4><p>This is some information.
</p><h4 id="hdr-C_Information">Information <a class="Documentation-idLink" href="#hdr-C_Information" aria-label="Go to Information">¶</a></h4><p>This is some other information.
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
					got := r.formatDocHTML(test.doc, test.decl, el)
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
