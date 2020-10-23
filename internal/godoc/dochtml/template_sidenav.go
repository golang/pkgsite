// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

const (
	IdentifierSidenavStart       = `<nav class="DocNav js-sideNav">`
	IdentifierSidenavMobileStart = `<nav class="DocNavMobile js-mobileNav">`
	IdentifierSidenavEnd         = `</nav>`
)

const tmplSidenav = `
{{if or .Doc .Consts .Vars .Funcs .Types}}
	` + IdentifierSidenavStart + `
		<ul role="tree" aria-label="Outline">
			{{if or .Doc (index .Examples.Map "")}}
				<li class="DocNav-overview" role="none">
					<a href="#pkg-overview" class="js-docNav" role="treeitem" aria-level="1" tabindex="0">Overview</a>
				</li>
			{{end}}
			{{- if or .Consts .Vars .Funcs .Types -}}
				<li class="DocNav-index" role="none">
					<a href="#pkg-index" class="DocNav-groupLabel{{if not .Examples.List}} DocNav-groupLabel--empty{{end}} js-docNav"
							role="treeitem" aria-expanded="false" aria-level="1" aria-owns="nav-group-index" tabindex="-1">
						Index
					</a>
					{{if .Examples.List}}
						<ul role="group" id="nav-group-index">
							<li role="none">
								<a href="#pkg-examples" role="treeitem" aria-level="2" tabindex="-1">Examples</a>
							</li>
						</ul>
					{{end}}
				</li>
				<li class="DocNav-constants" role="none">
					<a href="#pkg-constants" class="js-docNav" role="treeitem" aria-level="1" tabindex="-1">Constants</a>
				</li>
				<li class="DocNav-variables" role="none">
					<a href="#pkg-variables" class="js-docNav" role="treeitem" aria-level="1" tabindex="-1">Variables</a>
				</li>
				<li class="DocNav-functions" role="none">
					<a href="#pkg-functions" class="DocNav-groupLabel{{if eq (len .Funcs) 0}} DocNav-groupLabel--empty{{end}} js-docNav"
							role="treeitem" aria-expanded="false" aria-level="1" aria-owns="nav-group-functions" tabindex="-1">
						Functions
					</a>
					<ul role="group" id="nav-group-functions">
						{{range .Funcs}}
							<li role="none">
								<a href="#{{.Name}}" title="{{render_short_synopsis .Decl}}" role="treeitem" aria-level="2" tabindex="-1">{{render_short_synopsis .Decl}}</a>
							</li>
						{{end}}
					</ul>
				</li>
				<li class="DocNav-types" role="none">
					<a href="#pkg-types" class="DocNav-groupLabel{{if eq (len .Types) 0}} DocNav-groupLabel--empty{{end}} js-docNav"
							role="treeitem" aria-expanded="false" aria-level="1" aria-owns="nav-group-types" tabindex="-1">
						Types
					</a>
					<ul role="group" id="nav-group-types">
						{{range .Types}}
							{{$tname := .Name}}
							<li role="none">
								{{if or .Funcs .Methods}}
									{{$navgroupname := (printf "nav.group.%s" $tname)}}
									{{$navgroupid := (safe_id $navgroupname)}}
									<a class="DocNav-groupLabel js-docNavType" href="#{{$tname}}" role="treeitem" aria-expanded="false" aria-level="2" data-aria-owns="{{$navgroupid}}" tabindex="-1">type {{$tname}}</a>
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
					<a href="#pkg-notes" class="DocNav-groupLabel{{if eq (len .Notes) 0}} DocNav-groupLabel--empty{{end}} js-docNav"
							role="treeitem" aria-expanded="false" aria-level="1" aria-owns="nav-group-notes" tabindex="-1">Notes</a>
					<ul role="group" id="nav-group-notes">
						{{range $marker, $item := .Notes}}
							<li role="none">
								<a href="#pkg-note-{{$marker}}" role="treeitem" aria-level="2" tabindex="-1">{{(index $.NoteHeaders $marker).Label}}s</a>
							</li>
						{{end}}
					</ul>
				</li>
			{{end}}
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
			<option class="js-readmeOption" value="section-readme">README</option>
			<optgroup label="Documentation">
				{{if or .Doc (index .Examples.Map "")}}
					<option value="pkg-overview">Overview</option>
				{{end}}
				{{if or .Consts .Vars .Funcs .Types}}
					<option value="pkg-index">Index</option>
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
			</optgroup>

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
						<option value="pkg-note-{{$marker}}">{{(index $.NoteHeaders $marker).Label}}s</option>
					{{end}}
				</optgroup>
			{{end}}
			<option class="js-sourcefilesOption" value="section-sourcefiles">Source Files</option>
			<option class="js-directoriesOption" value="section-directories">Directories</option>
		</select>
	` + IdentifierSidenavEnd + `
{{end}}`
