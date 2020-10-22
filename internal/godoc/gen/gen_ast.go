// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/ast"
	"log"

	"golang.org/x/pkgsite/internal/godoc"
	"golang.org/x/pkgsite/internal/godoc/codec"
)

func main() {
	types := []interface{}{
		ast.ArrayType{},
		ast.AssignStmt{},
		ast.BadDecl{},
		ast.BadExpr{},
		ast.BadStmt{},
		ast.BasicLit{},
		ast.BinaryExpr{},
		ast.BlockStmt{},
		ast.BranchStmt{},
		ast.CallExpr{},
		ast.CaseClause{},
		ast.ChanType{},
		ast.CommClause{},
		ast.CommentGroup{},
		ast.Comment{},
		ast.CompositeLit{},
		ast.DeclStmt{},
		ast.DeferStmt{},
		ast.Ellipsis{},
		ast.EmptyStmt{},
		ast.ExprStmt{},
		ast.FieldList{},
		ast.Field{},
		ast.ForStmt{},
		ast.FuncDecl{},
		ast.FuncLit{},
		ast.FuncType{},
		ast.GenDecl{},
		ast.GoStmt{},
		ast.Ident{},
		ast.IfStmt{},
		ast.ImportSpec{},
		ast.IncDecStmt{},
		ast.IndexExpr{},
		ast.InterfaceType{},
		ast.KeyValueExpr{},
		ast.LabeledStmt{},
		ast.MapType{},
		ast.ParenExpr{},
		ast.RangeStmt{},
		ast.ReturnStmt{},
		ast.Scope{},
		ast.SelectStmt{},
		ast.SelectorExpr{},
		ast.SendStmt{},
		ast.SliceExpr{},
		ast.StarExpr{},
		ast.StructType{},
		ast.SwitchStmt{},
		ast.TypeAssertExpr{},
		ast.TypeSpec{},
		ast.TypeSwitchStmt{},
		ast.UnaryExpr{},
		ast.ValueSpec{},
	}
	// Add in some unexported types in the godoc package. Since they are unexported, we can't
	// write their names here, but the godoc package can provide us with values of those types,
	// which the reflect package can examine.
	types = append(types, godoc.TypesToGenerate...)
	// This is run by a "go generate" command in the internal/godoc directory, so that
	// is the current working directory. That is where we want the output file to be.
	const filename = "encode_ast.go"
	if err := codec.GenerateFile(filename, "godoc", types...); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Wrote %s.\n", filename)
}
