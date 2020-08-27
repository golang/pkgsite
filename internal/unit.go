// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"github.com/google/safehtml"
)

// Unit represents the contents of some path in the Go package/module
// namespace. It might be a module, a package, both a module and a package, or
// none of the above: a directory within a module that has no .go files, but
// contains other units, licenses and/or READMEs."
type Unit struct {
	DirectoryMeta
	Readme  *Readme
	Package *Package
	Imports []string
}

// Documentation is the rendered documentation for a given package
// for a specific GOOS and GOARCH.
type Documentation struct {
	// The values of the GOOS and GOARCH environment variables used to parse the
	// package.
	GOOS     string
	GOARCH   string
	Synopsis string
	HTML     safehtml.HTML
}

// Readme is a README at the specified filepath.
type Readme struct {
	Filepath string
	Contents string
}
