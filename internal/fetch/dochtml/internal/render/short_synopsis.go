// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// shortOneLineNodeDepth returns a heavily-truncated, one-line summary of the
// given node. It will only accept an *ast.FuncDecl when not called recursively.
// The depth specifies the current depth when traversing the AST and the
// function will stop traversing once it reaches maxSynopsisNodeDepth.
func shortOneLineNodeDepth(fset *token.FileSet, node ast.Node, depth int) (string, error) {
	if _, ok := node.(*ast.FuncDecl); !ok && depth == 0 {
		return "", fmt.Errorf("only *ast.FuncDecl nodes are supported at top level")
	}

	const dotDotDot = "..."
	if depth == maxSynopsisNodeDepth {
		return dotDotDot, nil
	}
	depth++

	switch n := node.(type) {
	case *ast.FuncDecl:
		// Formats func declarations.
		name := n.Name.Name
		recv, err := shortOneLineNodeDepth(fset, n.Recv, depth)
		if err != nil {
			return "", err
		}
		if len(recv) > 0 {
			recv = "(" + recv + ") "
		}
		fnc, err := shortOneLineNodeDepth(fset, n.Type, depth)
		if err != nil {
			return "", err
		}
		fnc = strings.TrimPrefix(fnc, "func")
		return recv + name + fnc, nil

	case *ast.FuncType:
		var params []string
		if n.Params != nil {
			for _, field := range n.Params.List {
				f, err := shortOneLineField(fset, field, depth)
				if err != nil {
					return "", err
				}
				params = append(params, f)
			}
		}
		return fmt.Sprintf("func(%s)", joinStrings(params)), nil

	case *ast.FieldList:
		if n == nil || len(n.List) == 0 {
			return "", nil
		}
		if len(n.List) == 1 {
			return shortOneLineField(fset, n.List[0], depth)
		}
		return dotDotDot, nil

	default:
		return "", nil
	}
}

// shortOneLineField returns a heavily-truncated, one-line summary of the field.
// Notably, it omits the field types in its result.
func shortOneLineField(fset *token.FileSet, field *ast.Field, depth int) (string, error) {
	if len(field.Names) == 0 {
		return shortOneLineNodeDepth(fset, field.Type, depth)
	}
	var names []string
	for _, name := range field.Names {
		names = append(names, name.Name)
	}
	return joinStrings(names), nil
}
