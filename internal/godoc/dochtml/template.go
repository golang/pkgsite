// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"go/doc"
	"path"
	"reflect"
	"sync"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/godoc/dochtml/internal/render"
)

var (
	loadOnce sync.Once

	// TODO(golang.org/issue/5060): finalize URL scheme and design for notes,
	// then it becomes more viable to factor out inline CSS style.
	bodyTemplate, outlineTemplate, sidenavTemplate *template.Template

	// basePath：单进程包级 URL 前缀（如 "/gogodocs"），由 [LoadTemplates] 写入；
	// 渲染 godoc cross-reference 时（[dochtml.go] 的 PackageURL）拼到链接前。
	// dochtml 包是单实例使用，存包级 var 可接受——pkgsite 单进程只有一个 BasePath。
	basePath string
)

// BasePath 暴露包级 base path（fork 用），供 [render] 包之外的链接生成需要时读。
// 仅在 [LoadTemplates] 之后读取才有意义；并发安全（一次写入后只读）。
func BasePath() string { return basePath }

func Templates() []*template.Template {
	return []*template.Template{bodyTemplate, outlineTemplate, sidenavTemplate}
}

// LoadTemplates reads and parses the templates used to generate documentation.
//
// basePathPrefix 形如 "/gogodocs" 或空字符串。空 = 站点挂根（pkg.go.dev 行为）。
// 仅在第一次调用生效（loadOnce 之后包级 templates 已 freeze），但 basePath
// 每次调用都会覆盖——单进程 pkgsite 全程只配一个 BasePath，重复写无副作用。
func LoadTemplates(fsys template.TrustedFS, basePathPrefix string) {
	basePath = basePathPrefix
	const dir = "doc"
	loadOnce.Do(func() {
		bodyTemplate = template.Must(template.New("body.tmpl").
			Funcs(tmpl).
			ParseFS(fsys,
				path.Join(dir, "body.tmpl"),
				path.Join(dir, "declaration.tmpl"),
				path.Join(dir, "example.tmpl")))
		outlineTemplate = template.Must(template.New("outline.tmpl").
			Funcs(tmpl).
			ParseFS(fsys, path.Join(dir, "outline.tmpl")))
		sidenavTemplate = template.Must(template.New("sidenav-mobile.tmpl").
			Funcs(tmpl).
			ParseFS(fsys, path.Join(dir, "sidenav-mobile.tmpl")))
	})
}

var tmpl = map[string]any{
	"ternary": func(q, a, b any) any {
		v := reflect.ValueOf(q)
		vz := reflect.New(v.Type()).Elem()
		if reflect.DeepEqual(v.Interface(), vz.Interface()) {
			return b
		}
		return a
	},
	// These are just placeholders, for parsing. The actual functions
	// are in dochtml.go.
	"render_short_synopsis":    (*render.Renderer)(nil).ShortSynopsis,
	"render_synopsis":          (*render.Renderer)(nil).Synopsis,
	"render_doc":               (*render.Renderer)(nil).DocHTML,
	"render_doc_extract_links": (*render.Renderer)(nil).DocHTMLExtractLinks,
	"render_decl":              (*render.Renderer)(nil).DeclHTML,
	"render_code":              (*render.Renderer)(nil).CodeHTML,
	"file_link":                func() string { return "" },
	"source_link":              func(string, any) string { return "" },
	"since_version":            func(string) safehtml.HTML { return safehtml.HTML{} },
	"play_url":                 func(*doc.Example) string { return "" },
	"safe_id":                  render.SafeGoID,
}
