// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"html/template"
	"reflect"

	"golang.org/x/discovery/internal/etl/dochtml/internal/render"
)

// htmlPackage is the template used to render documentation HTML.
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
					<li><ul class="Documentation-indexTypeFunctions">{{"\n" -}}
					{{range .}}<li><a href="#{{.Name}}">{{render_synopsis .Decl}}</a></li>{{"\n"}}{{end}}
					</ul></li>{{"\n" -}}
				{{- end -}}
				{{- with .Methods -}}
					<li><ul class="Documentation-indexTypeMethods">{{"\n" -}}
					{{range .}}<li><a href="#{{$tname}}.{{.Name}}">{{render_synopsis .Decl}}</a></li>{{"\n"}}{{end}}
					</ul></li>{{"\n" -}}
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
