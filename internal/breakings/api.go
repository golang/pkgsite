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
)

// typeString returns a string for the given type expression that represents the type.
// If two such strings are equal, then the corresponding types are equal.
// typeString returns "?" if typeExpr is nil.
func typeString(typeExpr ast.Expr) string {
	if typeExpr == nil {
		return "?"
	}
	return nodeString(typeExpr)
}

// nodeString returns a string for node.
func nodeString(node ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("<ERROR:%s>", err)
	}
	return buf.String()
}
