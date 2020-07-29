// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package htmlcheck

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func Test(t *testing.T) {
	const data = `
		<html>
			<div id="ID" class="CLASS1 CLASS2">
			    BEFORE <a href="HREF">DURING</a> AFTER
				</div>
				<div class="WHITESPACE">lots

				of

				whitespace
				</div>
		</html>`

	doc, err := html.Parse(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		checker Checker
	}{
		{
			"In",
			In("div.CLASS1",
				HasText(`^\s*BEFORE DURING AFTER\s*$`),
				HasAttr("id", "ID"),
				HasAttr("class", `\bCLASS2\b`),
			),
		},
		{
			"a",
			In("a", HasAttr("href", "^HREF$"), HasText("DURING")),
		},
		{
			"NotIn",
			NotIn("#foo"),
		},
		{
			"Redundant whitespace",
			In("div.WHITESPACE", HasExactTextCollapsed("lots of whitespace")),
		},
	} {
		got := test.checker(doc)
		if got != nil {
			t.Errorf("%s: failed with %v", test.name, got)
		}
	}
}
