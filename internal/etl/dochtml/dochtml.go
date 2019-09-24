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
	"fmt"
	"go/printer"
	"go/token"
	"html/template"
	pathpkg "path"
	"reflect"
	"sort"

	"golang.org/x/discovery/internal/etl/dochtml/internal/render"
	"golang.org/x/discovery/internal/etl/internal/doc"
)

// Render renders package documentation HTML for the
// provided file set and package.
func Render(fset *token.FileSet, p *doc.Package) ([]byte, error) {
	var buf bytes.Buffer
	r := render.New(fset, p, &render.Options{
		PackageURL: func(path string) (url string) {
			return pathpkg.Join("/pkg", path)
		},
		DisableHotlinking: true,
	})
	err := template.Must(htmlPackage.Clone()).Funcs(map[string]interface{}{
		"render_synopsis": r.Synopsis,
		"render_doc":      r.DocHTML,
		"render_decl":     r.DeclHTML,
		"render_code":     r.CodeHTML,
	}).Execute(&buf, struct {
		RootURL string
		*doc.Package
		Examples *examples
	}{
		RootURL:  "/pkg",
		Package:  p,
		Examples: collectExamples(p),
	})
	if err != nil {
		err = fmt.Errorf("dochtml.Render: %v", err)
	}
	return buf.Bytes(), err
}

// examples is an internal representation of all package examples.
type examples struct {
	List []*example            // sorted by ParentID
	Map  map[string][]*example // keyed by top-level ID (e.g., "NewRing" or "PubSub.Receive") or empty string for package examples
}

// example is an internal representation of a single example.
type example struct {
	*doc.Example
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

// htmlPackage is the template used to render
// documentation HTML.
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
	},
).Parse(`{{- "" -}}
<ul>{{"\n" -}}
<li><a href="#pkg-overview">Overview</a></li>{{"\n" -}}
{{- if or .Consts .Vars .Funcs .Types -}}
	<li><a href="#pkg-index">Index</a></li>{{"\n" -}}
{{- end -}}
{{- if .Examples.List -}}
	<li><a href="#pkg-examples">Examples</a></li>{{"\n" -}}
{{- end -}}
</ul>{{"\n" -}}

<h2 id="pkg-overview">Overview <a href="#pkg-overview">¶</a></h2>{{"\n\n" -}}
	{{render_doc .Doc}}{{"\n" -}}
	{{- template "example" (index $.Examples.Map "") -}}

{{- if or .Consts .Vars .Funcs .Types -}}
	<h2 id="pkg-index">Index <a href="#pkg-index">¶</a></h2>{{"\n\n" -}}
	<ul class="indent">{{"\n" -}}
	{{- if .Consts -}}<li><a href="#pkg-constants">Constants</a></li>{{"\n"}}{{- end -}}
	{{- if .Vars -}}<li><a href="#pkg-variables">Variables</a></li>{{"\n"}}{{- end -}}
	{{- range .Funcs -}}<li><a href="#{{.Name}}">{{render_synopsis .Decl}}</a></li>{{"\n"}}{{- end -}}
	{{- range .Types -}}
		{{- $tname := .Name -}}
		<li><a href="#{{$tname}}">type {{$tname}}</a></li>{{"\n"}}
		{{- range .Funcs -}}
			<li class="indent"><a href="#{{.Name}}">{{render_synopsis .Decl}}</a></li>{{"\n"}}
		{{- end -}}
		{{- range .Methods -}}
			<li class="indent"><a href="#{{$tname}}.{{.Name}}">{{render_synopsis .Decl}}</a></li>{{"\n"}}
		{{- end -}}
	{{- end -}}
	</ul>{{"\n" -}}
	{{- if .Examples.List -}}
	<h3 id="pkg-examples">Examples <a href="#pkg-examples">¶</a></h3>{{"\n" -}}
		<ul class="indent">{{"\n" -}}
		{{- range .Examples.List -}}
			{{- $suffix := ternary .Suffix (printf " (%s)" .Suffix) "" -}}
			<li><a href="#example-{{.Name}}">{{or .ParentID "Package"}}{{$suffix}}</a></li>{{"\n" -}}
		{{- end -}}
		</ul>{{"\n" -}}
	{{- end -}}

	{{- if .Consts -}}<h3 id="pkg-constants">Constants <a href="#pkg-constants">¶</a></h3>{{"\n"}}{{- end -}}
	{{- range .Consts -}}
		{{- $out := render_decl .Doc .Decl -}}
		{{- $out.Decl -}}
		{{- $out.Doc -}}
		{{"\n"}}
	{{- end -}}

	{{- if .Vars -}}<h3 id="pkg-variables">Variables <a href="#pkg-variables}">¶</a></h3>{{"\n"}}{{- end -}}
	{{- range .Vars -}}
		{{- $out := render_decl .Doc .Decl -}}
		{{- $out.Decl -}}
		{{- $out.Doc -}}
		{{"\n"}}
	{{- end -}}

	{{- range .Funcs -}}
		<h3 id="{{.Name}}">func {{.Name}} <a href="#{{.Name}}">¶</a></h3>{{"\n"}}
		{{- $out := render_decl .Doc .Decl -}}
		{{- $out.Decl -}}
		{{- $out.Doc -}}
		{{"\n"}}
		{{- template "example" (index $.Examples.Map .Name) -}}
	{{- end -}}

	{{- range .Types -}}
		{{- $tname := .Name -}}
		<h3 id="{{.Name}}">type {{.Name}} <a href="#{{.Name}}">¶</a></h3>{{"\n"}}
		{{- $out := render_decl .Doc .Decl -}}
		{{- $out.Decl -}}
		{{- $out.Doc -}}
		{{"\n"}}
		{{- template "example" (index $.Examples.Map .Name) -}}

		{{- range .Consts -}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
		{{- end -}}

		{{- range .Vars -}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
		{{- end -}}

		{{- range .Funcs -}}
			<h3 id="{{.Name}}">func {{.Name}} <a href="#{{.Name}}">¶</a></h3>{{"\n"}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
			{{- template "example" (index $.Examples.Map .Name) -}}
		{{- end -}}

		{{- range .Methods -}}
			{{- $name := (printf "%s.%s" $tname .Name) -}}
			<h3 id="{{$name}}">func ({{.Recv}}) {{.Name}} <a href="#{{$name}}">¶</a></h3>{{"\n"}}
			{{- $out := render_decl .Doc .Decl -}}
			{{- $out.Decl -}}
			{{- $out.Doc -}}
			{{"\n"}}
			{{- template "example" (index $.Examples.Map $name) -}}
		{{- end -}}
	{{- end -}}
{{- end -}}

{{- define "example" -}}
	{{- range . -}}
	<div id="example-{{.Name}}" class="example">{{"\n" -}}
		<div class="example-header">{{"\n" -}}
			{{- $suffix := ternary .Suffix (printf " (%s)" .Suffix) "" -}}
			<a href="#example-{{.Name}}">Example{{$suffix}}</a>{{"\n" -}}
		</div>{{"\n" -}}
		<div class="example-body">{{"\n" -}}
			{{- if .Doc -}}{{render_doc .Doc}}{{"\n" -}}{{- end -}}
			<p>Code:</p>{{"\n" -}}
			{{render_code .Code}}{{"\n" -}}
			{{- if (or .Output .EmptyOutput) -}}
				<p>{{ternary .Unordered "Unordered output:" "Output:"}}</p>{{"\n" -}}
				<pre>{{"\n"}}{{.Output}}</pre>{{"\n" -}}
			{{- end -}}
		</div>{{"\n" -}}
	</div>{{"\n" -}}
	{{"\n"}}
	{{- end -}}
{{- end -}}
`))
