// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"
)

const dotDotDot = "..."

// OneLineNodeDepth returns a one-line summary of the given input node.
// The depth specifies the current depth when traversing the AST and the
// function will stop traversing once depth reaches maxSynopsisNodeDepth.
func OneLineNodeDepth(fset *token.FileSet, node ast.Node, depth int) string {
	if depth == maxSynopsisNodeDepth {
		return dotDotDot
	}
	depth++

	switch n := node.(type) {
	case nil:
		return ""

	case *ast.GenDecl:
		trailer := ""
		if len(n.Specs) > 1 {
			trailer = " " + dotDotDot
		}

		switch n.Tok {
		case token.CONST, token.VAR:
			for i, spec := range n.Specs {
				valueSpec := spec.(*ast.ValueSpec) // must succeed; we can't mix types in one GenDecl.
				return ConstOrVarSynopsis(valueSpec, fset, n.Tok, trailer, i, depth)
			}
		case token.TYPE:
			if len(n.Specs) > 0 {
				return OneLineNodeDepth(fset, n.Specs[0], depth) + trailer
			}
		case token.IMPORT:
			if len(n.Specs) > 0 {
				pkg := n.Specs[0].(*ast.ImportSpec).Path.Value
				return fmt.Sprintf("%s %s%s", n.Tok, pkg, trailer)
			}
		}
		return fmt.Sprintf("%s ()", n.Tok)

	case *ast.FuncDecl:
		// Formats func declarations.
		name := n.Name.Name
		recv := OneLineNodeDepth(fset, n.Recv, depth)
		if len(recv) > 0 {
			recv = "(" + recv + ") "
		}
		fnc := OneLineNodeDepth(fset, n.Type, depth)
		if strings.Index(fnc, "func") == 0 {
			fnc = fnc[4:]
		}
		return fmt.Sprintf("func %s%s%s", recv, name, fnc)

	case *ast.TypeSpec:
		sep := " "
		if n.Assign.IsValid() {
			sep = " = "
		}
		return fmt.Sprintf("type %s%s%s", n.Name.Name, sep, OneLineNodeDepth(fset, n.Type, depth))

	case *ast.FuncType:
		var params []string
		if n.Params != nil {
			for _, field := range n.Params.List {
				params = append(params, OneLineField(fset, field, depth))
			}
		}
		needParens := false
		var results []string
		if n.Results != nil {
			needParens = needParens || len(n.Results.List) > 1
			for _, field := range n.Results.List {
				needParens = needParens || len(field.Names) > 0
				results = append(results, OneLineField(fset, field, depth))
			}
		}

		tparam := formatTypeParams(fset, n.TypeParams, depth)

		param := joinStrings(params)
		if len(results) == 0 {
			return fmt.Sprintf("func%s(%s)", tparam, param)
		}
		result := joinStrings(results)
		if !needParens {
			return fmt.Sprintf("func%s(%s) %s", tparam, param, result)
		}
		return fmt.Sprintf("func%s(%s) (%s)", tparam, param, result)

	case *ast.StructType:
		if n.Fields == nil || len(n.Fields.List) == 0 {
			return "struct{}"
		}
		return "struct{ ... }"

	case *ast.InterfaceType:
		if n.Methods == nil || len(n.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{ ... }"

	case *ast.FieldList:
		if n == nil || len(n.List) == 0 {
			return ""
		}
		if len(n.List) == 1 {
			return OneLineField(fset, n.List[0], depth)
		}
		return dotDotDot

	case *ast.FuncLit:
		return OneLineNodeDepth(fset, n.Type, depth) + " { ... }"

	case *ast.CompositeLit:
		typ := OneLineNodeDepth(fset, n.Type, depth)
		if len(n.Elts) == 0 {
			return fmt.Sprintf("%s{}", typ)
		}
		return fmt.Sprintf("%s{ %s }", typ, dotDotDot)

	case *ast.ArrayType:
		length := OneLineNodeDepth(fset, n.Len, depth)
		element := OneLineNodeDepth(fset, n.Elt, depth)
		return fmt.Sprintf("[%s]%s", length, element)

	case *ast.MapType:
		key := OneLineNodeDepth(fset, n.Key, depth)
		value := OneLineNodeDepth(fset, n.Value, depth)
		return fmt.Sprintf("map[%s]%s", key, value)

	case *ast.CallExpr:
		fnc := OneLineNodeDepth(fset, n.Fun, depth)
		var args []string
		for _, arg := range n.Args {
			args = append(args, OneLineNodeDepth(fset, arg, depth))
		}
		return fmt.Sprintf("%s(%s)", fnc, joinStrings(args))

	case *ast.UnaryExpr:
		return fmt.Sprintf("%s%s", n.Op, OneLineNodeDepth(fset, n.X, depth))

	case *ast.Ident:
		return n.Name

	default:
		// As a fallback, use default formatter for all unknown node types.
		buf := new(bytes.Buffer)
		format.Node(buf, fset, node)
		s := buf.String()
		if strings.Contains(s, "\n") {
			return dotDotDot
		}
		return s
	}
}

func ConstOrVarSynopsis(valueSpec *ast.ValueSpec, fset *token.FileSet, tok token.Token,
	trailer string, i, depth int) string {
	if len(valueSpec.Names) > 1 || len(valueSpec.Values) > 1 {
		trailer = " " + dotDotDot
	}

	// The type name may carry over from a previous specification in the
	// case of constants and iota.
	typ := ""
	if valueSpec.Type != nil {
		typ = fmt.Sprintf(" %s", OneLineNodeDepth(fset, valueSpec.Type, depth))
	} else if len(valueSpec.Values) > 0 {
		typ = ""
	}

	val := ""
	if i < len(valueSpec.Values) && valueSpec.Values[i] != nil {
		val = fmt.Sprintf(" = %s", OneLineNodeDepth(fset, valueSpec.Values[i], depth))
	}
	return fmt.Sprintf("%s %s%s%s%s", tok, valueSpec.Names[0], typ, val, trailer)
}

// OneLineField returns a one-line summary of the field.
func OneLineField(fset *token.FileSet, field *ast.Field, depth int) string {
	var names []string
	for _, name := range field.Names {
		names = append(names, name.Name)
	}
	if len(names) == 0 {
		return OneLineNodeDepth(fset, field.Type, depth)
	}
	return joinStrings(names) + " " + OneLineNodeDepth(fset, field.Type, depth)
}

// joinStrings formats the input as a comma-separated list,
// but truncates the list at some reasonable length if necessary.
func joinStrings(ss []string) string {
	const widthLimit = 80
	var n int
	for i, s := range ss {
		n += len(s) + len(", ")
		if n > widthLimit {
			ss = append(ss[:i:i], "...")
			break
		}
	}
	return strings.Join(ss, ", ")
}

func formatTypeParams(fset *token.FileSet, list *ast.FieldList, depth int) string {
	if list.NumFields() == 0 {
		return ""
	}
	var tparams []string
	for _, field := range list.List {
		tparams = append(tparams, OneLineField(fset, field, depth))
	}
	return "[" + joinStrings(tparams) + "]"
}
