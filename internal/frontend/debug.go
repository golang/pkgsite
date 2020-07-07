// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package frontend

import (
	"fmt"
	"strings"

	"github.com/russross/blackfriday/v2"
	"golang.org/x/net/html"
)

func dumpBlackfridayNode(n *blackfriday.Node, level int) string {
	var sb strings.Builder
	tab := strings.Repeat("\t", level)
	fmt.Fprintf(&sb, "%sType: %+v\n", tab, n.Type.String())
	fmt.Fprintf(&sb, "%sType: %+v\n", tab, n.Type.String())
	fmt.Fprintf(&sb, "%sLiteral: %+v", tab, string(n.Literal))
	fmt.Fprintf(&sb, "\n%sHeading Data: %+v", tab, n.HeadingData)
	fmt.Fprintf(&sb, "\n%sList Data: %+v", tab, n.ListData)
	fmt.Fprintf(&sb, "\n%sCodeBlock Data: %+v", tab, n.CodeBlockData)
	fmt.Fprintf(&sb, "\n%sLink Data: %+v", tab, n.LinkData)
	fmt.Fprintf(&sb, "\n%sTableCell Data: %+v\n", tab, n.TableCellData)
	for c := n.FirstChild; c != nil; c = c.Next {
		sb.WriteString(dumpBlackfridayNode(c, level+1))
	}
	return sb.String()
}

func dumpHTML(n *html.Node, level int) string {
	nodes := []string{
		"ErrorNode",
		"TextNode",
		"DocumentNode",
		"ElementNode",
		"CommentNode",
		"DoctypeNode",
	}
	var sb strings.Builder
	tab := strings.Repeat("\t", level)
	fmt.Fprintf(&sb, "%sType: %+v\n", tab, nodes[n.Type])
	fmt.Fprintf(&sb, "%sDataAtom: %+v\n", tab, n.DataAtom)
	fmt.Fprintf(&sb, "%sData: %+v\n", tab, n.Data)
	fmt.Fprintf(&sb, "%sNamespace: %+v\n", tab, n.Namespace)
	sb.WriteString(tab + "Attr: [")
	for _, attr := range n.Attr {
		fmt.Fprintf(&sb, "{Namespace: %+v, Key: %+v, Val: %+v}", attr.Namespace, attr.Key, attr.Val)
	}
	sb.WriteString("]")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(dumpHTML(c, level+1))
	}
	return sb.String()
}
