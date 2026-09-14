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
		typeParams: fieldListTypes(ft.TypeParams, tpm),
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
		//   - The old struct was comparable, but the new one isn't. This can happen if a slice, map, func
		//     chan, or non-comparable struct field was added.
		//     TODO: consider handling this case (although it's rare).
		//
		// If we are comparing the selectable fields, and they are a superset of
		// the top-level fields, then why bother with the top-level fields? Because
		// the latter is what you use in struct literals. Consider:
		//
		//     // old version
		//     type S struct { A, B int }
		//
		//     // new version
		//     type e struct { A int }
		//     type S struct { e; B int }
		//
		// The top-level fields have changed but the selectable fields haven't. You
		// can write `var s S; s.A; s.B` in both the old and new versions.
		// But this is a breaking change: you used to be able to write
		//     S{A: 1, B: 2}
		// but that no longer compiles.
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
		seen := map[string]bool{} // to avoid repeating a field

		yld := func(name string, kind changeKind) bool {
			if seen[name] {
				return true
			}
			seen[name] = true
			if kind == changeCallCompatible {
				kind = changeBreaking
			}
			return yield(name, kind)
		}

		for name, kind := range old.topLevelFields.changes(news.topLevelFields) {
			if !yld(name, kind) {
				return
			}
		}
		for name, kind := range old.selectableFields.changes(news.selectableFields) {
			if !yld(name, kind) {
				return
			}
		}
	}
}

// namedType is the type of a named type.
type namedType struct {
	typeParams       []string        // type parameters
	underlying       syntaxType      // the underlying type
	methods          *symbolSet      // all methods
	valueMethodNames map[string]bool // names of methods with value receiver
}

// newNamedType constructs a namedType from an ast.TypeSpec.
func newNamedType(ts *ast.TypeSpec, defs *defs) *namedType {
	name := ts.Name.Name
	methods := newSymbolSet()
	valueMethodNames := map[string]bool{}
	for _, m := range defs.methodsFor(name) {
		methods.symbols[m.Name.Name] = newMethodType(m)
		if !isPointerReceiver(m.Recv) {
			valueMethodNames[m.Name.Name] = true
		}
	}
	tpm := typeParamMap(ts.TypeParams)
	return &namedType{
		typeParams:       fieldListTypes(ts.TypeParams, tpm),
		underlying:       newType(substTypeParams(ts.Type, tpm), defs),
		methods:          methods,
		valueMethodNames: valueMethodNames,
	}
}

// newMethodType constructs a funcType for a method declaration, mapping receiver type parameters to #N.
func newMethodType(m *ast.FuncDecl) *funcType {
	ft := m.Type
	var variadic bool
	if params := ft.Params; params != nil && len(params.List) > 0 {
		last := params.List[len(params.List)-1]
		_, variadic = last.Type.(*ast.Ellipsis)
	}
	tpm := receiverTypeParamMap(m.Recv)
	return &funcType{
		typeParams: fieldListTypes(ft.TypeParams, tpm),
		params:     fieldListTypes(ft.Params, tpm),
		results:    fieldListTypes(ft.Results, tpm),
		variadic:   variadic,
	}
}

// receiverTypeParamMap returns a map from receiver type parameter names to their #N representation.
func receiverTypeParamMap(recv *ast.FieldList) map[string]string {
	if len(recv.List) == 0 {
		return nil
	}
	typeExpr := recv.List[0].Type
loop:
	for {
		switch t := typeExpr.(type) {
		case *ast.ParenExpr:
			typeExpr = t.X
		case *ast.StarExpr:
			typeExpr = t.X
		default:
			break loop
		}
	}
	var idents []*ast.Ident
	switch t := typeExpr.(type) {
	case *ast.IndexExpr:
		if id, ok := ast.Unparen(t.Index).(*ast.Ident); ok {
			idents = append(idents, id)
		}
	case *ast.IndexListExpr:
		for _, idx := range t.Indices {
			if id, ok := ast.Unparen(idx).(*ast.Ident); ok {
				idents = append(idents, id)
			}
		}
	}
	if len(idents) == 0 {
		return nil
	}
	m := make(map[string]string, len(idents))
	for i, id := range idents {
		if id.Name != "_" {
			m[id.Name] = fmt.Sprintf("#%d", i)
		}
	}
	return m
}

func isPointerReceiver(recv *ast.FieldList) bool {
	_, ok := ast.Unparen(recv.List[0].Type).(*ast.StarExpr)
	return ok
}

// changes returns all breaking and call-compatible changes between old and new.
// A named type can change if its underlying type changes, or if its methods change.
func (old *namedType) changes(newType syntaxType) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		newn, ok := newType.(*namedType)
		if !ok {
			yield("", changeBreaking)
			return
		}

		if !slices.Equal(old.typeParams, newn.typeParams) {
			if !yield("", changeBreaking) {
				return
			}
		}

		for k, v := range changeUnderlying(old.underlying, newn.underlying) {
			if !yield(k, v) {
				return
			}
		}
		for k, v := range old.methods.changes(newn.methods) {
			if !yield(k, v) {
				return
			}
		}
		// Checking the combined method set handles most cases. We also have to ensure
		// that a value method wasn't changed to a pointer method. It's enough to
		// show that no value method was removed from the value method set. Given
		// that the combined method set check showed no breaking changes, the only
		// other way to remove a value method would be to swap it with a pointer
		// method, and this check will catch that.
		for name := range old.valueMethodNames {
			if !newn.valueMethodNames[name] {
				if !yield(name, changeBreaking) {
					return
				}
			}
		}
	}
}

// changeUnderlying returns the breaking and call-compatible changes between the
// underlying types of two named types.
//
// Any changes to methods on an underlying named type are ignored.
// For example, in
//
//	type T U
//
// The type U might have methods that change, but we don't report them as breaking changes on T.
// We report them as breaking changes on U.
//
// If the underlying type is a function, a call-compatible change becomes a breaking change.
// For example, if someone declares
//
//	type T func(int)
//
// then they presumably intend that variables are declared to be of this type, and one typically
// assigns to variables as well as calls them.
func changeUnderlying(oldType, newType syntaxType) iter.Seq2[string, changeKind] {
	return func(yield func(string, changeKind) bool) {
		_, oldIsNamed := oldType.(*namedType)
		_, newIsNamed := newType.(*namedType)
		if oldIsNamed != newIsNamed {
			yield("", changeBreaking)
			return
		}
		if oldIsNamed { // and new is too
			// ignore changes on underlying named types
			return
		}
		_, isFuncType := oldType.(*funcType)
		for k, kind := range oldType.changes(newType) {
			if kind == changeCallCompatible && isFuncType {
				kind = changeBreaking
			}
			if !yield(k, kind) {
				return
			}
		}
	}
}
