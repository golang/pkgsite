// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"sort"

	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/codec"
)

// The encoding type identifies the encoding being used, to distinguish them
// when reading from the DB.
const (
	encodingTypeLen  = 4 // all encoding types must be this many bytes
	gobEncodingType  = "AST1"
	fastEncodingType = "AST2"
)

// ErrInvalidEncodingType is returned when the data to DecodePackage has an
// invalid encoding type.
var ErrInvalidEncodingType = fmt.Errorf("want initial bytes to be %q or %q but they aren't", gobEncodingType, fastEncodingType)

// Register ast types for gob, so it can decode concrete types that are stored
// in interface variables.
func init() {
	for _, n := range []interface{}{
		&ast.ArrayType{},
		&ast.AssignStmt{},
		&ast.BadDecl{},
		&ast.BadExpr{},
		&ast.BadStmt{},
		&ast.BasicLit{},
		&ast.BinaryExpr{},
		&ast.BlockStmt{},
		&ast.BranchStmt{},
		&ast.CallExpr{},
		&ast.CaseClause{},
		&ast.ChanType{},
		&ast.CommClause{},
		&ast.CommentGroup{},
		&ast.Comment{},
		&ast.CompositeLit{},
		&ast.DeclStmt{},
		&ast.DeferStmt{},
		&ast.Ellipsis{},
		&ast.EmptyStmt{},
		&ast.ExprStmt{},
		&ast.FieldList{},
		&ast.Field{},
		&ast.ForStmt{},
		&ast.FuncDecl{},
		&ast.FuncLit{},
		&ast.FuncType{},
		&ast.GenDecl{},
		&ast.GoStmt{},
		&ast.Ident{},
		&ast.IfStmt{},
		&ast.ImportSpec{},
		&ast.IncDecStmt{},
		&ast.IndexExpr{},
		&ast.InterfaceType{},
		&ast.KeyValueExpr{},
		&ast.LabeledStmt{},
		&ast.MapType{},
		&ast.ParenExpr{},
		&ast.RangeStmt{},
		&ast.ReturnStmt{},
		&ast.Scope{},
		&ast.SelectStmt{},
		&ast.SelectorExpr{},
		&ast.SendStmt{},
		&ast.SliceExpr{},
		&ast.StarExpr{},
		&ast.StructType{},
		&ast.SwitchStmt{},
		&ast.TypeAssertExpr{},
		&ast.TypeSpec{},
		&ast.TypeSwitchStmt{},
		&ast.UnaryExpr{},
		&ast.ValueSpec{},
	} {
		gob.Register(n)
	}
}

// Encode encodes a Package into a byte slice.
// During its operation, Encode modifies the AST,
// but it restores it to a state suitable for
// rendering before it returns.
func (p *Package) Encode(ctx context.Context) (_ []byte, err error) {
	defer derrors.Wrap(&err, "godoc.Package.Encode()")
	return p.fastEncode()
}

// DecodPackage decodes a byte slice encoded with Package.Encode into a Package.
func DecodePackage(data []byte) (_ *Package, err error) {
	defer derrors.Wrap(&err, "DecodePackage()")

	if len(data) < encodingTypeLen {
		return nil, ErrInvalidEncodingType
	}
	switch string(data[:encodingTypeLen]) {
	case gobEncodingType:
		return gobDecodePackage(data[encodingTypeLen:])
	case fastEncodingType:
		return fastDecodePackage(data[encodingTypeLen:])
	default:
		return nil, ErrInvalidEncodingType
	}
}

func gobDecodePackage(data []byte) (_ *Package, err error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	p := &Package{Fset: token.NewFileSet()}
	if err := p.Fset.Read(dec.Decode); err != nil {
		return nil, err
	}
	if err := dec.Decode(&p.encPackage); err != nil {
		return nil, err
	}
	for _, f := range p.Files {
		fixupObjects(f)
	}
	return p, nil
}

// removeCycles removes cycles from f. There are two sources of cycles
// in an ast.File: Scopes and Objects. Also, some Idents are shared.
//
// removeCycles removes all Scopes, since doc generation doesn't use them. Doc
// generation does use Objects, and it needs object identity to be preserved
// (see internal/doc/example.go). It also needs the Object.Decl field, to create
// anchor links (see dochtml/internal/render/idents.go). The Object.Decl field
// is responsible for cycles. Doc generation It doesn't need the Data or Type
// fields of Object.
//
// We need to break the cycles, and preserve Object identity when decoding. For
// an example of the latter, if ast.Idents A and B both pointed to the same
// Object, gob would write them as two separate objects, and decoding would
// preserve that. (See TestObjectIdentity for a small example of this sort of
// sharing.)
//
// We solve both problems by assigning numbers to Decls and Objects. We first
// walk through the AST to assign the numbers, then walk it again to put the
// numbers into Ident.Objs. We take advantage of the fact that the Data and Decl
// fields are of type interface{}, storing the object number into Data and the
// Decl number into Decl.
//
// The AST includes a list of unresolved Idents, which are shared with Idents
// in the tree itself. We assign these numbers as well, and store the numbers
// in a separate field of File.
func removeCycles(f *File) {
	// First pass: assign every Decl, Spec and Ident a number.
	// Since these aren't shared and Inspect is deterministic,
	// this walk will produce the same sequence of Decls after encoding/decoding.
	// Also assign a unique number to each Object we find in an Ident.
	// Objects may be shared; traversing the decoded AST would not
	// produce the same sequence. So we store their numbers separately.
	declNums := map[interface{}]int{}
	objNums := map[*ast.Object]int{}
	ast.Inspect(f.AST, func(n ast.Node) bool {
		if isRelevantDecl(n) {
			if _, ok := declNums[n]; ok {
				panic(fmt.Sprintf("duplicate decl %+v", n))
			}
			declNums[n] = len(declNums)
		} else if id, ok := n.(*ast.Ident); ok {
			declNums[id] = len(declNums) // remember Idents for Unresolved list.
			if id.Obj != nil {
				if _, ok := objNums[id.Obj]; !ok {
					objNums[id.Obj] = len(objNums)
				}
			}
		}
		return true
	})

	// Second pass: put the numbers into Ident.Objs.
	// The Decl field gets a number from the declNums map, or nil
	// if it's not a relevant Decl.
	// The Data field gets a number from the objNums map. (This destroys
	// whatever might be in the Data field, but doc generation doesn't care.)
	ast.Inspect(f.AST, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Obj == nil {
			return true
		}
		if _, ok := id.Obj.Decl.(int); ok { // seen this object already
			return true
		}
		id.Obj.Type = nil // Not needed for doc gen.
		id.Obj.Data, ok = objNums[id.Obj]
		if !ok {
			panic(fmt.Sprintf("no number for Object %v", id.Obj))
		}
		if d, ok := declNums[id.Obj.Decl]; ok {
			id.Obj.Decl = d
		} else {
			// We may not have seen this Ident's Decl because the definition was
			// removed from the AST, even though references remain. For example,
			// an exported var initialized to a call of an unexported function.
			// Ignore those by setting the Decl field to -1.
			id.Obj.Decl = -1
		}
		return true
	})

	// Replace the unresolved identifiers with their numbers.
	f.UnresolvedNums = nil
	for _, id := range f.AST.Unresolved {
		// If we can't find an identifier, assume it was in a part of the AST
		// deleted by removeUnusedASTNodes, and ignore it.
		if num, ok := declNums[id]; ok {
			f.UnresolvedNums = append(f.UnresolvedNums, num)
		}
	}
	f.AST.Unresolved = nil

	// Remember only those scope items that have been assigned a number; the others
	// are not relevant to doc (unexported functions, for instance).
	f.ScopeItems = nil
	for name, obj := range f.AST.Scope.Objects {
		if num, ok := obj.Data.(int); ok {
			f.ScopeItems = append(f.ScopeItems, scopeItem{name, num})
		}
	}
	// Sort for deterministic encoding.
	sort.Slice(f.ScopeItems, func(i, j int) bool {
		return f.ScopeItems[i].Name < f.ScopeItems[j].Name
	})
	f.AST.Scope.Objects = nil

}

// fixupObjects re-establishes the original Object and Decl relationships of the
// File.
//
// f is the result of Encode, which uses removeCycles (see above) to modify
// ast.Objects so that they are uniquely identified by their Data field, and
// refer to their Decl via a number in the Decl field. fixupObjects uses those
// values to reconstruct the same set of relationships.
func fixupObjects(f *File) {
	// First pass: reconstruct the numbers of every Decl and Ident.
	var decls []ast.Node
	ast.Inspect(f.AST, func(n ast.Node) bool {
		if _, ok := n.(*ast.Ident); ok || isRelevantDecl(n) {
			decls = append(decls, n)
		}
		return true
	})

	// Second pass: replace the numbers in Ident.Objs with the right Nodes.
	var objs []*ast.Object
	ast.Inspect(f.AST, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Obj == nil {
			return true
		}
		obj := id.Obj
		if obj.Data == nil {
			// We've seen this object already.
			// Possible if fixing up without serializing/deserializing, because
			// Objects are still shared in that case.
			// Do nothing.
			return true
		}
		num := obj.Data.(int)
		switch {
		case num < len(objs):
			// We've seen this Object before.
			id.Obj = objs[num]
		case num == len(objs):
			// A new object; fix it up and remember it.
			if obj.Decl != nil {
				num := obj.Decl.(int)
				if num >= 0 {
					obj.Decl = decls[num]
				}
			}
			objs = append(objs, obj)
		case num > len(objs):
			panic("n > len(objs); shouldn't happen")
		}
		return true
	})

	// Fix up unresolved identifiers.
	f.AST.Unresolved = make([]*ast.Ident, len(f.UnresolvedNums))
	for i, num := range f.UnresolvedNums {
		f.AST.Unresolved[i] = decls[num].(*ast.Ident)
	}
	f.UnresolvedNums = nil

	// Fix up file scope objects.
	f.AST.Scope.Objects = map[string]*ast.Object{}
	for _, item := range f.ScopeItems {
		f.AST.Scope.Objects[item.Name] = objs[item.Num]
	}
	f.ScopeItems = nil
}

// isRelevantDecl reports whether n is a Node for a declaration relevant to
// documentation.
func isRelevantDecl(n interface{}) bool {
	switch n.(type) {
	case *ast.FuncDecl, *ast.GenDecl, *ast.ValueSpec, *ast.TypeSpec, *ast.ImportSpec:
		return true
	default:
		return false
	}
}

func (p *Package) fastEncode() (_ []byte, err error) {
	defer derrors.Wrap(&err, "godoc.Package.FastEncode()")

	var buf bytes.Buffer
	io.WriteString(&buf, fastEncodingType)
	enc := codec.NewEncoder()
	fsb, err := fsetToBytes(p.Fset)
	if err != nil {
		return nil, err
	}
	if err := enc.Encode(fsb); err != nil {
		return nil, err
	}
	if err := enc.Encode(&p.encPackage); err != nil {
		return nil, err
	}
	buf.Write(enc.Bytes())
	return buf.Bytes(), nil
}

func fastDecodePackage(data []byte) (_ *Package, err error) {
	defer derrors.Wrap(&err, "FastDecodePackage()")

	dec := codec.NewDecoder(data)
	x, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	fsetBytes, ok := x.([]byte)
	if !ok {
		return nil, fmt.Errorf("first decoded value is %T, wanted []byte", fsetBytes)
	}
	fset, err := fsetFromBytes(fsetBytes)
	if err != nil {
		return nil, err
	}
	x, err = dec.Decode()
	if err != nil {
		return nil, err
	}
	ep, ok := x.(*encPackage)
	if !ok {
		return nil, fmt.Errorf("second decoded value is %T, wanted *encPackage", ep)
	}
	return &Package{
		Fset:       fset,
		encPackage: *ep,
	}, nil
}

// token.FileSet uses some unexported types in its encoding, so we can't use our
// own codec from it. Instead we use gob and encode the resulting bytes.
func fsetToBytes(fset *token.FileSet) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := fset.Write(enc.Encode); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fsetFromBytes(data []byte) (*token.FileSet, error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	fset := token.NewFileSet()
	if err := fset.Read(dec.Decode); err != nil {
		return nil, err
	}
	return fset, nil
}

//go:generate go run gen_ast.go

// Used by the gen program to generate encodings for unexported types.
var TypesToGenerate = []interface{}{&encPackage{}}
