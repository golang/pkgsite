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
