// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var packageToTest string = filepath.Join(runtime.GOROOT(), "src", "net", "http")

func TestEncodeDecodePackage(t *testing.T) {
	p, err := packageForDir(packageToTest, true)
	if err != nil {
		t.Fatal(err)
	}
	var want, got bytes.Buffer
	printPackage(&want, p)
	data, err := p.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p2, err := DecodePackage(data)
	if err != nil {
		t.Fatal(err)
	}
	printPackage(&got, p2)
	// Diff the textual output of printPackage, because cmp.Diff takes too long
	// on the Packages themselves.
	if diff := cmp.Diff(want.String(), got.String()); diff != "" {
		t.Errorf("package differs after decoding (-want, +got):\n%s", diff)
	}
}

func TestObjectIdentity(t *testing.T) {
	// Check that encoding and decoding preserves object identity.
	ctx := context.Background()
	const file = `
package p
var a int
func main() { a = 1 }
`

	compareObjs := func(f *ast.File) {
		t.Helper()
		// We know (from hand-inspecting the output of ast.Fprintf) that these two
		// objects are identical in the above program.
		o1 := f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Names[0].Obj
		o2 := f.Decls[1].(*ast.FuncDecl).Body.List[0].(*ast.AssignStmt).Lhs[0].(*ast.Ident).Obj
		if o1 != o2 {
			t.Fatal("objects not identical")
		}
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", file, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	compareObjs(f)

	p := NewPackage(fset, nil)
	p.AddFile(f, false)
	data, err := p.Encode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err = DecodePackage(data)
	if err != nil {
		t.Fatal(err)
	}
	compareObjs(p.Files[0].AST)
}

func packageForDir(dir string, removeNodes bool) (*Package, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	p := NewPackage(fset, nil)
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			p.AddFile(f, removeNodes)
		}
	}
	return p, nil
}

// Compare the time to decode AST files with and without
// removing parts of the AST not relevant to documentation.
//
// Run on a cloudtop 9/29/2020:
// - data size is 3.5x smaller
// - decode time is 4.5x faster
func BenchmarkRemovingAST(b *testing.B) {
	for _, removeNodes := range []bool{false, true} {
		b.Run(fmt.Sprintf("removeNodes=%t", removeNodes), func(b *testing.B) {
			p, err := packageForDir(packageToTest, removeNodes)
			if err != nil {
				b.Fatal(err)
			}
			data, err := p.Encode(context.Background())
			if err != nil {
				b.Fatal(err)
			}
			b.Logf("len(data) = %d", len(data))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := DecodePackage(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// printPackage outputs a human-readable form of p to w, deterministically. (The
// ast.Fprint function does not print ASTs deterministically: it is subject to
// random-order map iteration.) The output is designed to be diffed.
func printPackage(w io.Writer, p *Package) error {
	if err := printFileSet(w, p.Fset); err != nil {
		return err
	}
	var mpps []string
	for k := range p.ModulePackagePaths {
		mpps = append(mpps, k)
	}
	sort.Strings(mpps)
	if _, err := fmt.Fprintf(w, "ModulePackagePaths: %v\n", mpps); err != nil {
		return err
	}

	for _, pf := range p.Files {
		if _, err := fmt.Fprintf(w, "---- %s\n", pf.Name); err != nil {
			return err
		}
		if err := printNode(w, pf.AST); err != nil {
			return err
		}
	}
	return nil
}

func printNode(w io.Writer, root ast.Node) error {
	var err error
	seen := map[any]int{}

	pr := func(format string, args ...any) {
		if err == nil {
			_, err = fmt.Fprintf(w, format, args...)
		}
	}

	indent := func(d int) {
		for i := 0; i < d; i++ {
			pr("  ")
		}
	}

	var prValue func(any, int)
	prValue = func(x any, depth int) {
		indent(depth)
		if x == nil || reflect.ValueOf(x).IsNil() {
			pr("nil\n")
			return
		}
		ts := strings.TrimPrefix(fmt.Sprintf("%T", x), "*ast.")
		if idx, ok := seen[x]; ok {
			pr("%s@%d\n", ts, idx)
			return
		}
		idx := len(seen)
		seen[x] = idx
		pr("%s#%d", ts, idx)
		if obj, ok := x.(*ast.Object); ok {
			pr(" %s %s %v\n", obj.Name, obj.Kind, obj.Data)
			prValue(obj.Decl, depth+1)
			return
		}
		n, ok := x.(ast.Node)
		if !ok {
			pr(" %v\n", x)
			return
		}
		pr(" %d-%d", n.Pos(), n.End())
		switch n := n.(type) {
		case *ast.Ident:
			pr(" %q\n", n.Name)
			if n.Obj != nil {
				prValue(n.Obj, depth+1)
			}
		case *ast.BasicLit:
			pr(" %s %s %d\n", n.Value, n.Kind, n.ValuePos)
		case *ast.UnaryExpr:
			pr(" %s\n", n.Op)
		case *ast.BinaryExpr:
			pr(" %s\n", n.Op)
		case *ast.Comment:
			pr(" %q\n", n.Text)
		case *ast.File:
			// Doc, Name and Decls are walked, but not Scope or Unresolved.
			if n.Scope != nil {
				pr(" Scope.Outer: %p\n", n.Scope.Outer)
				var keys []string
				for k := range n.Scope.Objects {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					pr("  key %q\n", k)
					prValue(n.Scope.Objects[k], depth+1)
				}
			}
			indent(depth)
			pr("unresolved:\n")
			for _, id := range n.Unresolved {
				prValue(id, depth+1)
			}
		default:
			pr("\n")
		}
		ast.Inspect(n, func(m ast.Node) bool {
			if m == n {
				return true
			}
			if m != nil {
				prValue(m, depth+1)
			}
			return false
		})
	}

	prValue(root, 0)
	return err
}

func printFileSet(w io.Writer, fset *token.FileSet) error {
	var err error
	fset.Iterate(func(f *token.File) bool {
		_, err = fmt.Fprintf(w, "%s %d %d %d\n", f.Name(), f.Base(), f.Size(), f.LineCount())
		return err == nil
	})
	return err
}
