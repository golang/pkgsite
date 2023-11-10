// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package htmlcheck

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestParse(t *testing.T) {
	var testCases = []struct {
		text         string
		wantSelector *selector
		wantErr      error
	}{
		{
			"a#id",
			&selector{
				atoms: []selectorAtom{&elementSelector{"a"}, &idSelector{"id"}},
			},
			nil,
		},
		{
			"a",
			&selector{atoms: []selectorAtom{&elementSelector{"a"}}},
			nil,
		},
		{
			"#id",
			&selector{atoms: []selectorAtom{&idSelector{"id"}}},
			nil,
		},
		{
			".class a",
			&selector{
				atoms: []selectorAtom{&classSelector{"class"}},
				next:  &selector{atoms: []selectorAtom{&elementSelector{"a"}}},
			},
			nil,
		},
		{
			`[attribute-name="value"] a`,
			&selector{
				atoms: []selectorAtom{&attributeSelector{attribute: "attribute-name", value: "value"}},
				next:  &selector{atoms: []selectorAtom{&elementSelector{"a"}}},
			},
			nil,
		},
		{
			`a[attribute-name="value"] a`,
			&selector{
				atoms: []selectorAtom{&elementSelector{"a"}, &attributeSelector{attribute: "attribute-name", value: "value"}},
				next:  &selector{atoms: []selectorAtom{&elementSelector{"a"}}},
			},
			nil,
		},
		{
			`.class1.class2`,
			&selector{
				atoms: []selectorAtom{&classSelector{"class1"}, &classSelector{"class2"}},
			},
			nil,
		},
		{
			`a.class1.class2`,
			&selector{
				atoms: []selectorAtom{&elementSelector{"a"}, &classSelector{"class1"}, &classSelector{"class2"}},
			},
			nil,
		},
		{
			".",
			nil,
			errors.New("no class name after '.'"),
		},
		{
			"#.",
			nil,
			errors.New("no id name after '#'"),
		},
		{
			"[]",
			nil,
			errors.New("expected attribute name after '[' in attribute selector"),
		},
		{
			"[attribute-name]",
			nil,
			errors.New("expected '=' after attribute name in attribute selector"),
		},
		{
			"[attribute-name=]",
			nil,
			errors.New("expected '\"' after = in attribute selector"),
		},
		{
			`[attribute-name=""]`,
			&selector{atoms: []selectorAtom{&attributeSelector{attribute: "attribute-name", value: ""}}},
			nil,
		},
		{
			`[attribute-name="]`,
			nil,
			errors.New("expected '\"' after attribute value"),
		},
		{
			`[attribute-name="value`,
			nil,
			errors.New("expected '\"' after attribute value"),
		},
		{
			`[attribute-name="value"`,
			nil,
			errors.New("expected ']' at end of attribute selector"),
		},
		{
			`.VulnMain-title:nth-of-type(4)`,
			&selector{atoms: []selectorAtom{&classSelector{"VulnMain-title"}, &nthOfType{4}}},
			nil,
		},
		{
			`th:nth-child(2)`,
			&selector{atoms: []selectorAtom{&elementSelector{"th"}, &nthChild{2}}},
			nil,
		},
		{
			`th:(2)`,
			nil,
			errors.New("expected identifier after : in pseudo class"),
		},
		{
			`th:32(2)`,
			nil,
			errors.New("expected identifier after : in pseudo class"),
		},
		{
			`th:active`,
			nil,
			errors.New("only :nth-of-type() and :nth-child() pseudoclasses are supported"),
		},
		{
			`th:nth-child`,
			nil,
			errors.New("expected '(' after :nth-of-type or nth-child"),
		},
		{
			`th:nth-child(odd)`,
			nil,
			errors.New("only number arguments are supported for :nth-of-type() or :nth-child()"),
		},
		{
			`th:nth-child(14`,
			nil,
			errors.New("expected ')' after number argument to nth-of-type or nth-child"),
		},
		// We don't support the child combinator. Make sure it returns a parse error.
		{
			".Documentation-sinceVersion > .Documentation-sinceVersionVersion",
			nil,
			errors.New("unexpected character '>' in input"),
		},
	}
	for _, tc := range testCases {
		sel, err := parse(tc.text)
		if tc.wantErr != nil {
			if err == nil {
				t.Fatalf("parse(%q): got nil err, want err %q", tc.text, tc.wantErr)
			}
			if tc.wantErr.Error() != err.Error() {
				t.Fatalf("parse(%q): got err %q, want err %q", tc.text, err, tc.wantErr)
			}
		} else if err != nil {
			t.Fatalf("parse(%q): got err %q, want nil error", tc.text, err)
		}
		if !reflect.DeepEqual(sel, tc.wantSelector) {
			t.Fatalf("parse(%q): got %v; want %v", tc.text, sel, tc.wantSelector)
		}
	}
}

func TestQuery(t *testing.T) {
	var testCases = []struct {
		queriedText string
		selector    string
		want        string
	}{
		{"<a></a>", "a", "<a></a>"},
		{`<a></a><a id="id">text</a>`, "a#id", `<a id="id">text</a>`},
		{`<a></a><a class="class1"></a><a class="class2"></a><a class="class1 class2">text</a>`, ".class1.class2", `<a class="class1 class2">text</a>`},
		{`<a></a><a class="class1">first</a><a class="class2"></a><a class="class1 class2">text</a>`, ".class1", `<a class="class1">first</a>`},
		{`<div><div></div><div my-attr="my-val">text</div></div>`, `[my-attr="my-val"]`, `<div my-attr="my-val">text</div>`},
		{`<div><div></div><div my-attr="my-val">text</div></div>`, `[myattr="my-val"]`, ""},
		{`<html><head></head><body><div><div></div><div my-attr="my-val">text</div></div></body></html>`, ``, `<html><head></head><body><div><div></div><div my-attr="my-val">text</div></div></body></html>`},
		{`<div></div><div><div>match me</div></div>`, "div div", `<div>match me</div>`},
		{`<div></div><div><div>wrong</div></div><div id="wrong-id"><div class="my-class">also wrong</div></div><div id="my-id"><div class="wrong-class">still wrong</div></div><div id="my-id"><div class="my-class">match</div></div>`, "div#my-id div.my-class", `<div class="my-class">match</div>`},
		{`<a></a><div class="UnitMeta-repo"><a href="foo" title="">link body</a></div>`, ".UnitMeta-repo a", `<a href="foo" title="">link body</a>`},
		{`<ul class="UnitFiles-fileList"><li><a href="foo">a.go</a></li></ul>`, ".UnitFiles-fileList a", `<a href="foo">a.go</a>`},
		{`<ul><li>first child</li><li>second child</li></ul>`, "li:nth-child(2)", `<li>second child</li>`},
		{`<ul> <li>first child</li> <li>second child</li> </ul>`, "li:nth-child(2)", `<li>second child</li>`},
		{`<div><div>not counted</div><p class="class">first paragraph</p></div>`, ".class:nth-of-type(1)", `<p class="class">first paragraph</p>`},
		{`<div><div>not counted</div> <p class="class">first paragraph</p> </div>`, ".class:nth-of-type(1)", `<p class="class">first paragraph</p>`},
		{`<div><div class="class">not counted</div><p class="class">first paragraph</p>`, ".class:nth-of-type(2)", ``},
		{`<div><div class="class">not counted</div><p class="class">first paragraph</p><p class="class">second paragraph</p></div>`, ".class:nth-of-type(2)", `<p class="class">second paragraph</p>`},
		{`<div><div>not counted</div><p>first paragraph</p><p class="class">second paragraph</p></div>`, ".class:nth-of-type(2)", `<p class="class">second paragraph</p>`},
		{`<div><div>not counted</div><p>first paragraph</p><div>also not counted</div><p class="class">second paragraph</p></div>`, ".class:nth-of-type(2)", `<p class="class">second paragraph</p>`},
		{`<div><div>not counted</div><p>first paragraph</p><div>also not counted</div><p class="class">second paragraph</p><td>also not counted</td><p>third paragraph</p><p>fourth paragraph</p><p class="class">fifth paragraph</p></div>`, ".class:nth-of-type(5)", `<p class="class">fifth paragraph</p>`},
		{`<table class="UnitDirectories-table"><tbody><tr class="UnitDirectories-tableHeader"> <th>Path</th> <th class="UnitDirectories-desktopSynopsis">Synopsis</th></tr>`, "th:nth-child(1)", "<th>Path</th>"},
		{`<table class="UnitDirectories-table"> <tbody> <tr class="UnitDirectories-tableHeader"> <th>Path</th> <th class="UnitDirectories-desktopSynopsis"> Synopsis </th> </tr>`, "th:nth-child(1)", "<th>Path</th>"},
	}
	for _, tc := range testCases {
		n, err := html.Parse(strings.NewReader(tc.queriedText))
		if err != nil {
			t.Fatalf("parsing queried text %q: %v", tc.queriedText, err)
		}
		sel, err := parse(tc.selector)
		if err != nil {
			t.Fatalf("parsing selector %q: %v", tc.selector, err)
		}
		got := query(n, sel)
		if got == nil {
			if tc.want == "" {
				continue
			}
			t.Fatalf("query(%q, %q): got nil; want %q", tc.queriedText, tc.selector, tc.want)
		}
		var buf bytes.Buffer
		err = html.Render(&buf, got)
		if err != nil {
			t.Fatalf("rendering result of query: %v", err)
		}
		if buf.String() != tc.want {
			t.Fatalf("query(%q, %q): got %q; want %q", tc.queriedText, tc.selector, buf.String(), tc.want)
		}
	}
}
