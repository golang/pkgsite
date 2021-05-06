// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/dochtml/internal/render"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
)

// GetSymbols renders package documentation HTML for the
// provided file set and package, in separate parts.
//
// If any of the rendered documentation part HTML sizes exceeds the specified
// limit, an error with ErrTooLarge in its chain will be returned.
func GetSymbols(p *doc.Package, fset *token.FileSet) (_ []*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "GetSymbols for %q", p.ImportPath)
	if docIsEmpty(p) {
		return nil, nil
	}
	typs, err := types(p, fset)
	if err != nil {
		return nil, err
	}
	vars, err := variables(p.Vars, fset)
	if err != nil {
		return nil, err
	}
	return append(append(append(
		constants(p.Consts), vars...), functions(p, fset)...), typs...), nil
}

func constants(consts []*doc.Value) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, c := range consts {
		for _, n := range c.Names {
			if n == "_" {
				continue
			}
			syms = append(syms, &internal.Symbol{
				SymbolMeta: internal.SymbolMeta{
					Name:     n,
					Synopsis: "const " + n,
					Section:  internal.SymbolSectionConstants,
					Kind:     internal.SymbolKindConstant,
				},
			})
		}
	}
	return syms
}

func variables(vars []*doc.Value, fset *token.FileSet) (_ []*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "variables")
	var syms []*internal.Symbol
	for _, v := range vars {
		specs := v.Decl.Specs
		for _, spec := range specs {
			valueSpec := spec.(*ast.ValueSpec) // must succeed; we can't mix types in one GenDecl.
			for _, ident := range valueSpec.Names {
				if ident.Name == "_" {
					continue
				}
				vs := *valueSpec
				if len(valueSpec.Names) != 0 {
					vs.Names = []*ast.Ident{ident}
				}
				syn := render.ConstOrVarSynopsis(&vs, fset, token.VAR, "", 0, 0)
				syms = append(syms,
					&internal.Symbol{
						SymbolMeta: internal.SymbolMeta{
							Name:     ident.Name,
							Synopsis: syn,
							Section:  internal.SymbolSectionVariables,
							Kind:     internal.SymbolKindVariable,
						},
					})
			}

		}
	}
	return syms, nil
}

func functions(p *doc.Package, fset *token.FileSet) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, f := range p.Funcs {
		syms = append(syms, &internal.Symbol{
			SymbolMeta: internal.SymbolMeta{
				Name:     f.Name,
				Synopsis: render.OneLineNodeDepth(fset, f.Decl, 0),
				Section:  internal.SymbolSectionFunctions,
				Kind:     internal.SymbolKindFunction,
			},
		})
	}
	return syms
}

func types(p *doc.Package, fset *token.FileSet) ([]*internal.Symbol, error) {
	var syms []*internal.Symbol
	for _, typ := range p.Types {
		specs := typ.Decl.Specs
		if len(specs) != 1 {
			return nil, fmt.Errorf("unexpected number of t.Decl.Specs: %d (wanted len = 1)", len(typ.Decl.Specs))
		}
		spec, ok := specs[0].(*ast.TypeSpec)
		if !ok {
			return nil, fmt.Errorf("unexpected type for Spec node: %q", typ.Name)
		}
		mthds, err := methodsForType(typ, spec, fset)
		if err != nil {
			return nil, err
		}
		t := &internal.Symbol{
			SymbolMeta: internal.SymbolMeta{
				Name:     typ.Name,
				Synopsis: strings.TrimSuffix(strings.TrimSuffix(render.OneLineNodeDepth(fset, spec, 0), "{ ... }"), "{}"),
				Section:  internal.SymbolSectionTypes,
				Kind:     internal.SymbolKindType,
			},
		}
		fields := fieldsForType(typ.Name, spec, fset)
		if err != nil {
			return nil, err
		}
		syms = append(syms, t)
		vars, err := variablesForType(typ, fset)
		if err != nil {
			return nil, err
		}
		t.Children = append(append(append(append(append(
			t.Children,
			constantsForType(typ)...),
			vars...),
			functionsForType(typ, fset)...),
			fields...),
			mthds...)
	}
	return syms, nil
}

func constantsForType(t *doc.Type) []*internal.SymbolMeta {
	consts := constants(t.Consts)
	var typConsts []*internal.SymbolMeta
	for _, c := range consts {
		c2 := c.SymbolMeta
		c2.ParentName = t.Name
		c2.Section = internal.SymbolSectionTypes
		typConsts = append(typConsts, &c2)
	}
	return typConsts
}

func variablesForType(t *doc.Type, fset *token.FileSet) (_ []*internal.SymbolMeta, err error) {
	vars, err := variables(t.Vars, fset)
	if err != nil {
		return nil, err
	}
	var typVars []*internal.SymbolMeta
	for _, v := range vars {
		v2 := v.SymbolMeta
		v2.ParentName = t.Name
		v2.Section = internal.SymbolSectionTypes
		typVars = append(typVars, &v2)
	}
	return typVars, nil
}

func functionsForType(t *doc.Type, fset *token.FileSet) []*internal.SymbolMeta {
	var syms []*internal.SymbolMeta
	for _, f := range t.Funcs {
		syms = append(syms, &internal.SymbolMeta{
			Name:       f.Name,
			ParentName: t.Name,
			Kind:       internal.SymbolKindFunction,
			Synopsis:   render.OneLineNodeDepth(fset, f.Decl, 0),
			Section:    internal.SymbolSectionTypes,
		})
	}
	return syms
}

func fieldsForType(typName string, spec *ast.TypeSpec, fset *token.FileSet) []*internal.SymbolMeta {
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	var syms []*internal.SymbolMeta
	for _, f := range st.Fields.List {
		// It's not possible for there to be more than one name.
		// FieldList is also used by go/ast for st.Methods, which is the
		// only reason this type is a list.
		for _, n := range f.Names {
			synopsis := fmt.Sprintf("%s %s", n, render.OneLineNodeDepth(fset, f.Type, 0))
			name := typName + "." + n.Name
			syms = append(syms, &internal.SymbolMeta{
				Name:       name,
				ParentName: typName,
				Kind:       internal.SymbolKindField,
				Synopsis:   synopsis,
				Section:    internal.SymbolSectionTypes,
			})
		}
	}
	return syms
}

func methodsForType(t *doc.Type, spec *ast.TypeSpec, fset *token.FileSet) ([]*internal.SymbolMeta, error) {
	var syms []*internal.SymbolMeta
	for _, m := range t.Methods {
		syms = append(syms, &internal.SymbolMeta{
			Name:       t.Name + "." + m.Name,
			ParentName: t.Name,
			Kind:       internal.SymbolKindMethod,
			Synopsis:   render.OneLineNodeDepth(fset, m.Decl, 0),
			Section:    internal.SymbolSectionTypes,
		})
	}
	if st, ok := spec.Type.(*ast.InterfaceType); ok {
		for _, m := range st.Methods.List {
			// It's not possible for there to be more than one name.
			// FieldList is also used by go/ast for st.Methods, which is the
			// only reason this type is a list.
			if len(m.Names) > 1 {
				return nil, fmt.Errorf("len(m.Names) = %d; expected 0 or 1", len(m.Names))
			}
			for _, n := range m.Names {
				name := t.Name + "." + n.Name
				synopsis := render.OneLineField(fset, m, 0)
				syms = append(syms, &internal.SymbolMeta{
					Name:       name,
					ParentName: t.Name,
					Kind:       internal.SymbolKindMethod,
					Synopsis:   synopsis,
					Section:    internal.SymbolSectionTypes,
				})
			}
		}
	}
	return syms, nil
}
