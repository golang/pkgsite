// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dochtml renders Go package documentation into HTML.
//
// This package and its API are under development (see b/137567588).
// It currently relies on copies of external packages with active CLs applied.
// The plan is to iterate on the development internally for x/discovery
// needs first, before factoring it out somewhere non-internal where its
// API can no longer be easily modified.
package dochtml

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"html/template"
	pathpkg "path"
	"sort"

	"golang.org/x/discovery/internal/fetch/dochtml/internal/render"
	"golang.org/x/discovery/internal/fetch/internal/doc"
)

var (
	// ErrTooLarge represents an error where the rendered documentation HTML
	// size exceeded the specified limit. See the RenderOptions.Limit field.
	ErrTooLarge = errors.New("rendered documentation HTML size exceeded the specified limit")
)

// RenderOptions are options for Render.
type RenderOptions struct {
	SourceLinkFunc func(ast.Node) string
	Limit          int64 // If zero, a default limit of 10 megabytes is used.
}

// Render renders package documentation HTML for the
// provided file set and package.
//
// If the rendered documentation HTML size exceeds the specified limit,
// an error with ErrTooLarge in its chain will be returned.
func Render(fset *token.FileSet, p *doc.Package, opt RenderOptions) (string, error) {
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

	r := render.New(fset, p, &render.Options{
		PackageURL: func(path string) (url string) {
			return pathpkg.Join("/pkg", path)
		},
		DisableHotlinking: true,
	})

	sourceLink := func(name string, node ast.Node) template.HTML {
		link := opt.SourceLinkFunc(node)
		if link == "" {
			return template.HTML(name)
		}
		return template.HTML(fmt.Sprintf(`<a class="Documentation-source" href="%s">%s</a>`, link, name))
	}

	buf := &limitBuffer{
		B:      new(bytes.Buffer),
		Remain: opt.Limit,
	}
	err := template.Must(htmlPackage.Clone()).Funcs(map[string]interface{}{
		"render_synopsis": r.Synopsis,
		"render_doc":      r.DocHTML,
		"render_decl":     r.DeclHTML,
		"render_code":     r.CodeHTML,
		"source_link":     sourceLink,
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
	Suffix   string // optional suffix name
}

// Code returns an printer.CommentedNode if ex.Comments is non-nil,
// otherwise it returns ex.Code as is.
func (ex *example) Code() interface{} {
	if len(ex.Comments) > 0 {
		return &printer.CommentedNode{Node: ex.Example.Code, Comments: ex.Comments}
	}
	return ex.Example.Code
}

// collectExamples extracts examples from p
// into the internal examples representation.
func collectExamples(p *doc.Package) *examples {
	// TODO(dmitshur): Simplify this further.
	exs := &examples{
		List: nil,
		Map:  make(map[string][]*example),
	}
	for _, ex := range p.Examples {
		id := ""
		ex := &example{
			Example:  ex,
			ID:       exampleID(id, ex.Suffix),
			ParentID: id,
			Suffix:   ex.Suffix,
		}
		exs.List = append(exs.List, ex)
		exs.Map[id] = append(exs.Map[id], ex)
	}
	for _, f := range p.Funcs {
		for _, ex := range f.Examples {
			id := f.Name
			ex := &example{
				Example:  ex,
				ID:       exampleID(id, ex.Suffix),
				ParentID: id,
				Suffix:   ex.Suffix,
			}
			exs.List = append(exs.List, ex)
			exs.Map[id] = append(exs.Map[id], ex)
		}
	}
	for _, t := range p.Types {
		for _, ex := range t.Examples {
			id := t.Name
			ex := &example{
				Example:  ex,
				ID:       exampleID(id, ex.Suffix),
				ParentID: id,
				Suffix:   ex.Suffix,
			}
			exs.List = append(exs.List, ex)
			exs.Map[id] = append(exs.Map[id], ex)
		}
		for _, f := range t.Funcs {
			for _, ex := range f.Examples {
				id := f.Name
				ex := &example{
					Example:  ex,
					ID:       exampleID(id, ex.Suffix),
					ParentID: id,
					Suffix:   ex.Suffix,
				}
				exs.List = append(exs.List, ex)
				exs.Map[id] = append(exs.Map[id], ex)
			}
		}
		for _, m := range t.Methods {
			for _, ex := range m.Examples {
				id := t.Name + "." + m.Name
				ex := &example{
					Example:  ex,
					ID:       exampleID(id, ex.Suffix),
					ParentID: id,
					Suffix:   ex.Suffix,
				}
				exs.List = append(exs.List, ex)
				exs.Map[id] = append(exs.Map[id], ex)
			}
		}
	}
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
