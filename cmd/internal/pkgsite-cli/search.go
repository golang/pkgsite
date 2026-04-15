// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"io"
	"strings"
)

func runSearch(fs *flag.FlagSet, s *searchFlags, stdout, stderr io.Writer) int {
	if fs.NArg() < 1 {
		fs.Usage()
		return 2
	}
	query := strings.Join(fs.Args(), " ")

	c := newClient(s.server)
	results, err := c.search(query, s)
	if err != nil {
		return handleErr(stdout, stderr, err, s.jsonOut)
	}

	if s.jsonOut {
		return writeJSON(stdout, stderr, results)
	}
	formatSearch(stdout, results)
	return 0
}

// searchFlags are flags for the search subcommand.
type searchFlags struct {
	commonFlags
	symbol string
}

func (f *searchFlags) register(fs *flag.FlagSet) {
	f.commonFlags.register(fs)
	fs.StringVar(&f.symbol, "symbol", "", "search for a symbol")
}
