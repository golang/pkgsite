// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"reflect"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/fetch/dochtml/internal/render"
	"golang.org/x/pkgsite/internal/fetch/internal/doc"
)

// htmlPackage is the template used to render documentation HTML.
// TODO(golang.org/issue/5060): finalize URL scheme and design for notes,
// then it becomes more viable to factor out inline CSS style.
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
		"render_short_synopsis": (*render.Renderer)(nil).ShortSynopsis,
		"render_synopsis":       (*render.Renderer)(nil).Synopsis,
		"render_doc":            (*render.Renderer)(nil).DocHTML,
		"render_decl":           (*render.Renderer)(nil).DeclHTML,
		"render_code":           (*render.Renderer)(nil).CodeHTML,
		"file_link":             func() string { return "" },
		"source_link":           func() string { return "" },
		"play_url":              func(*doc.Example) string { return "" },
		"safe_id":               render.SafeGoID,
	},
).Parse(`{{- "" -}}
{{if or .Doc .Consts .Vars .Funcs .Types .Examples.List}}
	<nav class="DocNav js-sideNav">
		<ul role="tree" aria-label="Outline">
			{{if or .Doc (index .Examples.Map "")}}
				<li class="DocNav-overview" role="none">
					<a href="#pkg-overview" role="treeitem" aria-level="1" tabindex="0">Overview</a>
				</li>
			{{end}}
			{{if .Examples.List}}
				<li class="DocNav-examples" role="none">
					<a href="#pkg-examples" role="treeitem" aria-level="1" tabindex="-1">Examples</a>
				</li>
			{{end}}
			{{if .Consts}}
				<li class="DocNav-constants" role="none">
					<a href="#pkg-constants" role="treeitem" aria-level="1" tabindex="-1">Constants</a>
				</li>
			{{end}}
			{{if .Vars}}
				<li class="DocNav-variables" role="none">
					<a href="#pkg-variables" role="treeitem" aria-level="1" tabindex="-1">Variables</a>
				</li>
			{{end}}

			{{if .Funcs}}
				<li class="DocNav-functions" role="none">
					<span class="DocNav-groupLabel" role="treeitem" aria-expanded="true" aria-level="1" aria-owns="nav-group-functions" tabindex="-1">Functions</span>
					<ul role="group" id="nav-group-functions">
						{{range .Funcs}}
							<li role="none">
								<a href="#{{.Name}}" title="{{render_short_synopsis .Decl}}" role="treeitem" aria-level="2" tabindex="-1">{{render_short_synopsis .Decl}}</a>
							</li>
						{{end}}
					</ul>
				</li>
			{{end}}

			{{if .Types}}
				<li class="DocNav-types" role="none">
					<span class="DocNav-groupLabel" role="treeitem" aria-expanded="true" aria-level="1" aria-owns="nav-group-types" tabindex="-1">Types</span>
					<ul role="group" id="nav-group-types">
						{{range .Types}}
							{{$tname := .Name}}
							<li role="none">
								{{if or .Funcs .Methods}}
									{{$navgroupname := (printf "nav.group.%s" $tname)}}
									{{$navgroupid := (safe_id $navgroupname)}}
									<a class="DocNav-groupLabel" href="#{{$tname}}" role="treeitem" aria-expanded="true" aria-level="2" data-aria-owns="{{$navgroupid}}" tabindex="-1">type {{$tname}}</a>
									<ul role="group" id="{{$navgroupid}}">
										{{range .Funcs}}
											<li role="none">
												<a href="#{{.Name}}" title="{{render_short_synopsis .Decl}}" role="treeitem" aria-level="3" tabindex="-1">{{render_short_synopsis .Decl}}</a>
											</li>
										{{end}}
										{{range .Methods}}
											<li role="none">
												<a href="#{{$tname}}.{{.Name}}" title="{{render_short_synopsis .Decl}}" role="treeitem" aria-level="3" tabindex="-1">{{render_short_synopsis .Decl}}</a>
											</li>
										{{end}}
									</ul>
								{{else}}
									<a href="#{{$tname}}" role="treeitem" aria-level="2" tabindex="-1">type {{$tname}}</a>
								{{end}} {{/* if or .Funcs .Methods */}}
							</li>
						{{end}} {{/* range .Types */}}
					</ul>
				</li>
			{{end}}

			{{if .Notes}}
				<li class="DocNav-notes" role="none">
					<span class="DocNav-groupLabel" role="treeitem" aria-expanded="true" aria-level="1" aria-owns="nav-group-notes" tabindex="-1">Notes</span>
					<ul role="group" id="nav-group-notes">
						{{range $marker, $item := .Notes}}
							<li role="none">
								<a href="#pkg-note-{{$marker}}" role="treeitem" aria-level="2" tabindex="-1">{{$marker}}s</a>
							</li>
						{{end}}
					</ul>
				</li>
			{{end}}

			{{if .Filenames}}
				<li class="DocNav-files" role="none">
					<a href="#pkg-files" role="treeitem" aria-level="1" tabindex="-1">Package Files</a>
				</li>
			{{end}}
		</ul>
	</nav>
	<nav class="DocNavMobile js-mobileNav">
		<label for="DocNavMobile-select" class="DocNavMobile-label">
			<svg class="DocNavMobile-selectIcon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="black" width="18px" height="18px">
				<path d="M0 0h24v24H0z" fill="none"/><path d="M3 9h14V7H3v2zm0 4h14v-2H3v2zm0 4h14v-2H3v2zm16 0h2v-2h-2v2zm0-10v2h2V7h-2zm0 6h2v-2h-2v2z"/>
			</svg>
			<span class="DocNavMobile-selectText js-mobileNavSelectText">Outline</span>
		</label>
		<select id="DocNavMobile-select" class="DocNavMobile-select">
		  <option value="">Outline</option>
			{{if or .Doc (index .Examples.Map "")}}
				<option value="pkg-overview">Overview</option>
			{{end}}
			{{if .Examples.List}}
				<option value="pkg-examples">Examples</option>
			{{end}}
			{{if .Consts}}
				<option value="pkg-constants">Constants</option>
			{{end}}
			{{if .Vars}}
				<option value="pkg-variables">Variables</option>
			{{end}}

			{{if .Funcs}}
				<optgroup label="Functions">
					{{range .Funcs}}
						<option value="{{.Name}}">{{render_short_synopsis .Decl}}</option>
					{{end}}
				</optgroup>
			{{end}}

			{{if .Types}}
				<optgroup label="Types">
					{{range .Types}}
						{{$tname := .Name}}
						<option value="{{$tname}}">type {{$tname}}</option>
						{{range .Funcs}}
							<option value="{{.Name}}">{{render_short_synopsis .Decl}}</option>
						{{end}}
						{{range .Methods}}
							<option value="{{$tname}}.{{.Name}}">{{render_short_synopsis .Decl}}</option>
						{{end}}
					{{end}} {{/* range .Types */}}
				</optgroup>
			{{end}}

			{{if .Notes}}
				<optgroup label="Notes">
					{{range $marker, $item := .Notes}}
						<option value="pkg-note-{{$marker}}">{{$marker}}s</option>
					{{end}}
				</optgroup>
			{{end}}
		</select>
	</nav>
{{end}}

<div class="Documentation-content js-docContent"> {{/* Documentation content container */}}

{{- if or .Doc (index .Examples.Map "") -}}
	<section class="Documentation-overview">
		<h2 tabindex="-1" id="pkg-overview" class="Documentation-overviewHeader">Overview <a href="#pkg-overview">¶</a></h2>{{"\n\n" -}}
		{{render_doc .Doc}}{{"\n" -}}
		{{- template "example" (index .Examples.Map "") -}}
	</section>
{{- end -}}

{{- if or .Consts .Vars .Funcs .Types .Examples.List -}}
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

</div> {{/* End documentation content container */}}

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
`))
