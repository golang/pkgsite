// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// commonFlags are shared across all subcommands.
type commonFlags struct {
	jsonOut bool
	limit   int
	token   string
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
	doc      string
	examples bool
	imports  bool
	licenses bool
	module   string
	goos     string
	goarch   string
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
