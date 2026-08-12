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
	"reflect"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
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
	case *ast.StructType:
		return structString(t)
	default:
		return nodeString(t)
	}
}

// structString returns a string representation of a struct type
// with expanded field lists and canonical formatting (e.g., "struct{X T; Y T}").
func structString(st *ast.StructType) string {
	if st == nil || st.Fields == nil || len(st.Fields.List) == 0 {
		return "struct{}"
	}
	var fields []string
	for _, f := range st.Fields.List {
		tstr := typeString(f.Type)
		tag := ""
		if f.Tag != nil {
			tag = " " + f.Tag.Value
		}
		if len(f.Names) == 0 {
			fields = append(fields, tstr+tag)
		} else {
			for _, name := range f.Names {
				fields = append(fields, name.Name+" "+tstr+tag)
			}
		}
	}
	return "struct{" + strings.Join(fields, "; ") + "}"
}

// sigString returns a string representation of a function signature
// without parameter or return names (e.g., "(int, int) bool").
func sigString(ft *ast.FuncType) string {
	if ft == nil {
		return ""
	}
	var typeParamMap map[string]string
	if ft.TypeParams != nil {
		typeParamMap = make(map[string]string)
		i := 0
		for _, f := range ft.TypeParams.List {
			for _, name := range f.Names {
				typeParamMap[name.Name] = fmt.Sprintf("#%d", i)
				i++
			}
		}
	}

	typeParamTypes := fieldListTypes(ft.TypeParams, nil)
	paramTypes := fieldListTypes(ft.Params, typeParamMap)
	resTypes := fieldListTypes(ft.Results, typeParamMap)

	var buf strings.Builder

	if len(typeParamTypes) > 0 {
		buf.WriteString(commaList(typeParamTypes, "[", "]"))
	}

	buf.WriteString(commaList(paramTypes, "(", ")"))

	if len(resTypes) == 1 {
		buf.WriteString(" ")
		buf.WriteString(resTypes[0])
	} else if len(resTypes) > 1 {
		buf.WriteString(" ")
		buf.WriteString(commaList(resTypes, "(", ")"))
	}
	return buf.String()
}

func commaList(parts []string, left, right string) string {
	return left + strings.Join(parts, ", ") + right
}

// fieldListTypes returns the types of a field list, ignoring the field names.
// Note that "field" here means more than just a struct field: it could be
// the arguments or return values of a function.
func fieldListTypes(fl *ast.FieldList, typeParams map[string]string) []string {
	if fl == nil {
		return nil
	}
	var typeStrings []string
	for _, f := range fl.List {
		// convert "x, y, z T", to "T, T, T"
		n := max(1, len(f.Names))
		t := substTypeParams(f.Type, typeParams)
		tstr := typeString(t)
		for range n {
			typeStrings = append(typeStrings, tstr)
		}
	}
	return typeStrings
}

// substTypeParams returns a copy of expr with type parameter names replaced by their #N representation.
func substTypeParams(expr ast.Expr, typeParams map[string]string) ast.Expr {
	if expr == nil || len(typeParams) == 0 {
		return expr
	}
	expr = cloneNode(expr)
	return astutil.Apply(expr, func(c *astutil.Cursor) bool {
		if _, ok := c.Node().(*ast.SelectorExpr); ok {
			return false
		}
		if _, ok := c.Parent().(*ast.Field); ok && c.Name() == "Names" {
			return false
		}
		if id, ok := c.Node().(*ast.Ident); ok {
			if subst, ok := typeParams[id.Name]; ok {
				c.Replace(&ast.Ident{NamePos: id.NamePos, Name: subst})
			}
		}
		return true
	}, nil).(ast.Expr)
}

// nodeString returns a string for node.
func nodeString(node ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("<ERROR:%s>", err)
	}
	return buf.String()
}

// cloneNode returns a deep copy of a Node.
// It omits pointers to ast.{Scope,Object} variables.
// Copied from golang.org/x/tools/internal/astutil.CloneNode.
func cloneNode[T ast.Node](n T) T {
	var clone func(x reflect.Value) reflect.Value
	set := func(dst, src reflect.Value) {
		src = clone(src)
		if src.IsValid() {
			dst.Set(src)
		}
	}
	clone = func(x reflect.Value) reflect.Value {
		switch x.Kind() {
		case reflect.Pointer:
			if x.IsNil() {
				return x
			}
			// Skip fields of types potentially involved in cycles.
			switch x.Interface().(type) {
			//lint:ignore SA1019 ast.Object and ast.Scope are deprecated
			case *ast.Object, *ast.Scope:
				return reflect.Zero(x.Type())
			}
			y := reflect.New(x.Type().Elem())
			set(y.Elem(), x.Elem())
			return y

		case reflect.Struct:
			y := reflect.New(x.Type()).Elem()
			for i := 0; i < x.Type().NumField(); i++ {
				set(y.Field(i), x.Field(i))
			}
			return y

		case reflect.Slice:
			if x.IsNil() {
				return x
			}
			y := reflect.MakeSlice(x.Type(), x.Len(), x.Cap())
			for i := 0; i < x.Len(); i++ {
				set(y.Index(i), x.Index(i))
			}
			return y

		case reflect.Interface:
			y := reflect.New(x.Type()).Elem()
			set(y, x.Elem())
			return y

		case reflect.Array, reflect.Chan, reflect.Func, reflect.Map, reflect.UnsafePointer:
			panic(x) // unreachable in AST

		default:
			return x // bool, string, number
		}
	}
	return clone(reflect.ValueOf(n)).Interface().(T)
}
