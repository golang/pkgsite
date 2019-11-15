// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package htmlcheck provides a set of functions that check for properties
// of a parsed HTML document.
package htmlcheck

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// A Checker is a function from an HTML node to an error describing a failure.
type Checker func(*html.Node) error

// In returns a Checker that applies the given checkers to the first node
// matching the CSS selector. The empty selector denotes the entire subtree of
// the Checker's argument node.
//
// Calling In(selector), with no checkers, just checks for the presence of
// a node matching the selector. (For the negation, see NotIn.)
func In(selector string, checkers ...Checker) Checker {
	sel := mustParseSelector(selector)
	return func(n *html.Node) error {
		var m *html.Node
		// cascadia.Query does not test against its argument node.
		if sel.Match(n) {
			m = n
		} else {
			m = cascadia.Query(n, sel)
		}
		if m == nil {
			return fmt.Errorf("no element matches selector %q", selector)
		}
		if err := check(m, checkers); err != nil {
			if selector == "" {
				return err
			}
			return fmt.Errorf("%s: %v", selector, err)
		}
		return nil
	}
}

// InAt is like In, but instead of the first node satifying the selector, it
// applies the Checkers to the i'th node.
// InAt(s, 0, m) is equivalent to (but slower than) In(s, m).
func InAt(selector string, i int, checkers ...Checker) Checker {
	sel := mustParseSelector(selector)
	if i < 0 {
		panic("negative index")
	}
	return func(n *html.Node) error {
		var els []*html.Node
		if sel.Match(n) {
			els = append(els, n)
		}
		els = append(els, cascadia.QueryAll(n, sel)...)
		if i >= len(els) {
			return fmt.Errorf("%q: index %d is out of range for %d elements", selector, i, len(els))
		}
		if err := check(els[i], checkers); err != nil {
			return fmt.Errorf("%s[%d]: %s", selector, i, err)
		}
		return nil
	}
}

// NotIn returns a checker that succeeds only if no nodes match selector.
func NotIn(selector string) Checker {
	sel := mustParseSelector(selector)
	return func(n *html.Node) error {
		if sel.Match(n) || cascadia.Query(n, sel) != nil {
			return fmt.Errorf("%q matched one or more elements", selector)
		}
		return nil
	}
}

// check calls all the Checkers on n, returning the error of the first one to fail.
func check(n *html.Node, Checkers []Checker) error {
	for _, m := range Checkers {
		if err := m(n); err != nil {
			return err
		}
	}
	return nil
}

// mustParseSelector parses the given CSS selector. An empty string
// is treated as "*" (match everything).
func mustParseSelector(s string) cascadia.Sel {
	if s == "" {
		s = "*"
	}
	sel, err := cascadia.Parse(s)
	if err != nil {
		panic(fmt.Sprintf("parsing %q: %v", s, err))
	}
	return sel
}

// HasText returns a Checker that checks whether the given regexp matches the node's text.
// The text of a node n is the concatenated contents of all text nodes in n's subtree.
// HasText panics if the argument doesn't compile.
func HasText(wantRegexp string) Checker {
	re := regexp.MustCompile(wantRegexp)
	return func(n *html.Node) error {
		var b strings.Builder
		nodeText(n, &b)
		text := b.String()
		if !re.MatchString(text) {
			if len(text) > 100 {
				text = text[:97] + "..."
			}
			return fmt.Errorf("regexp `%s` does not match %q", wantRegexp, text)
		}
		return nil
	}
}

// nodeText appends the text of n's subtree to b. This is the concatenated
// contents of all text nodes, visited depth-first.
func nodeText(n *html.Node, b *strings.Builder) {
	if n == nil {
		return
	}
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
	case html.ElementNode, html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			nodeText(c, b)
		}
	}
}

// HasAttr returns a Checker that checks for an attribute with the given name whose
// value matches the given regular expression.
// HasAttr panics if wantValRegexp does not compile.
func HasAttr(name, wantValRegexp string) Checker {
	re := regexp.MustCompile(wantValRegexp)
	return func(n *html.Node) error {
		for _, a := range n.Attr {
			if a.Key == name {
				if !re.MatchString(a.Val) {
					return fmt.Errorf("[%q]: regexp `%s` does not match %q", name, wantValRegexp, a.Val)
				}
				return nil
			}
		}
		return fmt.Errorf("[%q]: no such attribute", name)
	}
}
