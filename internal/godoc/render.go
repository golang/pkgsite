// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"path"
	"sort"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
)

const (
	megabyte             = 1000 * 1000
	maxImportsPerPackage = 1000

	// Exported for tests.
	DocTooLargeReplacement = `<p>Documentation is too large to display.</p>`
)

// MaxDocumentationHTML is a limit on the rendered documentation HTML size.
//
// The current limit of is based on the largest packages that
// pkg.go.dev has encountered. See https://golang.org/issue/40576.
//
// It is a variable for testing.
var MaxDocumentationHTML = 20 * megabyte

// A Renderer renders documentation for a Package.
type Renderer struct {
}

// Render renders the documentation for the package.
// Rendering destroys p's AST; do not call any methods of p after it returns.
func (p *Package) Render(ctx context.Context, innerPath string, sourceInfo *source.Info, modInfo *ModuleInfo) (synopsis string, imports []string, html safehtml.HTML, err error) {
	// This is mostly copied from internal/fetch/fetch.go.
	defer derrors.Wrap(&err, "godoc.Package.Render(%q, %q, %q)", modInfo.ModulePath, modInfo.ResolvedVersion, innerPath)

	p.renderCalled = true
	d, err := p.docPackage(innerPath, modInfo)
	if err != nil {
		return "", nil, safehtml.HTML{}, err
	}

	// Render documentation HTML.
	opts := p.renderOptions(innerPath, sourceInfo, modInfo)
	docHTML, err := dochtml.Render(ctx, p.Fset, d, opts)
	if errors.Is(err, ErrTooLarge) {
		docHTML = template.MustParseAndExecuteToHTML(DocTooLargeReplacement)
	} else if err != nil {
		return "", nil, safehtml.HTML{}, fmt.Errorf("dochtml.Render: %v", err)
	}
	return doc.Synopsis(d.Doc), d.Imports, docHTML, err
}

// docPackage computes and returns a doc.Package.
func (p *Package) docPackage(innerPath string, modInfo *ModuleInfo) (_ *doc.Package, err error) {
	defer derrors.Wrap(&err, "docPackage(%q, %q, %q)", innerPath, modInfo.ModulePath, modInfo.ResolvedVersion)
	importPath := path.Join(modInfo.ModulePath, innerPath)
	if modInfo.ModulePath == stdlib.ModulePath {
		importPath = innerPath
	}
	if modInfo.ModulePackages == nil {
		modInfo.ModulePackages = p.ModulePackagePaths
	}

	// The "builtin" package in the standard library is a special case.
	// We want to show documentation for all globals (not just exported ones),
	// and avoid association of consts, vars, and factory functions with types
	// since it's not helpful (see golang.org/issue/6645).
	var noFiltering, noTypeAssociation bool
	if modInfo.ModulePath == stdlib.ModulePath && importPath == "builtin" {
		noFiltering = true
		noTypeAssociation = true
	}

	// Compute package documentation.
	var m doc.Mode
	if noFiltering {
		m |= doc.AllDecls
	}
	var allGoFiles []*ast.File
	for _, f := range p.Files {
		allGoFiles = append(allGoFiles, f.AST)
	}
	d, err := doc.NewFromFiles(p.Fset, allGoFiles, importPath, m)
	if err != nil {
		return nil, fmt.Errorf("doc.NewFromFiles: %v", err)
	}

	if d.ImportPath != importPath {
		panic(fmt.Errorf("internal error: *doc.Package has an unexpected import path (%q != %q)", d.ImportPath, importPath))
	}
	if noTypeAssociation {
		for _, t := range d.Types {
			d.Consts, t.Consts = append(d.Consts, t.Consts...), nil
			d.Vars, t.Vars = append(d.Vars, t.Vars...), nil
			d.Funcs, t.Funcs = append(d.Funcs, t.Funcs...), nil
		}
		sort.Slice(d.Funcs, func(i, j int) bool { return d.Funcs[i].Name < d.Funcs[j].Name })
	}

	// Process package imports.
	if len(d.Imports) > maxImportsPerPackage {
		return nil, fmt.Errorf("%d imports found package %q; exceeds limit %d for maxImportsPerPackage", len(d.Imports), importPath, maxImportsPerPackage)
	}
	return d, nil
}

// renderOptions returns a RenderOptions for p.
func (p *Package) renderOptions(innerPath string, sourceInfo *source.Info, modInfo *ModuleInfo) dochtml.RenderOptions {
	sourceLinkFunc := func(n ast.Node) string {
		if sourceInfo == nil {
			return ""
		}
		p := p.Fset.Position(n.Pos())
		if p.Line == 0 { // invalid Position
			return ""
		}
		return sourceInfo.LineURL(path.Join(innerPath, p.Filename), p.Line)
	}
	fileLinkFunc := func(filename string) string {
		if sourceInfo == nil {
			return ""
		}
		return sourceInfo.FileURL(path.Join(innerPath, filename))
	}

	return dochtml.RenderOptions{
		FileLinkFunc:   fileLinkFunc,
		SourceLinkFunc: sourceLinkFunc,
		ModInfo:        modInfo,
		Limit:          int64(MaxDocumentationHTML),
	}
}

// RenderParts renders the documentation for the package in parts.
// Rendering destroys p's AST; do not call any methods of p after it returns.
func (p *Package) RenderParts(ctx context.Context, innerPath string, sourceInfo *source.Info, modInfo *ModuleInfo) (_ *dochtml.Parts, err error) {
	p.renderCalled = true

	d, err := p.docPackage(innerPath, modInfo)
	if err != nil {
		return nil, err
	}
	opts := p.renderOptions(innerPath, sourceInfo, modInfo)
	parts, err := dochtml.RenderParts(ctx, p.Fset, d, opts)
	if errors.Is(err, ErrTooLarge) {
		return &dochtml.Parts{Body: template.MustParseAndExecuteToHTML(DocTooLargeReplacement)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dochtml.Render: %v", err)
	}
	return parts, nil
}

// RenderPartsFromUnit is a convenience function that first decodes the source
// in the unit, which must exist, and then calls RenderParts.
func RenderPartsFromUnit(ctx context.Context, u *internal.Unit) (_ *dochtml.Parts, err error) {
	docPkg, err := DecodePackage(u.Documentation[0].Source)
	if err != nil {
		return nil, err
	}
	modInfo := &ModuleInfo{
		ModulePath:      u.ModulePath,
		ResolvedVersion: u.Version,
		ModulePackages:  nil, // will be provided by docPkg
	}
	var innerPath string
	if u.ModulePath == stdlib.ModulePath {
		innerPath = u.Path
	} else if u.Path != u.ModulePath {
		innerPath = u.Path[len(u.ModulePath)+1:]
	}
	return docPkg.RenderParts(ctx, innerPath, u.SourceInfo, modInfo)
}
