// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build go1.27

package render

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

func TestNewDeclIDsGenericMethod(t *testing.T) {
	const src = `
package p

type S struct{}

func (s *S) M[T any, P someConstraint](x T, y P) {}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			funcDecl = fd
			break
		}
	}
	if funcDecl == nil {
		t.Fatal("missing FuncDecl")
	}

	dids := newDeclIDs(funcDecl)
	if dids.recvType != "S" {
		t.Errorf("got recvType %q, want %q", dids.recvType, "S")
	}

	wantParamTypes := map[string]string{
		"s": "S",
		"T": "any",
		"P": "someConstraint",
		"x": "T",
		"y": "P",
	}
	if !reflect.DeepEqual(dids.paramTypes, wantParamTypes) {
		t.Errorf("got paramTypes %+v, want %+v", dids.paramTypes, wantParamTypes)
	}
}
