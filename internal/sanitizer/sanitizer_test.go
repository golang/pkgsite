// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sanitizer

import (
	"reflect"
	"testing"

	"golang.org/x/net/html"
)

func TestSanitizeBytes(t *testing.T) {
	var testCases = []struct {
		input, want string
	}{
		{
			"<script>body</script>",
			"",
		},
		{
			"<script><tag>body</tag></script>",
			"",
		},
		{
			`<a href="%">body</a>`,
			`body`,
		},
		{
			`<a href="unrecognized://foo">body</a>`,
			`body`,
		},
		{
			`<p dir="RTL" lang="en" id="foo" title="a title"></p>`,
			`<p dir="RTL" lang="en" id="foo" title="a title"></p>`,
		},
		{
			`<p dir="ABC" lang="e" id=" foo" title="a title%"></p>`,
			`<p></p>`,
		},
		{
			`<a href="https://golang.org">body</a>`,
			`<a href="https://golang.org" rel="nofollow">body</a>`,
		},
		{
			`<script></script><a href="https://golang.org">body</a>`,
			`<a href="https://golang.org" rel="nofollow">body</a>`,
		},
		{
			`
<img src="file.jpeg" alt="alt text" usemap="#map" width="600", height="400">

<map name="map">
<area shape="rect" coords="1,2,3,4" alt="alt text" href="page.html">
</map>
`,
			`
<img src="file.jpeg" alt="alt text" usemap="#map" width="600" height="400"/>




`,
		},
		{
			`<area href="link" rel="value"/>`,
			``,
		},
		{
			`<p href="notonp">body</p>`,
			`<p>body</p>`,
		},
		{
			`<blockquote cite="valid_url">body</blockquote>`,
			`<blockquote cite="valid_url">body</blockquote>`,
		},
		{
			`<blockquote cite="invalid://url">body</blockquote>`,
			`<blockquote>body</blockquote>`,
		},
		{
			`<!DOCTYPE html>text<!-- comment --><p>`,
			`text<p></p>`,
		},
		{
			`<article badattr="bad"> <ol> <bad> <li>hello</li> </script> <li>thi<wbr/>ng</li> </ol> <ul> </ul> </article>`,
			`<article> <ol>  <li>hello</li>  <li>thi<wbr/>ng</li> </ol> <ul> </ul> </article>`,
		},
		{
			`<details open="closed"></details>`,
			`<details></details>`,
		},
		{
			`<details open="open"></details>`,
			`<details open="open"></details>`,
		},
		{
			`<details open=""></details>`,
			`<details open=""></details>`,
		},
		{
			`<div align="center">`,
			`<div align="center"></div>`,
		},
		{
			`<p><bad>A</bad><bad>B</bad></p>`,
			`<p>AB</p>`,
		},
		{
			`
<table height="40" width="30" summary="foo">
	<caption>a caption</caption>
	<colgroup align="left" valign="BOTTOM" span="4" height="30" width="20%">
		<col align="justify" span="32" valign="baseline" height="40%" width="40"/>
	</colgroup>
	<thead align="LeFt" valign="MiDdLe">
		<tr align="rIgHt" valign="tOp">
			<th abbr="hello world" align="right" colspan="4" rowspan="3" headers="foo bar" height="30%" width="4" scope="rowgroup" valign="top" nowrap="">th</th>
			<td abbr="goodbye world" align="left" colspan="3" rowspan="2" headers="baz quux" height="20" width="50%" scope="col" valign="baseline" nowrap="nowrap">td</td>
		</tr>
	</thead>
	<tbody align="justify" valign="bottom">
	</tbody>
	<tfoot align="center" valign="middle">
	</tfoot>
</table>
`, `
<table height="40" width="30" summary="foo">
	<caption>a caption</caption>
	<colgroup align="left" valign="BOTTOM" span="4" height="30" width="20%">
		<col align="justify" span="32" valign="baseline" height="40%" width="40"/>
	</colgroup>
	<thead align="LeFt" valign="MiDdLe">
		<tr align="rIgHt" valign="tOp">
			<th abbr="hello world" align="right" colspan="4" rowspan="3" headers="foo bar" height="30%" width="4" scope="rowgroup" valign="top" nowrap="">th</th>
			<td abbr="goodbye world" align="left" colspan="3" rowspan="2" headers="baz quux" height="20" width="50%" scope="col" valign="baseline" nowrap="nowrap">td</td>
		</tr>
	</thead>
	<tbody align="justify" valign="bottom">
	</tbody>
	<tfoot align="center" valign="middle">
	</tfoot>
</table>
`,
		},
		{`<p><bad><bad2><bad3><bad4>hello<bad5><bad6><p> middle</p>goodbye`,
			`<p>hello</p><p> middle</p>goodbye`},
	}

	for _, tc := range testCases {
		got := string(SanitizeBytes([]byte(tc.input)))
		if got != tc.want {
			t.Errorf("SanitizeBytes(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAddRelNoFollow(t *testing.T) {
	testCases := []struct {
		input []html.Attribute
		want  []html.Attribute
	}{
		{
			[]html.Attribute{},
			[]html.Attribute{},
		},
		{
			[]html.Attribute{{Key: "id", Val: "foo"}},
			[]html.Attribute{{Key: "id", Val: "foo"}},
		},
		{
			[]html.Attribute{{Key: "href", Val: "https://golang.org"}},
			[]html.Attribute{{Key: "href", Val: "https://golang.org"}, {Key: "rel", Val: "nofollow"}},
		},
		{
			[]html.Attribute{{Key: "href", Val: "https://golang.org"}, {Key: "rel", Val: "nofollow"}},
			[]html.Attribute{{Key: "href", Val: "https://golang.org"}, {Key: "rel", Val: "nofollow"}},
		},
		{
			[]html.Attribute{{Key: "href", Val: "https://golang.org"}, {Key: "rel", Val: "canonical"}},
			[]html.Attribute{{Key: "href", Val: "https://golang.org"}, {Key: "rel", Val: "nofollow"}},
		},
		{
			[]html.Attribute{{Key: "id", Val: "foo"}, {Key: "rel", Val: "canonical"}},
			[]html.Attribute{{Key: "id", Val: "foo"}, {Key: "rel", Val: "canonical"}},
		},
	}

	for _, tc := range testCases {
		got := addRelNoFollow(append([]html.Attribute{}, tc.input...))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("addRelNoFollow(%v): got %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestValidURL(t *testing.T) {
	testCases := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"#", true},
		{"https://golang.org", true},
		{"http://golang.org", true},
		{"mailto:golang.org", true},
		{"unsupported:golang.org", false},
		{" https://golang.org ", true},
		{"\thttps://golang.org ", true},
		{" https://golang.org/my file", false},
		{" https://golang.org/my\tfile", false},
		{" https://golang.org/my\nfile", false},
		{"%", false},
		{" % ", false},
		{" %\t% ", false},
	}

	for _, tc := range testCases {
		got := validURL(tc.input)
		if got != tc.want {
			t.Errorf("validURL(%q): got %v, want %v", tc.input, got, tc.want)
		}
	}
}
