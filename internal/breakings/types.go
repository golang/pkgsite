// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains syntax-only representations of types.

package api

import (
	"fmt"
	"go/ast"
	"slices"
)

// changeKind is the kind of change between two API elements.
type changeKind int

const (
	changeOther          changeKind = iota // no change or compatible change
	changeCallCompatible                   // technically a breaking change, but calls will compile
	changeBreaking
)

func (c changeKind) String() string {
	switch c {
	case changeOther:
		return "other"
	case changeCallCompatible:
		return "call-compatible"
	case changeBreaking:
		return "breaking"
	default:
		return fmt.Sprintf("changeKind(%d)", c)
	}
}

// syntaxType is a Go type extracted from syntax.
type syntaxType interface {
	// change reports the most significant change between the receiver,
	// which is the older version, and the argument. A breaking change
	// is considered more significant than a call-compatible change, which
	// is more significant than an "other" change.
	change(newType syntaxType) changeKind
}

// newType constructs a syntaxType from an ast.Expr.
func newType(e ast.Expr) syntaxType {
	switch e := ast.Unparen(e).(type) {
	case *ast.FuncType:
		return newFuncType(e)
	case *ast.InterfaceType:
		return newInterfaceType(e)
	default:
		return newSimpleType(e)
	}
}

// simpleType is a type represented only by a type string.
// The key property of a simpleType is that any change is a breaking
// one. It is used not only for basic types like int and string, but
// also for slices, arrays, maps, and channels. (Technically you can
// drop a direction on a channel compatibly, but that's rare enough so
// that we don't bother with it.)
type simpleType struct {
	typeString string
}

func newSimpleType(e ast.Expr) *simpleType {
	return &simpleType{typeString: typeString(e)}
}

// change returns the kind of change from old to new.
func (old *simpleType) change(newType syntaxType) changeKind {
	if n, ok := newType.(*simpleType); ok && old.typeString == n.typeString {
		return changeOther
	}
	return changeBreaking
}

// funcType is the type of a function.
type funcType struct {
	typeParams []string
	params     []string
	results    []string
	variadic   bool // shorthand for the last param having a "..."
}

// newFuncType constructs a funcType from an ast.FuncType.
func newFuncType(ft *ast.FuncType) *funcType {
	var variadic bool
	if params := ft.Params; params != nil && len(params.List) > 0 {
		last := params.List[len(params.List)-1]
		_, variadic = last.Type.(*ast.Ellipsis)
	}
	tpm := typeParamMap(ft.TypeParams)
	return &funcType{
		typeParams: fieldListTypes(ft.TypeParams, nil),
		params:     fieldListTypes(ft.Params, tpm),
		results:    fieldListTypes(ft.Results, tpm),
		variadic:   variadic,
	}
}

// equal reports whether f and other have equal typeParams, params and results.
func (f *funcType) equal(other syntaxType) bool {
	o, ok := other.(*funcType)
	if !ok {
		return false
	}
	// We don't need to check variadic here. That is just a convenience for the change method.
	// All the param information is in params.
	return slices.Equal(f.typeParams, o.typeParams) && slices.Equal(f.params, o.params) && slices.Equal(f.results, o.results)
}

// change returns the kind of change from old to new.
func (old *funcType) change(newType syntaxType) changeKind {
	newf, ok := newType.(*funcType)
	if !ok {
		return changeBreaking
	}
	if old.equal(newf) {
		return changeOther
	}
	// There are many kinds of call-compatible changes, but just look for
	// adding a variadic argument. That's the most common.
	if !old.variadic && newf.variadic &&
		slices.Equal(old.typeParams, newf.typeParams) &&
		slices.Equal(old.results, newf.results) &&
		len(newf.params) == len(old.params)+1 &&
		slices.Equal(old.params, newf.params[:len(old.params)]) {
		return changeCallCompatible
	}
	// Any other difference in a function signature is a breaking change.
	return changeBreaking
}

// symbolSet is a set of symbols and their types.
type symbolSet struct {
	symbols map[string]syntaxType
	// name of enclosing package, interface or struct
	parentName string
}

// changes returns the map of breaking changes from old to new.
// It assumes that all the symbols in both sets are exported.
func (old *symbolSet) changes(newSet *symbolSet) map[string]changeKind {
	res := make(map[string]changeKind)
	for name, oldType := range old.symbols {
		newType, ok := newSet.symbols[name]
		if !ok {
			// It's a breaking change to remove a symbol.
			res[old.parentName+"."+name] = changeBreaking
		} else if c := oldType.change(newType); c != changeOther {
			res[old.parentName+"."+name] = c
		}
	}
	return res
}

// interfaceType is the type of an interface.
type interfaceType struct {
	methods          *symbolSet // exported methods only
	unexportedMethod string     // any unexported method name, or "" if none
}

// newInterfaceType constructs an interfaceType from an ast.InterfaceType.
func newInterfaceType(it *ast.InterfaceType) *interfaceType {
	res := &interfaceType{methods: &symbolSet{symbols: map[string]syntaxType{}}}
	if it.Methods != nil {
		for _, m := range it.Methods.List {
			if len(m.Names) == 0 {
				// TODO: handle embedded interfaces.
				continue
			}
			// There is only one name
			name := m.Names[0]
			if name.IsExported() {
				res.methods.symbols[name.Name] = newType(m.Type)
			} else {
				res.unexportedMethod = name.Name
			}
		}
	}
	return res
}

// change returns the kind of change from old to new.
func (old *interfaceType) change(newType syntaxType) changeKind {
	newi, ok := newType.(*interfaceType)
	if !ok {
		return changeBreaking
	}
	changes := old.methods.changes(newi.methods)
	res := changeOther
	for _, kind := range changes {
		if kind == changeBreaking {
			return changeBreaking
		}
		if kind == changeCallCompatible {
			// If the interface doesn't have an unexported method, then other packages
			// can implement its methods, not just call them. Thus even if a method signature
			// changes call-compatibly, that's still a breaking change.
			if old.unexportedMethod == "" {
				return changeBreaking
			}
			res = changeCallCompatible
		}
	}
	if old.unexportedMethod == "" {
		// Adding any method, exported or not, to an interface without an unexported
		// method is a breaking change.
		if newi.unexportedMethod != "" {
			return changeBreaking
		}
		for nm := range newi.methods.symbols {
			if _, ok := old.methods.symbols[nm]; !ok {
				return changeBreaking
			}
		}
	}
	return res
}
