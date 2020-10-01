// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"go/ast"
	"go/token"
	"io"

	"golang.org/x/pkgsite/internal/derrors"
)

// encodingType identifies the encoding being used, in case
// we ever use a different one and need to distinguish them
// when reading from the DB.
// It should be a four-byte string.
const encodingType = "AST1"

// Register ast types for gob, so it can decode concrete types that are stored
// in interface variables.
func init() {
	for _, n := range []interface{}{
		&ast.ArrayType{},
		&ast.AssignStmt{},
		&ast.BasicLit{},
		&ast.BinaryExpr{},
		&ast.BlockStmt{},
		&ast.BranchStmt{},
		&ast.CallExpr{},
		&ast.CaseClause{},
		&ast.CompositeLit{},
		&ast.DeclStmt{},
		&ast.DeferStmt{},
		&ast.Ellipsis{},
		&ast.ExprStmt{},
		&ast.ForStmt{},
		&ast.FuncDecl{},
		&ast.FuncLit{},
		&ast.FuncType{},
		&ast.GenDecl{},
		&ast.GoStmt{},
		&ast.KeyValueExpr{},
		&ast.IfStmt{},
		&ast.ImportSpec{},
		&ast.IncDecStmt{},
		&ast.IndexExpr{},
		&ast.InterfaceType{},
		&ast.MapType{},
		&ast.ParenExpr{},
		&ast.RangeStmt{},
		&ast.ReturnStmt{},
		&ast.Scope{},
		&ast.SelectorExpr{},
		&ast.SliceExpr{},
		&ast.StarExpr{},
		&ast.StructType{},
		&ast.SwitchStmt{},
		&ast.TypeAssertExpr{},
		&ast.TypeSpec{},
		&ast.TypeSwitchStmt{},
		&ast.UnaryExpr{},
		&ast.ValueSpec{},
		&ast.Ident{},
	} {
		gob.Register(n)
	}
}

// Encode encodes a Package into a byte slice.
// During its operation, Encode modifies the AST,
// but it restores it to a state suitable for
// rendering before it returns.
func (p *Package) Encode() (_ []byte, err error) {
	defer derrors.Wrap(&err, "godoc.Package.Encode()")

	for _, f := range p.Files {
		removeCycles(f.AST)
	}

	var buf bytes.Buffer
	io.WriteString(&buf, encodingType)
	enc := gob.NewEncoder(&buf)
	// Encode the fset using the Write method it provides.
	if err := p.Fset.Write(enc.Encode); err != nil {
		return nil, err
	}
	if err := enc.Encode(p.gobPackage); err != nil {
		return nil, err
	}
	for _, f := range p.Files {
		fixupObjects(f.AST)
	}
	return buf.Bytes(), nil
}

// DecodPackage decodes a byte slice encoded with Package.Encode into a Package.
func DecodePackage(data []byte) (_ *Package, err error) {
	defer derrors.Wrap(&err, "DecodePackage()")

	le := len(encodingType)
	if len(data) < le || string(data[:le]) != encodingType {
		return nil, fmt.Errorf("want initial bytes to be %q but they aren't", encodingType)
	}
	dec := gob.NewDecoder(bytes.NewReader(data[le:]))
	p := &Package{Fset: token.NewFileSet()}
	if err := p.Fset.Read(dec.Decode); err != nil {
		return nil, err
	}
	if err := dec.Decode(&p.gobPackage); err != nil {
		return nil, err
	}
	for _, f := range p.Files {
		fixupObjects(f.AST)
	}
	return p, nil
}

// removeCycles removes cycles from f. There are two sources of cycles
// in an ast.File: Scopes and Objects.
//
// removeCycles removes all Scopes, since doc generation doesn't use them. Doc
// generation does use Objects, and it needs object identity to be preserved
// (see internal/fetch/internal/doc/example.go). But it doesn't need the Decl,
// Data or Type fields of ast.Object, which are responsible for cycles.
//
// If we just nulled out those three fields, there would be no cycles, but we
// wouldn't preserve Object identity when we decoded. For example, if ast.Idents
// A and B both pointed to the same Object, gob would write them as two separate
// objects, and decoding would preserve that. (See TestObjectIdentity for
// a small example of this sort of sharing.)
//
// So after nulling out those fields, we place a unique integer into the Decl
// field if one isn't there already. (Decl would never normally hold an int.)
// That serves to give a unique label to each object, which decoding can use
// to reconstruct the original set of relationships.
func removeCycles(f *ast.File) {
	next := 0
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.File:
			n.Scope.Objects = nil // doc doesn't use scopes
		case *ast.Ident:
			if n.Obj != nil {
				if _, ok := n.Obj.Decl.(int); !ok {
					n.Obj.Data = nil
					n.Obj.Type = nil
					n.Obj.Decl = next
					next++
				}
			}
		}
		return true
	})
}

// fixupObjects re-establishes the original object relationships of the ast.File f.
//
// f is the result of EncodeASTFiles, which uses removeCycles (see above) to
// modify ast.Objects so that they are uniquely identified by their Decl field.
// fixupObjects uses that value to reconstruct the same set of relationships.
func fixupObjects(f *ast.File) {
	objs := map[int]*ast.Object{}
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if id.Obj != nil {
				n := id.Obj.Decl.(int)
				if obj := objs[n]; obj != nil {
					// If we've seen object n before, then id.Obj should be the same object.
					id.Obj = obj
				} else {
					// If we haven't seen object n before, then remember it.
					objs[n] = id.Obj
				}
			}
		}
		return true
	})
}
