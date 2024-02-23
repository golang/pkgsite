// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

type PackageDoc struct {
	ImportPath     string
	ModulePath     string
	Version        string
	NumImporters   int
	PackageDoc     string
	SymbolDocs     []SymbolDoc
	ReadmeFilename *string
	ReadmeContents *string
}

type SymbolDoc struct {
	Names []string // consts and vars may have multiple names
	Decl  string   // the declaration as a string
	Doc   string
}
