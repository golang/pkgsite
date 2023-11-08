// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sanitizer provides a simple html sanitizer to remove
// tags and attributes that could potentially cause security issues.
package sanitizer

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// SanitizeBytes returns a sanitized version of the input.
// It throws out any attributes or tags that are not explicitly
// allowed in allowElems or allowAttributes below, including
// any child nodes of elements that are not allowed.
func SanitizeBytes(b []byte) []byte {
	// TODO(matloob): We want to sanitize a fragment that would
	// appear in the body. Can we call ParseFragment without
	// creating the body node here?
	document, err := html.Parse(strings.NewReader("<html><head></head><body></body></html>"))
	if err != nil {
		panic(fmt.Errorf("error parsing document: %v", err))
	}
	body := document.FirstChild.LastChild // document.FirstChild is the <html> node

	nodes, err := html.ParseFragment(bytes.NewReader(b), body)
	if err != nil {
		return []byte{}
	}
	var keepNodes []*html.Node
	for _, n := range nodes {
		if sanitize(n) {
			keepNodes = append(keepNodes, n)
		}
	}
	var buf bytes.Buffer
	for _, n := range keepNodes {
		html.Render(&buf, n)
	}
	return buf.Bytes()
}

// sanitize sanitizes the attributes and children of n.
// It returns false if the entire node should be cut out.
func sanitize(n *html.Node) bool {
	switch n.Type {
	case html.CommentNode:
		return false
	case html.DoctypeNode:
		return false
	case html.TextNode:
		return true // Assume text nodes are safe
	case html.ElementNode:
		if n.Namespace != "" {
			return false
		}
		n.Data = strings.ToLower(n.Data)
		if !allowElemsMap[n.Data] {
			return false
		}
		keepAttr := []html.Attribute{}
		for _, attr := range n.Attr {
			if attr.Namespace != "" {
				continue
			}
			if allow, ok := allowAttrMap[""][attr.Key]; ok {
				// This is an attribute that can be present on
				// any tag.
				if !allow(attr.Val) {
					continue
				}
			} else if allow, ok := allowAttrMap[n.Data][attr.Key]; ok {
				if !allow(attr.Val) {
					continue
				}
				if roundtripAttrs[attr.Key][n.Data] {
					attr.Val = roundtripURL(attr.Val)
					if attr.Val == "" {
						continue
					}
				}
			} else {
				// There is no allow function. It's not allowed.
				continue
			}
			keepAttr = append(keepAttr, attr)
		}
		if n.Data == "a" {
			keepAttr = addRelNoFollow(keepAttr)
		}
		n.Attr = keepAttr
		var removeChildren []*html.Node
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if !sanitize(child) {
				removeChildren = append(removeChildren, child)
			}
		}
		for _, child := range removeChildren {
			n.RemoveChild(child)
		}
		return true
	default:
		return false
	}
}

func addRelNoFollow(attrs []html.Attribute) []html.Attribute {
	hasHref := false
	for _, attr := range attrs {
		if attr.Namespace == "" && attr.Key == "href" {
			hasHref = true
		}
	}
	if !hasHref {
		return attrs
	}
	hasRel := false
	for i := range attrs {
		if attrs[i].Namespace == "" && attrs[i].Key == "rel" {
			hasRel = true
			attrs[i].Val = "nofollow"
		}
	}
	if !hasRel {
		attrs = append(attrs, html.Attribute{Key: "rel", Val: "nofollow"})
	}
	return attrs
}

var allowAttrMap map[string]map[string]func(string) bool
var allowElemsMap map[string]bool

func init() {
	allowAttrMap = make(map[string]map[string]func(string) bool)
	for _, aa := range allowAttrs {
		if allowAttrMap[aa.elem] == nil {
			allowAttrMap[aa.elem] = make(map[string]func(string) bool)
		}
		allowAttrMap[aa.elem][aa.attr] = aa.allow
	}
	allowElemsMap = make(map[string]bool)
	for _, ae := range allowElems {
		allowElemsMap[ae] = true
	}
}

// allowElems is the list of elements that are allowed in the
// sanitized html.
var allowElems = []string{
	"a",
	"abbr",
	"article",
	"aside",
	"b",
	"bdi",
	"bdo",
	"blockquote",
	"br",
	"caption",
	"cite",
	"code",
	"col",
	"colgroup",
	"details",
	"dd",
	"del",
	"div",
	"dl",
	"dt",
	"em",
	"figcaption",
	"figure",
	"h1",
	"h2",
	"h3",
	"h4",
	"h5",
	"h6",
	"hr",
	"i",
	"img",
	"ins",
	"li",
	"mark",
	"ol",
	"p",
	"pre",
	"q",
	"rp",
	"rt",
	"ruby",
	"s",
	"samp",
	"section",
	"small",
	"span",
	"strike",
	"strong",
	"sub",
	"sup",
	"summary",
	"table",
	"tbody",
	"tfoot",
	"thead",
	"time",
	"td",
	"th",
	"tr",
	"tt",
	"u",
	"ul",
	"var",
	"wbr",
}

type allowAttr struct {
	elem  string // "" to apply to all elements
	attr  string
	allow func(string) bool
}

var allowAttrs = []allowAttr{
	// bluemonday AllowStandardAttributes
	{"", "dir", re(`^(?i)(rtl|ltr)$`)},
	{"", "lang", re(`^[a-zA-Z]{2,20}$`)},
	{"", "id", re(`^[a-zA-Z0-9\:\-_\.]+$`)},
	{"", "title", para},

	{"details", "open", re(`(?i)^(|open)$`)},
	{"blockquote", "cite", validURL},
	{"a", "href", validURL},
	{"bdi", "dir", re(`(?i)^(rtl|ltr)$`)},
	{"bdo", "dir", re(`(?i)^(rtl|ltr)$`)},
	{"map", "name", re(`([\p{L}\p{N}_-]+)`)},
	{"img", "usemap", re(`(?i)^#[\p{L}\p{N}_-]+$`)},
	{"img", "src", validURL},
	{"img", "align", re(`(?i)^(left|right|top|texttop|middle|absmiddle|baseline|bottom|absbottom)$`)},
	{"img", "alt", para},
	{"img", "height", numOrPercent},
	{"img", "width", re(`^[0-9]+([%]|[a-z]+)?;?/?$`)}, // a hacky regexp to allow most commonly appearing width errors through
	{"div", "align", align},
	{"div", "width", numOrPercent},
	{"div", "role", re(`^[a-z]+$`)},
	{"div", "aria-level", integer},
	{"del", "cite", para},
	{"del", "datetime", iso8601},
	{"ins", "cite", para},
	{"ins", "datetime", iso8601},
	{"p", "align", align},        // pkgsite allows all values
	{"p", "width", numOrPercent}, // pkgsite allows all values
	{"q", "cite", validURL},
	{"time", "datetime", iso8601},
	{"ol", "type", re(`(?i)^(circle|disc|square|a|A|i|I|1)$`)},
	{"ul", "type", re(`(?i)^(circle|disc|square|a|A|i|I|1)$`)},
	{"li", "type", re(`(?i)^(circle|disc|square|a|A|i|I|1)$`)},
	{"li", "value", integer},

	{"table", "height", numOrPercent},
	{"table", "width", numOrPercent},
	{"table", "summary", para},
	{"col", "align", align},
	{"col", "height", numOrPercent},
	{"col", "width", numOrPercent},
	{"col", "span", integer},
	{"col", "valign", valign},
	{"colgroup", "align", align},
	{"colgroup", "height", numOrPercent},
	{"colgroup", "width", numOrPercent},
	{"colgroup", "span", integer},
	{"colgroup", "valign", valign},
	{"thead", "align", align},
	{"thead", "valign", valign},
	{"tr", "align", align},
	{"tr", "valign", valign},
	{"td", "abbr", para},
	{"td", "align", align},
	{"td", "colspan", integer},
	{"td", "rowspan", integer},
	{"td", "headers", spaceSepTokens},
	{"td", "height", numOrPercent},
	{"td", "width", numOrPercent},
	{"td", "scope", re(`(?i)(?:row|col)(?:group)?`)},
	{"td", "valign", valign},
	{"td", "nowrap", re(`(?i)|nowrap`)},
	{"th", "abbr", para},
	{"th", "align", align},
	{"th", "colspan", integer},
	{"th", "rowspan", integer},
	{"th", "headers", spaceSepTokens},
	{"th", "height", numOrPercent},
	{"th", "width", numOrPercent},
	{"th", "scope", re(`(?i)(?:row|col)(?:group)?`)},
	{"th", "valign", valign},
	{"th", "nowrap", re(`(?i)|nowrap`)},
	{"tbody", "align", align},
	{"tbody", "valign", valign},
	{"tfoot", "align", align},
	{"tfoot", "valign", valign},

	// Needed to preserve github styles heading font-sizes
	// Can we be stricter than spaceSepTokens?
	{"h1", "class", spaceSepTokens},
	{"h2", "class", spaceSepTokens},
	{"h3", "class", spaceSepTokens},
	{"h4", "class", spaceSepTokens},
	{"h5", "class", spaceSepTokens},
	{"h6", "class", spaceSepTokens},
}

// roundtripAttrs is a map from attribute keys which should be checked
// against roundtripURL to maps of tags which are allowed to have them.
var roundtripAttrs = map[string]map[string]bool{
	"src":  {"img": true},
	"href": {"a": true},
	"cite": {
		"blockquote": true,
		"del":        true,
		"ins":        true,
		"q":          true,
	},
}

var align = re(`(?i)^(center|justify|left|right)$`)

var valign = re(`(?i)^(baseline|bottom|middle|top)$`)

var para = re(`^[\p{L}\p{N}\s\-_',\[\]!\./\\\(\)]*$`)

var spaceSepTokens = re(`^([\s\p{L}\p{N}_-]+)$`)

var numOrPercent = re(`^[0-9]+[%]?$`)

var integer = re(`^[0-9]+$`)

var iso8601 = re(`^[0-9]{4}(-[0-9]{2}(-[0-9]{2}([ T][0-9]{2}(:[0-9]{2}){1,2}(.[0-9]{1,6})` +
	`?Z?([\+-][0-9]{2}:[0-9]{2})?)?)?)?$`)

func re(rx string) func(string) bool {
	return regexp.MustCompile(rx).MatchString
}

func validURL(rawurl string) bool {
	rawurl = strings.TrimSpace(rawurl)

	if strings.ContainsAny(rawurl, " \t\n") {
		return false
	}

	u, err := url.Parse(rawurl)
	if err != nil {
		return false
	}

	switch u.Scheme {
	case "", "mailto", "http", "https":
		break
	default:
		return false
	}

	return true
}

func roundtripURL(rawurl string) string {
	u, err := url.Parse(strings.TrimSpace(rawurl))
	if err != nil {
		return ""
	}
	return u.String()
}
