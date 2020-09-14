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
).Parse(tmplHTML))

const tmplHTML = `{{- "" -}}` + tmplSidenav + tmplBody + tmplExample
