// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package breakings finds the API of a package: the set of exported
// symbols and their types.
package breakings

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"iter"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// An API contains the exported symbols for a package.
type API struct {
	packageName string
	version     string
	symbols     *symbolSet
	kinds       map[string]token.Token // CONST, VAR, FUNC or TYPE
}

// NewAPI constructs an API from the AST of a package.
func NewAPI(packageName, version string, files []*ast.File) (*API, error) {
	defs, err := newDefs(files)
	if err != nil {
		return nil, err
	}
	syms := newSymbolSet()
	kinds := make(map[string]token.Token)
	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				// Exported functions only.
				if decl.Recv != nil || !decl.Name.IsExported() {
					continue
				}
				syms.symbols[decl.Name.Name] = newFuncType(decl.Type)
				kinds[decl.Name.Name] = token.FUNC
			case *ast.GenDecl:
				switch decl.Tok {
				case token.CONST, token.VAR:
					for _, spec := range decl.Specs {
						spec := spec.(*ast.ValueSpec)
						for _, name := range spec.Names {
							if name.IsExported() {
								syms.symbols[name.Name] = newType(spec.Type, defs)
								kinds[name.Name] = decl.Tok
							}
						}
					}
				case token.TYPE:
					// Top-level named type.
					for _, spec := range decl.Specs {
						spec := spec.(*ast.TypeSpec)
						if spec.Assign.IsValid() {
							return nil, fmt.Errorf("type aliases are not implemented")
						}
						if spec.Name.IsExported() {
							syms.symbols[spec.Name.Name] = newNamedType(spec, defs)
							kinds[spec.Name.Name] = token.TYPE
						}
					}
				}
			}
		}
	}
	return &API{
		packageName: packageName,
		version:     version,
		symbols:     syms,
		kinds:       kinds,
	}, nil
}

// Changes returns an iterator over the breaking and call-compatible changes between
// two APIs (which should be two versions of the same package). The first value of
// each item is the name of the top-level exported symbol, or the dotted name of
// one of its fields or methods.
func (old *API) Changes(newa *API) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		seen := map[string]bool{}

		yld := func(name string, kind changeKind) bool {
			if seen[name] {
				return true
			}
			seen[name] = true
			return yield(name, kind)
		}

		// Check for kind mismatches. With one exception (see below),
		// a change between kinds (var to const, func to type, etc.) is
		// a breaking change.
		for name, oldKind := range old.kinds {
			newKind, ok := newa.kinds[name]
			if !ok || oldKind == newKind {
				continue
			}
			if oldKind == token.FUNC && newKind == token.VAR {
				// It's okay to change a function to a variable of the same type.
				// (If there is a type mismatch, we'll catch it below.)
				// The reverse (variable to function) is breaking because there
				// might be assignments to the variable.
				continue
			}
			if !yld(name, changeBreaking) {
				return
			}
		}

		for name, kind := range old.symbols.changes(newa.symbols) {
			if !yld(name, kind) {
				return
			}
		}
	}
}

// defs holds all the top-level type and method definitions in a package.
type defs struct {
	types   map[string]*ast.TypeSpec   // all top-level types by name
	methods map[string][]*ast.FuncDecl // exported methods by type name
}

func newDefs(files []*ast.File) (*defs, error) {
	defs := &defs{
		types:   map[string]*ast.TypeSpec{},
		methods: map[string][]*ast.FuncDecl{},
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				// We only want exported methods.
				if decl.Recv != nil && decl.Name.IsExported() {
					name, _ := baseTypeName(decl.Recv.List[0].Type)
					if name == "" {
						return nil, fmt.Errorf("bad receiver: %+v", decl.Recv)
					}
					defs.methods[name] = append(defs.methods[name], decl)
				}
			case *ast.GenDecl:
				if decl.Tok != token.TYPE {
					continue
				}
				for _, spec := range decl.Specs {
					spec := spec.(*ast.TypeSpec)
					if spec.Assign.IsValid() {
						return nil, fmt.Errorf("type aliases are not implemented")
					}
					defs.types[spec.Name.Name] = spec
				}
			}
		}
	}
	return defs, nil
}

// typeFor returns the TypeSpec for the type with the given name,
// or nil if there is none.
func (d *defs) typeFor(name string) *ast.TypeSpec {
	if d == nil {
		return nil
	}
	return d.types[name]
}

// methodsFor returns the exported method declarations for the type with the given name,
// or nil if there are none.
func (d *defs) methodsFor(name string) []*ast.FuncDecl {
	if d == nil {
		return nil
	}
	return d.methods[name]
}

// baseTypeName returns the base type name of an expression (e.g. an embedded field or receiver).
// It returns the empty string if there is no base name.
// It also reports whether there was a selector expression.
func baseTypeName(typeExpr ast.Expr) (string, bool) {
	for {
		switch t := typeExpr.(type) {
		case *ast.ParenExpr:
			typeExpr = t.X
		case *ast.StarExpr:
			typeExpr = t.X
		case *ast.IndexExpr:
			typeExpr = t.X
		case *ast.IndexListExpr:
			typeExpr = t.X
		case *ast.SelectorExpr:
			return t.Sel.Name, true
		case *ast.Ident:
			return t.Name, false
		default:
			return "", false
		}
	}
}

// typeString returns a string for the given type expression that represents the type.
// If two such strings are equal, then the corresponding types are equal.
// typeString returns "?" if typeExpr is nil.
func typeString(typeExpr ast.Expr) string {
	switch t := typeExpr.(type) {
	case nil:
		return "?"
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeString(t.Elt)
		}
		return "[" + nodeString(t.Len) + "]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.RECV:
			return "<-chan " + typeString(t.Value)
		case ast.SEND:
			return "chan<- " + typeString(t.Value)
		default:
			s := typeString(t.Value)
			if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
				return "chan " + s
			}
			if strings.HasPrefix(s, "<-chan") {
				return "chan (" + s + ")"
			}
			return "chan " + s
		}
	case *ast.ParenExpr:
		return "(" + typeString(t.X) + ")"
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.IndexExpr:
		return typeString(t.X) + "[" + typeString(t.Index) + "]"
	case *ast.IndexListExpr:
		indices := make([]string, 0, len(t.Indices))
		for _, idx := range t.Indices {
			indices = append(indices, typeString(idx))
		}
		return typeString(t.X) + commaList(indices, "[", "]")
	case *ast.UnaryExpr:
		return t.Op.String() + typeString(t.X)
	case *ast.BinaryExpr:
		return typeString(t.X) + " " + t.Op.String() + " " + typeString(t.Y)
	case *ast.FuncType:
		return "func" + sigString(t)
	case *ast.StructType:
		return structString(t)
	case *ast.InterfaceType:
		return interfaceString(t)
	default:
		return nodeString(t)
	}
}

// interfaceString returns a string representation of an interface type
// with sorted methods and canonical formatting (e.g., "interface{Close() error; Read([]byte) (int, error)}").
// Embedded interfaces should be expanded to their methods, but that would require
// access to other packages; their names are included instead.
func interfaceString(it *ast.InterfaceType) string {
	methods := interfaceMethods(it)
	if len(methods) == 0 {
		return "interface{}"
	}
	slices.Sort(methods)
	methods = slices.Compact(methods)
	return "interface{" + strings.Join(methods, "; ") + "}"
}

func interfaceMethods(it *ast.InterfaceType) []string {
	if it == nil || it.Methods == nil {
		return nil
	}
	var methods []string
	for _, m := range it.Methods.List {
		if len(m.Names) > 0 {
			if ft, ok := m.Type.(*ast.FuncType); ok {
				for _, name := range m.Names {
					methods = append(methods, name.Name+sigString(ft))
				}
			}
			continue
		}

		// Handle embedded types.
		if embedded, ok := m.Type.(*ast.InterfaceType); ok {
			methods = append(methods, interfaceMethods(embedded)...)
		} else {
			methods = append(methods, typeString(m.Type))
		}
	}
	return methods
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
	tpm := typeParamMap(ft.TypeParams)
	typeParamTypes := fieldListTypes(ft.TypeParams, nil)
	paramTypes := fieldListTypes(ft.Params, tpm)
	resTypes := fieldListTypes(ft.Results, tpm)

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

// typeParamMap returns a map from type parameter names to their #N representation.
func typeParamMap(fl *ast.FieldList) map[string]string {
	if fl == nil {
		return nil
	}
	m := make(map[string]string)
	i := 0
	for _, f := range fl.List {
		for _, name := range f.Names {
			if name.Name != "_" {
				m[name.Name] = fmt.Sprintf("#%d", i)
			}
			i++
		}
	}
	return m
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
