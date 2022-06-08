// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"math/rand"
	"net/http"
)

// searchTip represents a snippet of text on the homepage demonstrating
// how to effectively use pkg.go.dev search.
type searchTip struct {
	Text     string
	Example1 string
	Example2 string
}

var searchTips = []searchTip{
	{
		"Search for a package, for example",
		"http",
		"command",
	},
	{
		"Search for a symbol, for example",
		"Unmarshal",
		"io.Reader",
	},
	{
		"Search for symbols within a package using the # filter. For example",
		"golang.org/x #error",
		"#reader io",
	},
}

// Homepage contains fields used in rendering the homepage template.
type homepage struct {
	basePage

	// TipIndex is the index of the initial search tip to render.
	TipIndex int

	// SearchTips is a collection of search tips to show on the homepage.
	SearchTips []searchTip
}

func (s *Server) serveHomepage(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	s.servePage(ctx, w, "homepage", homepage{
		basePage:   s.newBasePage(r, "Go Packages"),
		SearchTips: searchTips,
		TipIndex:   rand.Intn(len(searchTips)),
	})
}
