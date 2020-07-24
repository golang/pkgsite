// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dochtml renders Go package documentation into HTML.
//
// This package and its API are under development (see golang.org/issue/39883).
// The plan is to iterate on the development internally for x/pkgsite
// needs first, before factoring it out somewhere non-internal where its
// API can no longer be easily modified.
package dochtml

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/google/safehtml"
	"github.com/google/safehtml/legacyconversions"
	"github.com/google/safehtml/template"
	"github.com/google/safehtml/uncheckedconversions"
	"golang.org/x/pkgsite/internal/fetch/dochtml/internal/render"
	"golang.org/x/pkgsite/internal/fetch/internal/doc"
)

var (
	// ErrTooLarge represents an error where the rendered documentation HTML
	// size exceeded the specified limit. See the RenderOptions.Limit field.
	ErrTooLarge = errors.New("rendered documentation HTML size exceeded the specified limit")
)

// ModuleInfo contains all the information a package needs about the module it
// belongs to in order to render its documentation.
type ModuleInfo struct {
	ModulePath      string
	ResolvedVersion string
	// ModulePackages is the set of all full package paths in the module.
	ModulePackages map[string]bool
}

// RenderOptions are options for Render.
type RenderOptions struct {
	// FileLinkFunc optionally specifies a function that
	// returns a URL where file should be linked to.
	// file is the name component of a .go file in the package,
	// including the .go qualifier.
	// As a special case, FileLinkFunc may return the empty
	// string to indicate that a given file should not be linked.
	FileLinkFunc   func(file string) (url string)
	SourceLinkFunc func(ast.Node) string
	PlayURLFunc    func(*doc.Example) string // If set, returns the Go playground URL for the example
	// ModInfo optionally specifies information about the module the package
	// belongs to in order to render module-related documentation.
	ModInfo *ModuleInfo
	Limit   int64 // If zero, a default limit of 10 megabytes is used.
}

// Render renders package documentation HTML for the
// provided file set and package.
//
// If the rendered documentation HTML size exceeds the specified limit,
// an error with ErrTooLarge in its chain will be returned.
func Render(ctx context.Context, fset *token.FileSet, p *doc.Package, opt RenderOptions) (safehtml.HTML, error) {
	if opt.Limit == 0 {
		const megabyte = 1000 * 1000
		opt.Limit = 10 * megabyte
	}

	// Make a copy to avoid modifying caller's *doc.Package.
	p2 := *p
	p = &p2

	// When rendering documentation for commands, display
	// the package comment and notes, but no declarations.
	if p.Name == "main" {
		// Clear top-level declarations.
		p.Consts = nil
		p.Types = nil
		p.Vars = nil
		p.Funcs = nil
		p.Examples = nil
	}

	// Remove everything from the notes section that is not a bug. This
	// includes TODOs and other arbitrary notes.
	for k := range p.Notes {
		if k == "BUG" {
			continue
		}
		delete(p.Notes, k)
	}

	r := render.New(ctx, fset, p, &render.Options{
		PackageURL: func(path string) string {
			// Use the same module version for imported packages that belong to
			// the same module.
			versionedPath := path
			if opt.ModInfo != nil {
				versionedPath = versionedPkgPath(path, opt.ModInfo)
			}
			return pathpkg.Join("/pkg", versionedPath)
		},
		DisableHotlinking: true,
	})

	fileLink := func(name string) safehtml.HTML {
		return linkHTML(name, opt.FileLinkFunc(name), "Documentation-file")
	}
	sourceLink := func(name string, node ast.Node) safehtml.HTML {
		return linkHTML(name, opt.SourceLinkFunc(node), "Documentation-source")
	}
	playURLFunc := opt.PlayURLFunc
	if playURLFunc == nil {
		playURLFunc = func(*doc.Example) string {
			return ""
		}
	}

	tmpl := template.Must(htmlPackage.Clone()).Funcs(map[string]interface{}{
		"render_short_synopsis": r.ShortSynopsis,
		"render_synopsis":       r.Synopsis,
		"render_doc":            r.DocHTML,
		"render_decl":           r.DeclHTML,
		"render_code":           r.CodeHTML,
		"file_link":             fileLink,
		"source_link":           sourceLink,
		"play_url":              playURLFunc,
	})
	data := struct {
		RootURL string
		*doc.Package
		Examples *examples
		NoteIDs  map[string]safehtml.Identifier
	}{
		RootURL:  "/pkg",
		Package:  p,
		Examples: collectExamples(p),
		NoteIDs:  buildNoteIDs(p.Notes),
	}
	return executeToHTMLWithLimit(tmpl, data, opt.Limit)
}

// executeToHTMLWithLimit executes tmpl on data and returns the result as a safehtml.HTML.
// It returns an error if the size of the result exceeds limit.
func executeToHTMLWithLimit(tmpl *template.Template, data interface{}, limit int64) (safehtml.HTML, error) {
	buf := &limitBuffer{B: new(bytes.Buffer), Remain: limit}
	err := tmpl.Execute(buf, data)
	if buf.Remain < 0 {
		return safehtml.HTML{}, fmt.Errorf("dochtml.Render: %w", ErrTooLarge)
	} else if err != nil {
		return safehtml.HTML{}, fmt.Errorf("dochtml.Render: %v", err)
	}
	// This is safe because we're executing a safehtml template and not modifying the result afterwards.
	// We're just doing what safehtml/template.Template.ExecuteToHTML does
	// (https://github.com/google/safehtml/blob/b8ae3e5e1ce3/template/template.go#L136).
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(buf.B.String()), nil
}

// linkHTML returns an HTML-formatted name linked to the given URL.
// The class argument is the class of the 'a' tag.
// If url is the empty string, the name is not linked.
func linkHTML(name, url, class string) safehtml.HTML {
	if url == "" {
		return safehtml.HTMLEscaped(name)
	}
	return render.ExecuteToHTML(render.LinkTemplate, render.Link{Class: class, Href: url, Text: name})
}

// examples is an internal representation of all package examples.
type examples struct {
	List []*example            // sorted by ParentID
	Map  map[string][]*example // keyed by top-level ID (e.g., "NewRing" or "PubSub.Receive") or empty string for package examples
}

// example is an internal representation of a single example.
type example struct {
	*doc.Example
	ID       safehtml.Identifier // ID of example
	ParentID string              // ID of top-level declaration this example is attached to
	Suffix   string              // optional suffix name in title case
}

// Code returns an printer.CommentedNode if ex.Comments is non-nil,
// otherwise it returns ex.Code as is.
func (ex *example) Code() interface{} {
	if len(ex.Comments) > 0 {
		return &printer.CommentedNode{Node: ex.Example.Code, Comments: ex.Comments}
	}
	return ex.Example.Code
}

// WalkExamples calls fn for each Example in p,
// setting id to the name of the parent structure.
func WalkExamples(p *doc.Package, fn func(id string, ex *doc.Example)) {
	for _, ex := range p.Examples {
		fn("", ex)
	}
	for _, f := range p.Funcs {
		for _, ex := range f.Examples {
			fn(f.Name, ex)
		}
	}
	for _, t := range p.Types {
		for _, ex := range t.Examples {
			fn(t.Name, ex)
		}
		for _, f := range t.Funcs {
			for _, ex := range f.Examples {
				fn(f.Name, ex)
			}
		}
		for _, m := range t.Methods {
			for _, ex := range m.Examples {
				fn(t.Name+"."+m.Name, ex)
			}
		}
	}
}

// collectExamples extracts examples from p
// into the internal examples representation.
func collectExamples(p *doc.Package) *examples {
	exs := &examples{
		List: nil,
		Map:  make(map[string][]*example),
	}
	WalkExamples(p, func(id string, ex *doc.Example) {
		suffix := strings.Title(ex.Suffix)
		ex0 := &example{
			Example:  ex,
			ID:       exampleID(id, suffix),
			ParentID: id,
			Suffix:   suffix,
		}
		exs.List = append(exs.List, ex0)
		exs.Map[id] = append(exs.Map[id], ex0)
	})
	sort.SliceStable(exs.List, func(i, j int) bool {
		// TODO: Break ties by sorting by suffix, unless
		// not needed because of upstream slice order.
		return exs.List[i].ParentID < exs.List[j].ParentID
	})
	return exs
}

func exampleID(id, suffix string) safehtml.Identifier {
	switch {
	case id == "" && suffix == "":
		return safehtml.IdentifierFromConstant("example-package")
	case id == "" && suffix != "":
		render.ValidateGoDottedExpr(suffix)
		return legacyconversions.RiskilyAssumeIdentifier("example-package-" + suffix)
	case id != "" && suffix == "":
		render.ValidateGoDottedExpr(id)
		return legacyconversions.RiskilyAssumeIdentifier("example-" + id)
	case id != "" && suffix != "":
		render.ValidateGoDottedExpr(id)
		render.ValidateGoDottedExpr(suffix)
		return legacyconversions.RiskilyAssumeIdentifier("example-" + id + "-" + suffix)
	default:
		panic("unreachable")
	}
}

// buildNoteIDs constructs safehtml identifiers from note markers.
// It returns a map from each marker to its corresponding safehtml ID.
func buildNoteIDs(notes map[string][]*doc.Note) map[string]safehtml.Identifier {
	ids := map[string]safehtml.Identifier{}
	for marker := range notes {
		ids[marker] = safehtml.IdentifierFromConstantPrefix("pkg-note", marker)
	}
	return ids
}

// versionedPkgPath transforms package paths to contain the same version as the
// current module if the package belongs to the module. As a special case,
// versionedPkgPath will not add versions to standard library packages.
func versionedPkgPath(pkgPath string, modInfo *ModuleInfo) string {
	if modInfo == nil || !modInfo.ModulePackages[pkgPath] {
		return pkgPath
	}
	// We don't need to do anything special here for standard library packages
	// since pkgPath will never contain the "std/" module prefix, and
	// modInfo.ModulePackages contains this prefix for standard library packages.
	innerPkgPath := pkgPath[len(modInfo.ModulePath):]
	return fmt.Sprintf("%s@%s%s", modInfo.ModulePath, modInfo.ResolvedVersion, innerPkgPath)
}
