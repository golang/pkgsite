// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"context"
	"reflect"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc/dochtml/internal/render"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
)

// htmlPackage is the template used to render documentation HTML.
// TODO(golang.org/issue/5060): finalize URL scheme and design for notes,
// then it becomes more viable to factor out inline CSS style.
func htmlPackage(ctx context.Context) *template.Template {
	t := template.New("package").Funcs(tmpl)
	if experiment.IsActive(ctx, internal.ExperimentUnitPage) {
		return template.Must(t.Parse(tmplHTML))
	}
	return template.Must(t.Parse(legacyTmplHTML))
}

const (
	tmplHTML = `{{- "" -}}` + tmplSidenav + tmplBody + tmplExample

	// legacyTmplHTML should not be edited.
	legacyTmplHTML = `{{- "" -}}` + legacyTmplSidenav + legacyTmplBody + tmplExample
)

var tmpl = map[string]interface{}{
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
	"uses_link":             func() string { return "" },
	"play_url":              func(*doc.Example) string { return "" },
	"safe_id":               render.SafeGoID,
}
