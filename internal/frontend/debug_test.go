// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore
// +build ignore

package frontend

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
