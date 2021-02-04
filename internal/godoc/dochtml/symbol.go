// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"fmt"
	"go/ast"
	"go/token"

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
	return append(append(append(
		constants(p), variables(p)...), functions(p, fset)...), typs...), nil
}

func constants(p *doc.Package) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, c := range p.Consts {
		for _, n := range c.Names {
			syms = append(syms, &internal.Symbol{
				Name:     n,
				Synopsis: "const " + n,
				Section:  internal.SymbolSectionConstants,
				Kind:     internal.SymbolKindConstant,
			})
		}
	}
	return syms
}

func variables(p *doc.Package) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, v := range p.Vars {
		for _, n := range v.Names {
			syms = append(syms, &internal.Symbol{
				Name:     n,
				Synopsis: "var " + n,
				Section:  internal.SymbolSectionVariables,
				Kind:     internal.SymbolKindVariable,
			})
		}
	}
	return syms
}

func functions(p *doc.Package, fset *token.FileSet) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, f := range p.Funcs {
		syms = append(syms, &internal.Symbol{
			Name:     f.Name,
			Synopsis: render.OneLineNodeDepth(fset, f.Decl, 0),
			Section:  internal.SymbolSectionFunctions,
			Kind:     internal.SymbolKindFunction,
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
			Name:     typ.Name,
			Synopsis: render.OneLineNodeDepth(fset, spec, 0),
			Section:  internal.SymbolSectionTypes,
			Kind:     internal.SymbolKindType,
		}
		syms = append(syms, t)
		t.Children = append(append(append(append(append(
			t.Children,
			constantsForType(typ)...),
			variablesForType(typ)...),
			functionsForType(typ, fset)...),
			fieldsForType(typ.Name, spec, fset)...),
			mthds...)
	}
	return syms, nil
}

func constantsForType(t *doc.Type) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, c := range t.Consts {
		for _, n := range c.Names {
			syms = append(syms, &internal.Symbol{
				Name:       n,
				ParentName: t.Name,
				Kind:       internal.SymbolKindConstant,
				Synopsis:   "const " + n,
				Section:    internal.SymbolSectionTypes,
			})
		}
	}
	return syms
}

func variablesForType(t *doc.Type) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, v := range t.Vars {
		for _, n := range v.Names {
			syms = append(syms, &internal.Symbol{
				Name:       n,
				ParentName: t.Name,
				Kind:       internal.SymbolKindVariable,
				Synopsis:   "var " + n,
				Section:    internal.SymbolSectionTypes,
			})
		}
	}
	return syms
}

func functionsForType(t *doc.Type, fset *token.FileSet) []*internal.Symbol {
	var syms []*internal.Symbol
	for _, f := range t.Funcs {
		syms = append(syms, &internal.Symbol{
			Name:       f.Name,
			ParentName: t.Name,
			Kind:       internal.SymbolKindFunction,
			Synopsis:   render.OneLineNodeDepth(fset, f.Decl, 0),
			Section:    internal.SymbolSectionTypes,
		})
	}
	return syms
}

func fieldsForType(typName string, spec *ast.TypeSpec, fset *token.FileSet) []*internal.Symbol {
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		return nil
	}
	var syms []*internal.Symbol
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			syms = append(syms, &internal.Symbol{
				Name:       typName + "." + n.Name,
				ParentName: typName,
				Kind:       internal.SymbolKindField,
				Synopsis:   render.OneLineNodeDepth(fset, n, 0),
				Section:    internal.SymbolSectionTypes,
			})
		}
	}
	return syms
}

func methodsForType(t *doc.Type, spec *ast.TypeSpec, fset *token.FileSet) ([]*internal.Symbol, error) {
	var syms []*internal.Symbol
	for _, m := range t.Methods {
		syms = append(syms, &internal.Symbol{
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
				syms = append(syms, &internal.Symbol{
					Name:       t.Name + "." + n.Name,
					ParentName: t.Name,
					Kind:       internal.SymbolKindMethod,
					Synopsis:   render.OneLineNodeDepth(fset, n, 0),
					Section:    internal.SymbolSectionTypes,
				})
			}
		}
	}
	return syms, nil
}
