// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

const legacyTmplSidenav = `
{{- if or .Doc .Consts .Vars .Funcs .Types .Examples.List}}
	` + IdentifierSidenavStart + `
		<ul role="tree" aria-label="Outline">
			{{- if or .Doc (index .Examples.Map "") }}
				<li class="DocNav-overview" role="none">
					<a href="#pkg-overview" role="treeitem" aria-level="1" tabindex="0">Overview</a>
				</li>
			{{end}}
			{{- if or .Consts .Vars .Funcs .Types .Examples.List -}}
				<li class="DocNav-index" role="none">
					<a href="#pkg-index" role="treeitem" aria-level="1" tabindex="0">Index</a>
				</li>
			{{- end}}
			{{- if .Examples.List}}
				<li class="DocNav-examples" role="none">
					<a href="#pkg-examples" role="treeitem" aria-level="1" tabindex="-1">Examples</a>
				</li>
			{{- end}}
			{{- if .Consts}}
				<li class="DocNav-constants" role="none">
					<a href="#pkg-constants" role="treeitem" aria-level="1" tabindex="-1">Constants</a>
				</li>
			{{- end}}
			{{- if .Vars}}
				<li class="DocNav-variables" role="none">
					<a href="#pkg-variables" role="treeitem" aria-level="1" tabindex="-1">Variables</a>
				</li>
			{{- end}}

			{{- if .Funcs}}
				<li class="DocNav-functions" role="none">
					<span class="DocNav-groupLabel" role="treeitem" aria-expanded="true" aria-level="1" aria-owns="nav-group-functions" tabindex="-1">Functions</span>
					<ul role="group" id="nav-group-functions">
						{{- range .Funcs}}
							<li role="none">
								<a href="#{{.Name}}" title="{{render_short_synopsis .Decl}}" role="treeitem" aria-level="2" tabindex="-1">{{render_short_synopsis .Decl}}</a>
							</li>
						{{- end}}
					</ul>
				</li>
			{{- end}}

			{{- if .Types}}
				<li class="DocNav-types" role="none">
					<span class="DocNav-groupLabel" role="treeitem" aria-expanded="true" aria-level="1" aria-owns="nav-group-types" tabindex="-1">Types</span>
					<ul role="group" id="nav-group-types">
						{{- range .Types}}
							{{- $tname := .Name}}
							<li role="none">
								{{- if or .Funcs .Methods}}
									{{- $navgroupname := (printf "nav.group.%s" $tname)}}
									{{- $navgroupid := (safe_id $navgroupname)}}
									<a class="DocNav-groupLabel" href="#{{$tname}}" role="treeitem" aria-expanded="true" aria-level="2" data-aria-owns="{{$navgroupid}}" tabindex="-1">type {{$tname}}</a>
									<ul role="group" id="{{$navgroupid}}">
										{{- range .Funcs}}
											<li role="none">
												<a href="#{{.Name}}" title="{{render_short_synopsis .Decl}}" role="treeitem" aria-level="3" tabindex="-1">{{render_short_synopsis .Decl}}</a>
											</li>
										{{- end}}
										{{- range .Methods}}
											<li role="none">
												<a href="#{{$tname}}.{{.Name}}" title="{{render_short_synopsis .Decl}}" role="treeitem" aria-level="3" tabindex="-1">{{render_short_synopsis .Decl}}</a>
											</li>
										{{- end}}
									</ul>
								{{- else}}
									<a href="#{{$tname}}" role="treeitem" aria-level="2" tabindex="-1">type {{$tname}}</a>
								{{- end -}} {{/* if or .Funcs .Methods */}}
							</li>
						{{- end -}} {{/* range .Types */}}
					</ul>
				</li>
			{{- end}}

			{{- if .Notes}}
				<li class="DocNav-notes" role="none">
					<span class="DocNav-groupLabel" role="treeitem" aria-expanded="true" aria-level="1" aria-owns="nav-group-notes" tabindex="-1">Notes</span>
					<ul role="group" id="nav-group-notes">
						{{- range $marker, $item := .Notes}}
							<li role="none">
								<a href="#pkg-note-{{$marker}}" role="treeitem" aria-level="2" tabindex="-1">{{$marker}}s</a>
							</li>
						{{- end}}
					</ul>
				</li>
			{{- end}}

			{{- if .Filenames}}
				<li class="DocNav-files" role="none">
					<a href="#pkg-files" role="treeitem" aria-level="1" tabindex="-1">Package Files</a>
				</li>
			{{- end}}
		</ul>
	</nav>
	` + IdentifierSidenavMobileStart + `
		<label for="DocNavMobile-select" class="DocNavMobile-label">
			<svg class="DocNavMobile-selectIcon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="black" width="18px" height="18px">
				<path d="M0 0h24v24H0z" fill="none"/><path d="M3 9h14V7H3v2zm0 4h14v-2H3v2zm0 4h14v-2H3v2zm16 0h2v-2h-2v2zm0-10v2h2V7h-2zm0 6h2v-2h-2v2z"/>
			</svg>
			<span class="DocNavMobile-selectText js-mobileNavSelectText">Outline</span>
		</label>
		<select id="DocNavMobile-select" class="DocNavMobile-select">
			<option value="">Outline</option>
			{{- if or .Doc (index .Examples.Map "")}}
				<option value="pkg-overview">Overview</option>
			{{- end}}
			{{- if .Examples.List}}
				<option value="pkg-examples">Examples</option>
			{{- end}}
			{{- if .Consts}}
				<option value="pkg-constants">Constants</option>
			{{- end}}
			{{- if .Vars}}
				<option value="pkg-variables">Variables</option>
			{{- end}}

			{{- if .Funcs}}
				<optgroup label="Functions">
					{{- range .Funcs}}
						<option value="{{.Name}}">{{render_short_synopsis .Decl}}</option>
					{{- end}}
				</optgroup>
			{{- end}}

			{{- if .Types}}
				<optgroup label="Types">
					{{- range .Types}}
						{{- $tname := .Name}}
						<option value="{{$tname}}">type {{$tname}}</option>
						{{- range .Funcs}}
							<option value="{{.Name}}">{{render_short_synopsis .Decl}}</option>
						{{- end}}
						{{- range .Methods}}
							<option value="{{$tname}}.{{.Name}}">{{render_short_synopsis .Decl}}</option>
						{{- end}}
					{{- end -}} {{/* range .Types */}}
				</optgroup>
			{{- end}}

			{{- if .Notes}}
				<optgroup label="Notes">
					{{- range $marker, $item := .Notes}}
						<option value="pkg-note-{{$marker}}">{{$marker}}s</option>
					{{- end}}
				</optgroup>
			{{- end}}
		</select>
	` + IdentifierSidenavEnd + `
{{end}}`
