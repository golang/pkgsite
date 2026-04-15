// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
)

func runPackage(fs *flag.FlagSet, p *packageFlags, stdout, stderr io.Writer) int {
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "Error: expected exactly 1 package argument, got %d\n", fs.NArg())
		fs.Usage()
		return 2
	}
	path, version := splitPathVersion(fs.Arg(0))

	c := newClient(p.server)
	pkg, err := c.getPackage(path, version, p)
	if err != nil {
		return handleErr(stdout, stderr, err, p.jsonOut)
	}
	result := packageResult{Package: pkg}

	if p.symbols {
		syms, err := c.getSymbols(path, version, p)
		if err != nil {
			return handleErr(stdout, stderr, err, p.jsonOut)
		}
		result.Symbols = syms
	}
	if p.importedBy {
		ib, err := c.getImportedBy(path, version, p)
		if err != nil {
			return handleErr(stdout, stderr, err, p.jsonOut)
		}
		result.ImportedBy = ib
	}

	if p.jsonOut {
		return writeJSON(stdout, stderr, result)
	}
	formatPackage(stdout, result)
	return 0
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
