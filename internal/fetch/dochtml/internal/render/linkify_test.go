// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"github.com/google/safehtml/testconversions"
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
			r := New(nil, pkgTime, nil)
			got := r.declHTML(test.doc, nil).Doc
			want := testconversions.MakeHTMLForTest(test.want)
			if diff := cmp.Diff(want, got, cmp.AllowUnexported(safehtml.HTML{})); diff != "" {
				t.Errorf("r.declHTML() mismatch (-want +got)\n%s", diff)
			}
		})
	}
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
