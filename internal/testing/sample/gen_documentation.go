// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

// Run this to generate documentation.go.

package main

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"

	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func main() {
	ctx := context.Background()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../../godoc/testdata/p", nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}
	p := godoc.NewPackage(fset, sample.GOOS, sample.GOARCH, nil)
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			p.AddFile(f, true)
		}
	}
	src, err := p.Encode(ctx)
	if err != nil {
		log.Fatal(err)
	}
	si := source.NewGitHubInfo(sample.ModulePath, "", "abcde")
	mi := &godoc.ModuleInfo{
		ModulePath:      sample.ModulePath,
		ResolvedVersion: sample.VersionString,
		ModulePackages:  nil,
	}
	_, _, html, err := p.Render(ctx, "p", si, mi, "", "")
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Create("documentation.gen.go")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(f, `
// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sample

import (
	"github.com/google/safehtml/testconversions"
	"golang.org/x/pkgsite/internal"
)



var (
	DocumentationHTML = testconversions.MakeHTMLForTest(%s)
	DocumentationSource = %#v
	Documentation = &internal.Documentation{
		Synopsis: Synopsis,
		GOOS:     GOOS,
		GOARCH:   GOARCH,
		HTML:     DocumentationHTML,
		Source:   DocumentationSource,
	}
)
`, "`"+html.String()+"`", src)
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
