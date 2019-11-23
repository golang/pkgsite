// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal/etl/internal/doc"
	"golang.org/x/net/html"
)

func TestRender(t *testing.T) {
	fset, d := mustLoadPackage("everydecl")

	rawDoc, err := Render(fset, d, RenderOptions{
		SourceLinkFunc: func(ast.Node) string { return "src" },
	})
	if err != nil {
		t.Fatal(err)
	}
	htmlDoc, err := html.Parse(strings.NewReader(rawDoc))
	if err != nil {
		t.Fatal(err)
	}
	// Check that there are no duplicate id attributes.
	t.Run("duplicate ids", func(t *testing.T) {
		testDuplicateIDs(t, htmlDoc)
	})
	t.Run("ids-and-kinds", func(t *testing.T) {
		// Check that the id and data-kind labels are right.
		testIDsAndKinds(t, htmlDoc)
	})
}

func testDuplicateIDs(t *testing.T, htmlDoc *html.Node) {
	idCounts := map[string]int{}
	walk(htmlDoc, func(n *html.Node) {
		id := attr(n, "id")
		if id != "" {
			idCounts[id]++
		}
	})
	var dups []string
	for id, n := range idCounts {
		if n > 1 {
			dups = append(dups, id)
		}
	}
	if len(dups) > 0 {
		t.Errorf("duplicate ids: %v", dups)
	}
}

func testIDsAndKinds(t *testing.T, htmlDoc *html.Node) {
	type attrs struct {
		ID, Kind string // export fields for cmp
	}

	// want is a complete list of id, kind pairs we expect to see the HTML.
	want := []attrs{
		{"C", "constant"},
		{"CT", "constant"},
		{"F", "function"},
		{"TF", "function"},
		{"T.M", "method"},
		{"V", "variable"},
		{"VT", "variable"},
		{"T", "type"},
		{"S1", "type"},
		{"S1.F", "field"},
		{"S2", "type"},
		{"S2.S1", "field"},
		{"S2.G", "field"},
		{"I1", "type"},
		{"I1.M1", "method"},
		{"I2", "type"},
		{"I2.M2", "method"},
	}

	var got []attrs
	walk(htmlDoc, func(n *html.Node) {
		if kind := attr(n, "data-kind"); kind != "" {
			got = append(got, attrs{attr(n, "id"), kind})
		}
	})

	diff := cmp.Diff(want, got, cmpopts.SortSlices(func(a1, a2 attrs) bool {
		return a1.ID < a2.ID
	}))
	if diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func walk(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// Copied from internal/render/render_test.go, with the slight modification of returning the fset.
func mustLoadPackage(path string) (*token.FileSet, *doc.Package) {
	// simpleImporter is used by ast.NewPackage.
	simpleImporter := func(imports map[string]*ast.Object, pkgPath string) (*ast.Object, error) {
		pkg := imports[pkgPath]
		if pkg == nil {
			pkgName := pkgPath[strings.LastIndex(pkgPath, "/")+1:]
			pkg = ast.NewObj(ast.Pkg, pkgName)
			pkg.Data = ast.NewScope(nil) // required for or dot-imports
			imports[pkgPath] = pkg
		}
		return pkg, nil
	}

	srcName := filepath.Base(path) + ".go"
	code, err := ioutil.ReadFile(filepath.Join("testdata", srcName))
	if err != nil {
		panic(err)
	}

	fset := token.NewFileSet()
	pkgFiles := make(map[string]*ast.File)
	astFile, _ := parser.ParseFile(fset, srcName, code, parser.ParseComments)
	pkgFiles[srcName] = astFile
	astPkg, _ := ast.NewPackage(fset, pkgFiles, simpleImporter, nil)
	return fset, doc.New(astPkg, path, 0)
}
