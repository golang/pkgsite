// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package htmlcheck

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// A selector represents a parsed css selector that can be used in a query.
// The atoms all match against a given element and next matches against
// children of that element. So, for example "div#id a" parses into a selector that
// has atoms for matching the div and the id and a next that points to another
// selector that has an atom for "a".
type selector struct {
	atoms []selectorAtom
	next  *selector
}

// String returns a string used for debugging test failures.
func (s *selector) String() string {
	if s == nil {
		return "nil"
	}
	str := "["
	for i, atom := range s.atoms {
		str += fmt.Sprintf("%#v", atom)
		if i != len(s.atoms)-1 {
			str += ","
		}
	}
	str += "]->" + s.next.String()
	return str
}

// selectorAtom represents a part of a selector that individually
// matches a single element name, id, class, or attribute value.
type selectorAtom interface {
	match(n *html.Node) bool
}

// query returns the first node in n that matches the given selector,
// or nil if there are no nodes matching the selector.
func query(n *html.Node, selector *selector) *html.Node {
	allMatch := true
	for _, atom := range selector.atoms {
		if !atom.match(n) {
			allMatch = false
			break
		}
	}
	if allMatch {
		if selector.next != nil {
			if result := queryChildren(n, selector.next); result != nil {
				return result
			}
		} else {
			return n
		}
	}
	return queryChildren(n, selector)
}

func queryChildren(n *html.Node, selector *selector) *html.Node {
	child := n.FirstChild
	for child != nil {
		if result := query(child, selector); result != nil {
			return result
		}
		child = child.NextSibling
	}
	return nil
}

// parse parses the string into a selector. It matches the following
// atoms: element, #id, .class, [attribute="value"]. It allows the atoms
// to be combined where they all need to match (for example, a#id) and
// for nested selectors to be combined with a space.
// For simplicity, the selector must not have any non-ASCII bytes.
func parse(s string) (*selector, error) {
	sel := &selector{}
	if !isAscii(s) {
		return nil, errors.New("non ascii byte found in selector string")
	}
	for len(s) > 0 {
		switch {
		case isLetter(s[0]):
			ident, rest := consumeIdentifier(s)
			sel.atoms = append(sel.atoms, &elementSelector{ident})
			s = rest
		case s[0] == '.':
			ident, rest := consumeIdentifier(s[1:])
			if len(ident) == 0 {
				return nil, errors.New("no class name after '.'")
			}
			sel.atoms = append(sel.atoms, &classSelector{ident})
			s = rest
		case s[0] == '#':
			ident, rest := consumeIdentifier(s[1:])
			if len(ident) == 0 {
				return nil, errors.New("no id name after '#'")
			}
			sel.atoms = append(sel.atoms, &idSelector{ident})
			s = rest
		case s[0] == '[':
			attributeSelector, rest, err := parseAttributeSelector(s)
			if err != nil {
				return nil, err
			}
			sel.atoms = append(sel.atoms, attributeSelector)
			s = rest
		case s[0] == ' ':
			s = strings.TrimLeft(s, " ")
			next, err := parse(s)
			if err != nil {
				return nil, err
			}
			sel.next = next
			return sel, nil
		default:
			return nil, fmt.Errorf("unexpected character %q in input", s[0])
		}
	}
	return sel, nil
}

// parseAttributeSelector parses an attribute selector of the form [attribute-name="attribute=value"]
func parseAttributeSelector(s string) (*attributeSelector, string, error) {
	if s[0] != '[' {
		return nil, "", errors.New("expected '[' at beginning of attribute selector")
	}
	ident, rest := consumeIdentifier(s[1:])
	if len(ident) == 0 {
		return nil, "", errors.New("expected attribute name after '[' in attribute selector")
	}
	attributeName := ident
	s = rest
	if len(s) == 0 || s[0] != '=' {
		return nil, "", errors.New("expected '=' after attribute name in attribute selector")
	}
	s = s[1:]
	if len(s) == 0 || s[0] != '"' {
		return nil, "", errors.New("expected '\"' after = in attribute selector")
	}
	s = s[1:]
	i := 0
	for ; i < len(s) && s[i] != '"'; i++ {
	}
	attributeValue, s := s[:i], s[i:]
	if len(s) == 0 || s[0] != '"' {
		return nil, "", errors.New("expected '\"' after attribute value")
	}
	s = s[1:]
	if len(s) == 0 || s[0] != ']' {
		return nil, "", errors.New("expected ']' at end of attribute selector")
	}
	s = s[1:]
	return &attributeSelector{attribute: attributeName, value: attributeValue}, s, nil
}

func isLetter(b byte) bool {
	return ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z')
}

func isNumber(b byte) bool {
	return ('0' <= b && b <= '9')
}

// consumeIdentifier consumes and returns a identifier at the beginning
// of the given string, and the rest of the string.
func consumeIdentifier(s string) (letters, rest string) {
	i := 0
	for ; i < len(s); i++ {
		// must start with letter or hyphen or underscore
		if i == 0 {
			if !(isLetter(s[i]) || s[i] == '-' || s[i] == '_') {
				break
			}
		} else {
			if !(isLetter(s[i]) || isNumber(s[i]) || s[i] == '-' || s[i] == '_') {
				break
			}
		}
		// CSS doesn't allow identifiers to start with two hyphens or a hyphen
		// followed by a digit, but we'll allow it.
	}
	return s[:i], s[i:]
}

func isAscii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// elementSelector matches a node that has the given element name.
type elementSelector struct {
	name string
}

func (s *elementSelector) match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	return n.Data == s.name
}

type attributeSelector struct {
	attribute, value string
}

// attributeSelector matches a node with an attribute that has a given value.
func (s *attributeSelector) match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for _, attr := range n.Attr {
		if attr.Key == s.attribute {
			return attr.Val == s.value
		}
	}
	return false
}

// idSelector matches an element that has the given id.
type idSelector struct {
	id string
}

func (s *idSelector) match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for _, attr := range n.Attr {
		if attr.Key == "id" {
			return attr.Val == s.id
		}
	}
	return false
}

// classSelector matches an element that has the given class set on it.
type classSelector struct {
	class string
}

func (s *classSelector) match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			for _, f := range strings.Fields(attr.Val) {
				if f == s.class {
					return true
				}
			}
			break
		}
	}
	return false
}
