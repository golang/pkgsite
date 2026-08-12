// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package api finds the API of a package: the set of exported
// symbols and their types.
package api

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
)

// typeString returns a string for the given type expression that represents the type.
// If two such strings are equal, then the corresponding types are equal.
// typeString returns "?" if typeExpr is nil.
func typeString(typeExpr ast.Expr) string {
	switch t := typeExpr.(type) {
	case nil:
		return "?"
	case *ast.FuncType:
		return "func" + sigString(t)
	default:
		return nodeString(t)
	}
}

// sigString returns a string representation of a function signature
// without parameter or return names (e.g., "(int, int) bool").
func sigString(ft *ast.FuncType) string {
	if ft == nil {
		return ""
	}
	paramTypes := fieldListTypes(ft.Params)
	resTypes := fieldListTypes(ft.Results)

	var buf strings.Builder
	buf.WriteString("(")
	buf.WriteString(strings.Join(paramTypes, ", "))
	buf.WriteString(")")

	if len(resTypes) == 1 {
		buf.WriteString(" ")
		buf.WriteString(resTypes[0])
	} else if len(resTypes) > 1 {
		buf.WriteString(" (")
		buf.WriteString(strings.Join(resTypes, ", "))
		buf.WriteString(")")
	}
	return buf.String()
}

// fieldListTypes returns the types of a field list, ignoring the field names.
// Note that "field" here means more than just a struct field: it could be
// the arguments or return values of a function.
func fieldListTypes(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var typeStrings []string
	for _, f := range fl.List {
		// convert "x, y, z T", to "T, T, T"
		n := max(1, len(f.Names))
		tstr := typeString(f.Type)
		for i := 0; i < n; i++ {
			typeStrings = append(typeStrings, tstr)
		}
	}
	return typeStrings
}

// nodeString returns a string for node.
func nodeString(node ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("<ERROR:%s>", err)
	}
	return buf.String()
}
