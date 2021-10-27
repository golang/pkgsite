// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"context"
	"path"
	"reflect"
	"sync"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc/dochtml/internal/render"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
)

var (
	loadOnce sync.Once

	// TODO(golang.org/issue/5060): finalize URL scheme and design for notes,
	// then it becomes more viable to factor out inline CSS style.
	bodyTemplate, outlineTemplate, sidenavTemplate *template.Template
)

// LoadTemplates reads and parses the templates used to generate documentation.
func LoadTemplates(fsys template.TrustedFS) {
	const dir = "doc"
	loadOnce.Do(func() {
		bodyTemplate = template.Must(template.New("body.tmpl").
			Funcs(tmpl).
			ParseFS(fsys,
				path.Join(dir, "body.tmpl"),
				path.Join(dir, "declaration.tmpl"),
				path.Join(dir, "example.tmpl")))
		if experiment.IsActive(context.Background(), internal.ExperimentNewUnitLayout) {
			outlineTemplate = template.Must(template.New("outline.tmpl").
				Funcs(tmpl).
				ParseFS(fsys, path.Join(dir, "outline.tmpl")))
		} else {
			outlineTemplate = template.Must(template.New("legacy-outline.tmpl").
				Funcs(tmpl).
				ParseFS(fsys, path.Join(dir, "legacy-outline.tmpl")))
		}
		sidenavTemplate = template.Must(template.New("sidenav-mobile.tmpl").
			Funcs(tmpl).
			ParseFS(fsys, path.Join(dir, "sidenav-mobile.tmpl")))
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
	"source_link":              func(string, interface{}) string { return "" },
	"since_version":            func(string) safehtml.HTML { return safehtml.HTML{} },
	"play_url":                 func(*doc.Example) string { return "" },
	"safe_id":                  render.SafeGoID,
}
