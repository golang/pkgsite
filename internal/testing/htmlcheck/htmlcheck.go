// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package htmlcheck provides a set of functions that check for properties
// of a parsed HTML document.
package htmlcheck

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// A Checker is a function from an HTML node to an error describing a failure.
type Checker func(*html.Node) error

// Run is a convenience function to run the checker against HTML read from
// reader.
func Run(reader io.Reader, checker Checker) error {
	node, err := html.Parse(reader)
	if err != nil {
		return err
	}
	return checker(node)
}

// In returns a Checker that applies the given checkers to the first node
// matching the CSS selector. The empty selector denotes the entire subtree of
// the Checker's argument node.
//
// Calling In(selector), with no checkers, just checks for the presence of
// a node matching the selector. (For the negation, see NotIn.)
//
// A nil Checker is valid and always succeeds.
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
		els := allMatching(n, sel)
		if i >= len(els) {
			return fmt.Errorf("%q: index %d is out of range for %d elements", selector, i, len(els))
		}
		if err := check(els[i], checkers); err != nil {
			return fmt.Errorf("%s[%d]: %v", selector, i, err)
		}
		return nil
	}
}

// InAll runs the checkers against all nodes matching selector.
func InAll(selector string, checkers ...Checker) Checker {
	sel := mustParseSelector(selector)
	return func(n *html.Node) error {
		els := allMatching(n, sel)
		for i, el := range els {
			if err := check(el, checkers); err != nil {
				return fmt.Errorf("%s, #%d: %v", selector, i, err)
			}
		}
		return nil
	}
}

func allMatching(n *html.Node, sel cascadia.Sel) []*html.Node {
	var els []*html.Node
	if sel.Match(n) {
		els = append(els, n)
	}
	return append(els, cascadia.QueryAll(n, sel)...)
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
		if m == nil {
			continue
		}
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
			return fmt.Errorf("\n`%s` does not match\n%q", wantRegexp, text)
		}
		return nil
	}
}

// HasExactText returns a checker that checks whether the given string matches
// the node's text exactly.
func HasExactText(want string) Checker {
	return HasText("^" + regexp.QuoteMeta(want) + "$")
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
					return fmt.Errorf("[%q]:\n`%s` does not match\n%q", name, wantValRegexp, a.Val)
				}
				return nil
			}
		}
		return fmt.Errorf("[%q]: no such attribute", name)
	}
}

// HasHref returns a Checker that checks whether the node has an "href"
// attribute with exactly val.
func HasHref(val string) Checker {
	return HasAttr("href", "^"+regexp.QuoteMeta(val)+"$")
}

// Dump returns a Checker that always returns nil, and as a side-effect writes a
// human-readable description of n's subtree to standard output. It is useful
// for debugging.
func Dump() Checker {
	return func(n *html.Node) error {
		dump(n, 0)
		return nil
	}
}

func dump(n *html.Node, depth int) {
	for i := 0; i < depth; i++ {
		fmt.Print("  ")
	}
	fmt.Printf("type %d, data %q, attr %v\n", n.Type, n.Data, n.Attr)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		dump(c, depth+1)
	}
}
