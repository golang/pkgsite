// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "flag"

const defaultServer = "https://pkg.go.dev"

// commonFlags are shared across all subcommands.
type commonFlags struct {
	jsonOut bool
	limit   int
	token   string
	server  string
}

func (f *commonFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&f.jsonOut, "json", false, "output JSON")
	fs.IntVar(&f.limit, "limit", 0, "max results (default: 20 text, 100 json)")
	fs.StringVar(&f.token, "token", "", "pagination token (JSON mode)")
	fs.StringVar(&f.server, "server", defaultServer, "API server URL")
}

func (f *commonFlags) effectiveLimit() int {
	if f.limit > 0 {
		return f.limit
	}
	if f.jsonOut {
		return 100
	}
	return 20
}

// packageFlags are flags for the package subcommand.
type packageFlags struct {
	commonFlags
	doc        string
	examples   bool
	imports    bool
	importedBy bool
	symbols    bool
	licenses   bool
	module     string
	goos       string
	goarch     string
}

func (f *packageFlags) register(fs *flag.FlagSet) {
	f.commonFlags.register(fs)
	fs.StringVar(&f.doc, "doc", "", "render docs in format: text, md, html")
	fs.BoolVar(&f.examples, "examples", false, "include examples (requires -doc)")
	fs.BoolVar(&f.imports, "imports", false, "list imported packages")
	fs.BoolVar(&f.importedBy, "imported-by", false, "list reverse dependencies")
	fs.BoolVar(&f.symbols, "symbols", false, "list exported symbols")
	fs.BoolVar(&f.licenses, "licenses", false, "show license information")
	fs.StringVar(&f.module, "module", "", "disambiguate module path")
	fs.StringVar(&f.goos, "goos", "", "target GOOS")
	fs.StringVar(&f.goarch, "goarch", "", "target GOARCH")
}

// moduleFlags are flags for the module subcommand.
type moduleFlags struct {
	commonFlags
	readme   bool
	licenses bool
}

// searchFlags are flags for the search subcommand.
type searchFlags struct {
	commonFlags
	symbol string
}
