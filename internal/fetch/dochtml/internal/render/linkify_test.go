// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"github.com/google/safehtml/testconversions"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/fetch/internal/doc"
)

func TestDocHTML(t *testing.T) {
	for _, test := range []struct {
		name string
		doc  string
		want string
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
			want: `<h3 id="hdr-The_Go_Project">The Go Project<a href="#hdr-The_Go_Project">¶</a></h3>
  <p>Go is an open source project.
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
</p><p>TLSUnique contains the tls-unique channel binding value (see RFC
5929, section 3). The newline-separated RFC should be linked, but the words RFC and RFCs should not be.
</p>`,
		},
		{
			name: "quoted strings",
			doc:  `Bar returns the string "bar".`,
			want: `<p>Bar returns the string &#34;bar&#34;.
</p>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := New(context.Background(), nil, pkgTime, nil)
			got := r.declHTML(test.doc, nil).Doc
			want := testconversions.MakeHTMLForTest(test.want)
			if diff := cmp.Diff(want, got, cmp.AllowUnexported(safehtml.HTML{})); diff != "" {
				t.Errorf("r.declHTML() mismatch (-want +got)\n%s", diff)
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
			want: `<pre>
const (
<span id="Nanosecond" data-kind="constant"></span>	Nanosecond  <a href="#Duration">Duration</a> = 1
<span id="Microsecond" data-kind="constant"></span>	Microsecond          = 1000 * <a href="#Nanosecond">Nanosecond</a>
<span id="Millisecond" data-kind="constant"></span>	Millisecond          = 1000 * <a href="#Microsecond">Microsecond</a> <span class="comment">// comment</span>
<span id="Second" data-kind="constant"></span>	Second               = 1000 * <a href="#Millisecond">Millisecond</a> <span class="comment">/* multi
	line
	comment */</span>
<span id="Minute" data-kind="constant"></span>	Minute = 60 * <a href="#Second">Second</a>
<span id="Hour" data-kind="constant"></span>	Hour   = 60 * <a href="#Minute">Minute</a>
)</pre>
`,
		},
		{
			name:   "var",
			symbol: "UTC",
			want: `<pre>
<span id="UTC" data-kind="variable"></span>var UTC *<a href="#Location">Location</a> = &amp;utcLoc</pre>
`,
		},
		{
			name:   "type",
			symbol: "Ticker",
			want: `<pre>
type Ticker struct {
<span id="Ticker.C" data-kind="field"></span>	C &lt;-chan <a href="#Time">Time</a> <span class="comment">// The channel on which the ticks are delivered.</span>
	<span class="comment">// contains filtered or unexported fields</span>
}</pre>
`,
		},
		{
			name:   "func",
			symbol: "Sleep",
			want: `<pre>
func Sleep(d <a href="#Duration">Duration</a>)</pre>
`,
		},
		{
			name:   "method",
			symbol: "After",
			want: `<pre>
func After(d <a href="#Duration">Duration</a>) &lt;-chan <a href="#Time">Time</a></pre>
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			decl := declForName(t, pkgTime, test.symbol)
			r := New(context.Background(), fsetTime, pkgTime, nil)
			got := r.DeclHTML("", decl).Decl
			want := template.HTML(test.want)
			if diff := cmp.Diff(want, got); diff != "" {
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
<pre>
a := 1
<span class="comment">// a comment</span>
b := 2 <span class="comment">/* another comment */</span>
</pre>`,
		},
		{
			"trailing newlines",
			`a := 1


`,
			`
<pre>
a := 1
</pre>`,
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
<pre>
a := 1
<span class="comment">// Output:</span>
b := 1
</pre>
`,
		},
		{
			"stripped output comment and trailing newlines",
			`a := 1
// Output:
b := 1


// Output:
// removed
`,
			`
<pre>
a := 1
<span class="comment">// Output:</span>
b := 1
</pre>
`,
		},
	} {
		out := codeHTML(test.in)
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
	for _, tc := range []struct {
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
		t.Run(tc.name, func(t *testing.T) {
			ctx := experiment.NewContext(context.Background(), internal.ExperimentExecutableExamples)
			r := New(ctx, fset, pkgTime, nil)
			got, err := r.codeString(&tc.example)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):%s", diff)
			}
		})
	}
}
