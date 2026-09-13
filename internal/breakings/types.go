// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains syntax-only representations of types.

package breakings

import (
	"cmp"
	"fmt"
	"go/ast"
	"iter"
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
	// changes returns an iterator over all breaking and call-compatible changes
	// between the receiver, which is the older version, and the argument. Each change's
	// first value is a struct field or interface method. For other types, there
	// is only one result, with first value "".
	changes(newType syntaxType) iter.Seq2[string, changeKind]
}

// newType constructs a syntaxType from an ast.Expr.
func newType(e ast.Expr, defs *defs) syntaxType {
	switch e := ast.Unparen(e).(type) {
	case *ast.FuncType:
		return newFuncType(e)
	case *ast.InterfaceType:
		return newInterfaceType(e, defs)
	case *ast.StructType:
		return newStructType(e, defs)
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

// changes returns all breaking and call-compatible changes between old and new.
func (old *simpleType) changes(newType syntaxType) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		if n, ok := newType.(*simpleType); ok && old.typeString == n.typeString {
			return
		}
		yield("", changeBreaking)
	}
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

// changes returns all breaking and call-compatible changes between old and new.
func (old *funcType) changes(newType syntaxType) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		newf, ok := newType.(*funcType)
		if !ok {
			yield("", changeBreaking)
			return
		}
		if old.equal(newf) {
			return
		}
		// There are many kinds of call-compatible changes, but just look for
		// adding a variadic argument. That's the most common.
		if !old.variadic && newf.variadic &&
			slices.Equal(old.typeParams, newf.typeParams) &&
			slices.Equal(old.results, newf.results) &&
			len(newf.params) == len(old.params)+1 &&
			slices.Equal(old.params, newf.params[:len(old.params)]) {
			yield("", changeCallCompatible)
			return
		}
		// Any other difference in a function signature is a breaking change.
		yield("", changeBreaking)
	}
}

// symbolSet is a set of symbols and their types.
type symbolSet struct {
	symbols map[string]syntaxType
}

func newSymbolSet() *symbolSet {
	return &symbolSet{symbols: make(map[string]syntaxType)}
}

// changes returns an iterator of breaking changes from old to new.
// It assumes that all the symbols in both sets are exported.
func (old *symbolSet) changes(newSet *symbolSet) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		for name, oldType := range old.symbols {
			newType, ok := newSet.symbols[name]
			if !ok {
				// It's a breaking change to remove a symbol.
				if !yield(name, changeBreaking) {
					return
				}
			} else {
				for name2, c := range oldType.changes(newType) {
					n := name
					if name2 != "" {
						n += "." + name2
					}
					if !yield(n, c) {
						return
					}
				}
			}
		}
	}
}

// interfaceType is the type of an interface.
type interfaceType struct {
	methods          *symbolSet // exported methods only
	unexportedMethod string     // any unexported method name, or "" if none
}

// newInterfaceType constructs an interfaceType from an ast.InterfaceType.
func newInterfaceType(it *ast.InterfaceType, defs *defs) *interfaceType {
	res := &interfaceType{methods: newSymbolSet()}
	if it.Methods != nil {
		for _, m := range it.Methods.List {
			if len(m.Names) == 0 {
				// TODO: handle embedded interfaces.
				continue
			}
			// There is only one name
			name := m.Names[0]
			if name.IsExported() {
				res.methods.symbols[name.Name] = newType(m.Type, defs)
			} else {
				res.unexportedMethod = name.Name
			}
		}
	}
	return res
}

// changes returns all breaking and call-compatible changes between old and new.
func (old *interfaceType) changes(newType syntaxType) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		newi, ok := newType.(*interfaceType)
		if !ok {
			yield("", changeBreaking)
			return
		}
		for nm, kind := range old.methods.changes(newi.methods) {
			if old.unexportedMethod == "" && kind == changeCallCompatible {
				// If the interface doesn't have an unexported method, then other packages
				// can implement its methods, not just call them. Thus even if a method signature
				// changes call-compatibly, that's still a breaking change.
				kind = changeBreaking
			}
			if !yield(nm, kind) {
				return
			}
		}
		if old.unexportedMethod == "" {
			// Adding any method, exported or not, to an interface without an unexported
			// method is a breaking change.
			if newi.unexportedMethod != "" {
				if !yield(newi.unexportedMethod, changeBreaking) {
					return
				}
			}
			for nm := range newi.methods.symbols {
				if _, ok := old.methods.symbols[nm]; !ok {
					if !yield(nm, changeBreaking) {
						return
					}
				}
			}
		}
	}
}

// structType is the type of a struct.
type structType struct {
	topLevelFields   *symbolSet // top-level exported fields
	selectableFields *symbolSet // fields that can be selected, at any depth
}

// newStructType constructs a structType from an ast.StructType.
func newStructType(st *ast.StructType, defs *defs) *structType {
	topFields := newSymbolSet()
	selFields := newSymbolSet()

	// Get the selectable exported fields.
	// This algorithm is the clearest one I can think of, but not the most efficient.
	// That's OK: structs rarely have lots of embedding.
	//
	// Get all the exported fields.
	fields := appendExportedFields(st, defs, 0, nil, map[*ast.StructType]bool{st: true})
	// Sort by depth and name.
	slices.SortFunc(fields, func(f1, f2 *field) int {
		if f1.depth != f2.depth {
			return cmp.Compare(f1.depth, f2.depth)
		}
		return cmp.Compare(f1.name, f2.name)
	})
	// Keep each field if it is unique at its highest level.
	// We can't use slices.CompactFunc, because a duplicate at level N invalidates
	// the field for level N+1 and higher, even if those occurrences are unique.
	dups := map[string]bool{}
	for i, f := range fields {
		if _, ok := selFields.symbols[f.name]; ok {
			continue
		}
		if dups[f.name] {
			continue
		}
		if i+1 < len(fields) && fields[i+1].depth == f.depth && fields[i+1].name == f.name {
			dups[f.name] = true
			continue
		}
		// We haven't seen this name before and it's not a duplicate, so it's a selectable field.
		stype := newType(f.typeExpr, defs)
		selFields.symbols[f.name] = stype
		if f.depth == 0 {
			topFields.symbols[f.name] = stype
		}
	}
	return &structType{topLevelFields: topFields, selectableFields: selFields}
}

type field struct {
	name     string
	typeExpr ast.Expr
	depth    int
}

// appendExportedFields appends all the exported fields of st to fields.
// It descends into embedded structs.
func appendExportedFields(st *ast.StructType, defs *defs, depth int, fields []*field, seen map[*ast.StructType]bool) []*field {
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 { // embedded field
			name, hasSel := baseTypeName(f.Type)
			if ast.IsExported(name) {
				fields = append(fields, &field{name, f.Type, depth})
			}
			// Exported or not, an embedded struct's exported fields may be visible.
			if ts := defs.typeFor(name); ts == nil {
				// We don't know this type: another package or a file we were't given.
				// This can result in wrong answers: selectable fields that we miss,
				// or ones we include but that would have been cancelled out by same-named
				// fields at the same depth. We'll live with all that.
			} else if embeddedSt, ok := ts.Type.(*ast.StructType); ok && !hasSel {
				// A struct type from this package. We know it's from this package because there was no selector modifying the base name: we didn't see something like
				//    struct { pkg.T ... }
				if !seen[embeddedSt] {
					seen[embeddedSt] = true
					fields = appendExportedFields(embeddedSt, defs, depth+1, fields, seen)
					delete(seen, embeddedSt)
				}
			}
			// An embedded non-struct. Ignore it.
		} else {
			for _, name := range f.Names {
				if name.IsExported() {
					fields = append(fields, &field{name.Name, f.Type, depth})
				}
			}
		}
	}
	return fields
}

// changes returns all breaking and call-compatible changes between old and new.
func (old *structType) changes(newType syntaxType) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		// A struct has a breaking change if one of three things occurs:
		//   - One of its top-level fields has a breaking change. That includes anonymous
		//     (embedded) fields.
		//   - One of its selectable fields has a breaking change. A field F is selectable if you can
		//     write S.F. That includes the top-level fields, but also the fields of an embedded struct
		//     that aren't hidden by a field at a higher depth.
		//     TODO: handle this case as best we can (we only know about types declared in this package).
		//   - The old struct was comparable, but the new one isn't. This can happen if a slice, map, func
		//     chan, or non-comparable struct field was added.
		//     TODO: consider handling this case (although it's rare).
		news, ok := newType.(*structType)
		if !ok {
			yield("", changeBreaking)
			return
		}
		// A struct type changes if one of its fields changes.
		// The order of the fields doesn't matter. We're not comparing two struct types
		// for identity, we're comparing a struct type across two versions of a package.
		// The two different types can't exist in the same program at the same time,
		// so there is no way to compare them.
		// Any breaking change in the fields is a breaking change for the entire struct.
		// A call-compatible change could happen if a field has function type, and that function
		// type was changed call-compatibly. But that is really a breaking change, because users
		// are likely to assign to the field as well as call it.
		for nm, kind := range old.topLevelFields.changes(news.topLevelFields) {
			if kind == changeCallCompatible {
				kind = changeBreaking
			}
			if !yield(nm, kind) {
				return
			}
		}
	}
}
