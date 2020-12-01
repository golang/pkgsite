// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"reflect"
	"sync"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/godoc/dochtml/internal/render"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
)

var (
	loadOnce sync.Once

	// TODO(golang.org/issue/5060): finalize URL scheme and design for notes,
	// then it becomes more viable to factor out inline CSS style.
	unitTemplate *template.Template
)

// LoadTemplates reads and parses the templates used to generate documentation.
func LoadTemplates(dir template.TrustedSource) {
	loadOnce.Do(func() {
		join := template.TrustedSourceJoin
		tc := template.TrustedSourceFromConstant

		example := join(dir, tc("example.tmpl"))
		unitTemplate = template.Must(template.New("unit.tmpl").
			Funcs(tmpl).
			ParseFilesFromTrustedSources(
				join(dir, tc("unit.tmpl")),
				join(dir, tc("outline.tmpl")),
				join(dir, tc("sidenav.tmpl")),
				join(dir, tc("sidenav-mobile.tmpl")),
				join(dir, tc("body.tmpl")),
				join(dir, tc("links.tmpl")),
				example))
	})
}

var tmpl = map[string]interface{}{
	"ternary": func(q, a, b interface{}) interface{} {
		v := reflect.ValueOf(q)
		vz := reflect.New(v.Type()).Elem()
		if reflect.DeepEqual(v.Interface(), vz.Interface()) {
			return b
		}
		return a
	},
	"render_short_synopsis":    (*render.Renderer)(nil).ShortSynopsis,
	"render_synopsis":          (*render.Renderer)(nil).Synopsis,
	"render_doc":               (*render.Renderer)(nil).DocHTML,
	"render_doc_extract_links": (*render.Renderer)(nil).DocHTML,
	"render_decl":              (*render.Renderer)(nil).DeclHTML,
	"render_code":              (*render.Renderer)(nil).CodeHTML,
	"file_link":                func() string { return "" },
	"source_link":              func() string { return "" },
	"play_url":                 func(*doc.Example) string { return "" },
	"safe_id":                  render.SafeGoID,
}
