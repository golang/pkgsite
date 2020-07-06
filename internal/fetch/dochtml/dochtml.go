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
	"html"
	"html/template"
	pathpkg "path"
	"sort"
	"strings"

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
func Render(ctx context.Context, fset *token.FileSet, p *doc.Package, opt RenderOptions) (string, error) {
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

	fileLink := func(name string) template.HTML {
		return fileLinkHTML(name, opt.FileLinkFunc(name))
	}
	sourceLink := func(name string, node ast.Node) template.HTML {
		link := opt.SourceLinkFunc(node)
		if link == "" {
			return template.HTML(name)
		}
		return template.HTML(fmt.Sprintf(`<a class="Documentation-source" href="%s">%s</a>`, link, name))
	}
	playURLFunc := opt.PlayURLFunc
	if playURLFunc == nil {
		playURLFunc = func(*doc.Example) string {
			return ""
		}
	}
	buf := &limitBuffer{
		B:      new(bytes.Buffer),
		Remain: opt.Limit,
	}
	err := template.Must(htmlPackage.Clone()).Funcs(map[string]interface{}{
		"render_short_synopsis": r.ShortSynopsis,
		"render_synopsis":       r.Synopsis,
		"render_doc":            r.DocHTML,
		"render_decl":           r.DeclHTML,
		"render_code":           r.CodeHTML,
		"file_link":             fileLink,
		"source_link":           sourceLink,
		"play_url":              playURLFunc,
	}).Execute(buf, struct {
		RootURL string
		*doc.Package
		Examples *examples
	}{
		RootURL:  "/pkg",
		Package:  p,
		Examples: collectExamples(p),
	})
	if buf.Remain < 0 {
		return "", fmt.Errorf("dochtml.Render: %w", ErrTooLarge)
	} else if err != nil {
		return "", fmt.Errorf("dochtml.Render: %v", err)
	}
	return buf.B.String(), nil
}

// fileLinkHTML returns an HTML-formatted file name linked to the source URL.
// If link is the empty string, the file name is not linked.
func fileLinkHTML(name, link string) template.HTML {
	name = html.EscapeString(name)
	if link == "" {
		return template.HTML(name)
	}
	link = html.EscapeString(link)
	return template.HTML(fmt.Sprintf(`<a class="Documentation-file" href="%s">%s</a>`, link, name))
}

// examples is an internal representation of all package examples.
type examples struct {
	List []*example            // sorted by ParentID
	Map  map[string][]*example // keyed by top-level ID (e.g., "NewRing" or "PubSub.Receive") or empty string for package examples
}

// example is an internal representation of a single example.
type example struct {
	*doc.Example
	ID       string // ID of example
	ParentID string // ID of top-level declaration this example is attached to
	Suffix   string // optional suffix name in title case
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

func exampleID(id, suffix string) string {
	switch {
	case id == "" && suffix == "":
		return "example-package"
	case id == "" && suffix != "":
		return "example-package-" + suffix
	case id != "" && suffix == "":
		return "example-" + id
	case id != "" && suffix != "":
		return "example-" + id + "-" + suffix
	default:
		panic("unreachable")
	}
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
