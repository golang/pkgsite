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
	"reflect"
	"sort"

	"golang.org/x/discovery/internal/etl/dochtml/internal/render"
	"golang.org/x/discovery/internal/etl/internal/doc"
	"golang.org/x/xerrors"
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
func Render(fset *token.FileSet, p *doc.Package, opt RenderOptions) ([]byte, error) {
	if opt.Limit == 0 {
		const megabyte = 1000 * 1000
		opt.Limit = 10 * megabyte
	}

	// When rendering documentation for commands, display
	// the package comment and notes, but no declarations.
	if p.Name == "main" {
		// Make a copy to avoid modifying caller's *doc.Package.
		p2 := *p
		p = &p2

		// Clear top-level declarations.
		p.Consts = nil
		p.Types = nil
		p.Vars = nil
		p.Funcs = nil
		p.Examples = nil
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
		return nil, xerrors.Errorf("dochtml.Render: %w", ErrTooLarge)
	} else if err != nil {
		return nil, fmt.Errorf("dochtml.Render: %v", err)
	}
	return buf.B.Bytes(), nil
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

// htmlPackage is the template used to render
// documentation HTML.
// TODO(b/142795082): finalize URL scheme and design for notes, then factor out
// inline CSS style.
var htmlPackage = template.Must(template.New("package").Funcs(
	map[string]interface{}{
		"ternary": func(q, a, b interface{}) interface{} {
			v := reflect.ValueOf(q)
			vz := reflect.New(v.Type()).Elem()
			if reflect.DeepEqual(v.Interface(), vz.Interface()) {
				return b
			}
			return a
		},
		"render_synopsis": (*render.Renderer)(nil).Synopsis,
		"render_doc":      (*render.Renderer)(nil).DocHTML,
		"render_decl":     (*render.Renderer)(nil).DeclHTML,
		"render_code":     (*render.Renderer)(nil).CodeHTML,
		"source_link":     func() string { return "" },
	},
).Parse(`{{- "" -}}
{{- if or .Doc .Consts .Vars .Funcs .Types .Examples.List -}}
	<ul class="Documentation-toc">{{"\n" -}}
	{{- if or .Doc (index .Examples.Map "") -}}
		<li class="Documentation-tocItem"><a href="#pkg-overview">Overview</a></li>{{"\n" -}}
	{{- end -}}
	{{- if or .Consts .Vars .Funcs .Types -}}
		<li class="Documentation-tocItem"><a href="#pkg-index">Index</a></li>{{"\n" -}}
	{{- end -}}
	{{- if .Examples.List -}}
		<li class="Documentation-tocItem"><a href="#pkg-examples">Examples</a></li>{{"\n" -}}
	{{- end -}}
	</ul>{{"\n" -}}
{{- end -}}

{{- if or .Doc (index .Examples.Map "") -}}
        <section class="Documentation-overview">
		<h2 id="pkg-overview" class="Documentation-overviewHeader">Overview <a href="#pkg-overview">¶</a></h2>{{"\n\n" -}}
		{{render_doc .Doc}}{{"\n" -}}
		{{- template "example" (index .Examples.Map "") -}}
	</section>
{{- end -}}

{{- if or .Consts .Vars .Funcs .Types -}}
        <section class="Documentation-index">
		<h2 id="pkg-index" class="Documentation-indexHeader">Index <a href="#pkg-index">¶</a></h2>{{"\n\n" -}}
		<ul class="Documentation-indexList">{{"\n" -}}
			{{- if .Consts -}}<li class="Documentation-indexConstants"><a href="#pkg-constants">Constants</a></li>{{"\n"}}{{- end -}}
			{{- if .Vars -}}<li class="Documentation-indexVariables"><a href="#pkg-variables">Variables</a></li>{{"\n"}}{{- end -}}

			{{- range .Funcs -}}
			<li class="Documentation-indexFunction">
				<a href="#{{.Name}}">{{render_synopsis .Decl}}</a>
			</li>{{"\n"}}
			{{- end -}}

			{{- range .Types -}}
				{{- $tname := .Name -}}
				<li class="Documentation-indexType"><a href="#{{$tname}}">type {{$tname}}</a></li>{{"\n"}}
				{{- with .Funcs -}}
					<ul class="Documentation-indexTypeFunctions">{{"\n" -}}
					{{range .}}<li><a href="#{{.Name}}">{{render_synopsis .Decl}}</a></li>{{"\n"}}{{end}}
					</ul>{{"\n" -}}
				{{- end -}}
				{{- with .Methods -}}
					<ul class="Documentation-indexTypeMethods">{{"\n" -}}
					{{range .}}<li><a href="#{{$tname}}.{{.Name}}">{{render_synopsis .Decl}}</a></li>{{"\n"}}{{end}}
					</ul>{{"\n" -}}
				{{- end -}}
			{{- end -}}

			{{- range $marker, $item := .Notes -}}
			<li class="Documentation-indexNote"><a href="#pkg-note-{{$marker}}">{{$marker}}s</a></li>
			{{- end -}}
		</ul>{{"\n" -}}
	</section>

	{{- if .Examples.List -}}
	<section class="Documentation-examples">
		<h3 id="pkg-examples" class="Documentation-examplesHeader">Examples <a href="#pkg-examples">¶</a></h3>{{"\n" -}}
		<ul class="Documentation-examplesList">{{"\n" -}}
			{{- range .Examples.List -}}
				<li><a href="#{{.ID}}">{{or .ParentID "Package"}}{{with .Suffix}} ({{.}}){{end}}</a></li>{{"\n" -}}
			{{- end -}}
		</ul>{{"\n" -}}
	</section>
	{{- end -}}

	{{- if .Consts -}}
	<section class="Documentation-constants">
		<h3 id="pkg-constants" class="Documentation-constantsHeader">Constants <a href="#pkg-constants">¶</a></h3>{{"\n"}}
		{{- range .Consts -}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
		{{- end -}}
	</section>
	{{- end -}}

	{{- if .Vars -}}
	<section class="Documentation-variables">
		<h3 id="pkg-variables" class="Documentation-variablesHeader">Variables <a href="#pkg-variables">¶</a></h3>{{"\n"}}
		{{- range .Vars -}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
		{{- end -}}
	</section>
	{{- end -}}

	{{- if .Funcs -}}
	<section class="Documentation-functions">
		{{- range .Funcs -}}
		<div class="Documentation-function">
			<h3 id="{{.Name}}" class="Documentation-functionHeader">func {{source_link .Name .Decl}} <a href="#{{.Name}}">¶</a></h3>{{"\n"}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
			{{- template "example" (index $.Examples.Map .Name) -}}
		</div>
		{{- end -}}
	</section>
	{{- end -}}

	{{- if .Types -}}
	<section class="Documentation-types">
		{{- range .Types -}}
		<div class="Documentation-type">
			{{- $tname := .Name -}}
			<h3 id="{{.Name}}" class="Documentation-typeHeader">type {{source_link .Name .Decl}} <a href="#{{.Name}}">¶</a></h3>{{"\n"}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
			{{- template "example" (index $.Examples.Map .Name) -}}

			{{- range .Consts -}}
			<div class="Documentation-typeConstant">
				{{- $out := render_decl .Doc .Decl -}}
				{{- $out.Decl -}}
				{{- $out.Doc -}}
				{{"\n"}}
			</div>
			{{- end -}}

			{{- range .Vars -}}
			<div class="Documentation-typeVariable">
				{{- $out := render_decl .Doc .Decl -}}
				{{- $out.Decl -}}
				{{- $out.Doc -}}
				{{"\n"}}
			</div>
			{{- end -}}

			{{- range .Funcs -}}
			<div class="Documentation-typeFunc">
				<h3 id="{{.Name}}" class="Documentation-typeFuncHeader">func {{source_link .Name .Decl}} <a href="#{{.Name}}">¶</a></h3>{{"\n"}}
				{{- $out := render_decl .Doc .Decl -}}
				{{- $out.Decl -}}
				{{- $out.Doc -}}
				{{"\n"}}
				{{- template "example" (index $.Examples.Map .Name) -}}
			</div>
			{{- end -}}

			{{- range .Methods -}}
			<div class="Documentation-typeMethod">
				{{- $name := (printf "%s.%s" $tname .Name) -}}
				<h3 id="{{$name}}" class="Documentation-typeMethodHeader">func ({{.Recv}}) {{source_link .Name .Decl}} <a href="#{{$name}}">¶</a></h3>{{"\n"}}
				{{- $out := render_decl .Doc .Decl -}}
				{{- $out.Decl -}}
				{{- $out.Doc -}}
				{{"\n"}}
				{{- template "example" (index $.Examples.Map $name) -}}
			</div>
			{{- end -}}
		</div>
		{{- end -}}
	</section>
	{{- end -}}
{{- end -}}

{{- if .Notes -}}
<section class="Documentation-notes">
	{{- range $marker, $content := .Notes -}}
	<div class="Documentation-note">
		<h2 id="pkg-note-{{$marker}}" class="Documentation-noteHeader">{{$marker}}s <a href="#pkg-note-{{$marker}}">¶</a></h2>
		<ul class="Documentation-noteList" style="padding-left: 20px; list-style: initial;">{{"\n" -}}
		{{- range $v := $content -}}
			<li style="margin: 6px 0 6px 0;">{{render_doc $v.Body}}</li>
		{{- end -}}
		</ul>{{"\n" -}}
	</div>
	{{- end -}}
</section>
{{- end -}}

{{- define "example" -}}
	{{- range . -}}
	<details id="{{.ID}}" class="Documentation-exampleDetails">{{"\n" -}}
		<summary class="Documentation-exampleDetailsHeader">Example{{with .Suffix}} ({{.}}){{end}} <a href="#{{.ID}}">¶</a></summary>{{"\n" -}}
		<div class="Documentation-exampleDetailsBody">{{"\n" -}}
			{{- if .Doc -}}{{render_doc .Doc}}{{"\n" -}}{{- end -}}
			<p>Code:</p>{{"\n" -}}
			{{render_code .Code}}{{"\n" -}}
			{{- if (or .Output .EmptyOutput) -}}
				<p>{{ternary .Unordered "Unordered output:" "Output:"}}</p>{{"\n" -}}
				<pre>{{"\n"}}{{.Output}}</pre>{{"\n" -}}
			{{- end -}}
		</div>{{"\n" -}}
	</details>{{"\n" -}}
	{{"\n"}}
	{{- end -}}
{{- end -}}
`))
