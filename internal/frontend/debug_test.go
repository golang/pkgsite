// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/russross/blackfriday/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func TestDumpHTML(t *testing.T) {
	for _, test := range []struct {
		name string
		node *html.Node
		want string
	}{
		{
			name: "Dumping an image html into a readable string",
			node: &html.Node{
				Type:      3,
				DataAtom:  atom.Img,
				Data:      "img",
				Namespace: "",
				Attr: []html.Attribute{
					{
						Namespace: "",
						Key:       "src",
						Val:       "https://raw.githubusercontent.com/pdfcpu/pdfcpu/v0.3.3/resources/Go-Logo_Aqua.png",
					},
					{
						Namespace: "",
						Key:       "width",
						Val:       "200",
					},
				},
			},
			want: "Type: ElementNode\nDataAtom: img\nData: img\nNamespace: \nAttr: [{Namespace: , Key: src, Val: https://raw.githubusercontent.com/pdfcpu/pdfcpu/v0.3.3/resources/Go-Logo_Aqua.png}{Namespace: , Key: width, Val: 200}]",
		},
	} {
		got := dumpHTML(test.node, 0)
		if diff := cmp.Diff(test.want, got); diff != "" {
			t.Errorf("%s: mismatch (-want +got):\n%s", test.name, diff)
		}
	}
}

func TestBlackfridayNode(t *testing.T) {
	bfNode := blackfriday.NewNode(blackfriday.HTMLBlock)
	bfNode.Literal = []byte("<p align=\"center\"><img src=\"foo.png\" /></p>")
	for _, test := range []struct {
		name string
		node *blackfriday.Node
		want string
	}{
		{
			name: "Dumping a blackfriday node into a readable string",
			node: bfNode,
			want: "Type: HTMLBlock\nType: HTMLBlock\nLiteral: <p align=\"center\"><img src=\"foo.png\" /></p>\nHeading Data: {Level:0 HeadingID: IsTitleblock:false}\nList Data: {ListFlags:0 Tight:false BulletChar:0 Delimiter:0 RefLink:[] IsFootnotesList:false}\nCodeBlock Data: {IsFenced:false Info:[] FenceChar:0 FenceLength:0 FenceOffset:0}\nLink Data: {Destination:[] Title:[] NoteID:0 Footnote:<nil>}\nTableCell Data: {IsHeader:false Align:0}\n",
		},
	} {
		got := dumpBlackfridayNode(test.node, 0)
		if diff := cmp.Diff(test.want, got); diff != "" {
			t.Errorf("%s: mismatch (-want +got):\n%s", test.name, diff)
		}
	}
}
