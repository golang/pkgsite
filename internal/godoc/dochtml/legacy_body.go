// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

const (
	IdentifierBodyStart = `<div class="Documentation-content js-docContent">`
	IdentifierBodyEnd   = `</div>`
)

const legacyTmplBody = `

` + IdentifierBodyStart + ` {{/* Documentation content container */}}

{{- if or .Doc (index .Examples.Map "") -}}
	<section class="Documentation-overview">
		<h2 tabindex="-1" id="pkg-overview" class="Documentation-overviewHeader">Overview <a href="#pkg-overview">¶</a></h2>{{"\n\n" -}}
		{{render_doc .Doc}}{{"\n" -}}
		{{- template "example" (index .Examples.Map "") -}}
	</section>
{{- end -}}

{{- if or .Consts .Vars .Funcs .Types .Examples.List -}}
	<section class="Documentation-index">
		<h3 id="pkg-index" class="Documentation-indexHeader">Index <a href="#pkg-index">¶</a></h3>{{"\n\n" -}}
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
		<h3 tabindex="-1" id="pkg-examples" class="Documentation-examplesHeader">Examples <a href="#pkg-examples">¶</a></h3>{{"\n" -}}
		<ul class="Documentation-examplesList">{{"\n" -}}
			{{- range .Examples.List -}}
				<li><a href="#{{.ID}}" class="js-exampleHref">{{or .ParentID "Package"}}{{with .Suffix}} ({{.}}){{end}}</a></li>{{"\n" -}}
			{{- end -}}
		</ul>{{"\n" -}}
	</section>
	{{- end -}}

	{{- if .Consts -}}
	<section class="Documentation-constants">
		<h3 tabindex="-1" id="pkg-constants" class="Documentation-constantsHeader">Constants <a href="#pkg-constants">¶</a></h3>{{"\n"}}
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
		<h3 tabindex="-1" id="pkg-variables" class="Documentation-variablesHeader">Variables <a href="#pkg-variables">¶</a></h3>{{"\n"}}
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
			{{- $id := safe_id .Name -}}
			<h3 tabindex="-1" id="{{$id}}" data-kind="function" class="Documentation-functionHeader">func {{source_link .Name .Decl}} <a href="#{{$id}}">¶</a></h3>{{"\n"}}
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
			{{- $id := safe_id .Name -}}
			<h3 tabindex="-1" id="{{$id}}" data-kind="type" class="Documentation-typeHeader">type {{source_link .Name .Decl}} <a href="#{{$id}}">¶</a></h3>{{"\n"}}
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
				{{- $id := safe_id .Name -}}
				<h3 tabindex="-1" id="{{$id}}" data-kind="function" class="Documentation-typeFuncHeader">func {{source_link .Name .Decl}} <a href="#{{$id}}">¶</a></h3>{{"\n"}}
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
				{{- $id := (safe_id $name) -}}
				<h3 tabindex="-1" id="{{$id}}" data-kind="method" class="Documentation-typeMethodHeader">func ({{.Recv}}) {{source_link .Name .Decl}} <a href="#{{$id}}">¶</a></h3>{{"\n"}}
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

	<section class="Documentation-files">
		<h3 tabindex="-1" id="pkg-files" class="Documentation-filesHeader">Package Files <a href="#pkg-files">¶</a></h3>
		<ul class="Documentation-filesList">
			{{- range .Filenames -}}
				<li>{{file_link .}}</li>
			{{- end -}}
		</ul>
	</section>
	{{- end -}}
{{- end -}}

{{- if .Notes -}}
<section class="Documentation-notes">
	{{- range $marker, $content := .Notes -}}
	<div class="Documentation-note">
		<h2 tabindex="-1" id="{{index $.NoteIDs $marker}}" class="Documentation-noteHeader">{{$marker}}s <a href="#pkg-note-{{$marker}}">¶</a></h2>
		<ul class="Documentation-noteList" style="padding-left: 20px; list-style: initial;">{{"\n" -}}
		{{- range $v := $content -}}
			<li style="margin: 6px 0 6px 0;">{{render_doc $v.Body}}</li>
		{{- end -}}
		</ul>{{"\n" -}}
	</div>
	{{- end -}}
</section>
{{- end -}}

` + IdentifierBodyEnd + ` {{/* End documentation content container */}}
`

const tmplExample = `
{{- define "example" -}}
	{{- range . -}}
	<details tabindex="-1" id="{{.ID}}" class="Documentation-exampleDetails js-exampleContainer">{{"\n" -}}
		<summary class="Documentation-exampleDetailsHeader">Example{{with .Suffix}} ({{.}}){{end}} <a href="#{{.ID}}">¶</a></summary>{{"\n" -}}
		<div class="Documentation-exampleDetailsBody">{{"\n" -}}
			{{- if .Doc -}}{{render_doc .Doc}}{{"\n" -}}{{- end -}}
			{{- with play_url .Example -}}
			<p><a class="Documentation-examplesPlay" href="{{.}}">Open in Go playground »</a></p>{{"\n" -}}
			{{- end -}}
			<p>Code:</p>{{"\n" -}}
			{{render_code .Example}}{{"\n" -}}
			{{- if (or .Output .EmptyOutput) -}}
				<pre class="Documentation-exampleOutput">{{"\n"}}{{.Output}}</pre>{{"\n" -}}
			{{- end -}}
		</div>{{"\n" -}}
		{{- if .Play -}}
			<div class="Documentation-exampleButtonsContainer">
				<p class="Documentation-exampleError" role="alert" aria-atomic="true"></p>
				<button class="Documentation-examplePlayButton" aria-label="Play Code">Play</button>
			</div>
		{{- end -}}
	</details>{{"\n" -}}
	{{"\n"}}
	{{- end -}}
{{- end -}}
`
