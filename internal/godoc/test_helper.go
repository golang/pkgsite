// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"

	"golang.org/x/pkgsite/internal"
)

// DocumentationForTesting returns a Documentation value for the given Go source.
// It panics if there are errors parsing or encoding the source.
// It is intended for testing only.
//
// This should live in the testing/sample package, but it can't because of a circular dependency.
func DocumentationForTesting(goos, goarch, fileContents string) *internal.Documentation {
	fset := token.NewFileSet()
	pf, err := parser.ParseFile(fset, "sample.go", fileContents, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	docPkg := NewPackage(fset, nil)
	docPkg.AddFile(pf, true)
	src, err := docPkg.Encode(context.Background())
	if err != nil {
		panic(err)
	}
	return &internal.Documentation{
		GOOS:     goos,
		GOARCH:   goarch,
		Synopsis: fmt.Sprintf("synopsis for GOOS=%s, GOARCH=%s", goos, goarch),
		Source:   src,
	}
}
