// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package templates

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/google/safehtml/template"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var templateFuncs = template.FuncMap{
	"add":      func(i, j int) int { return i + j },
	"subtract": func(i, j int) int { return i - j },
	"pluralize": func(i int, s string) string {
		if i == 1 {
			return s
		}
		return s + "s"
	},
	"commaseparate": func(s []string) string {
		return strings.Join(s, ", ")
	},
	"stripscheme": stripScheme,
	"capitalize":  cases.Title(language.Und).String,
	"queryescape": url.QueryEscape,
}

func stripScheme(url string) string {
	if i := strings.Index(url, "://"); i > 0 {
		return url[i+len("://"):]
	}
	return url
}

// funcsWithBasePath 在内置 templateFuncs 之上叠两个 base-path 助手：
//
//   - `{{abs "/static/foo.svg"}}` → 静态绝对路径前置 BasePath，
//     站点挂根时输出 `/static/foo.svg`，挂 -base-path=/gogodocs 时输出
//     `/gogodocs/static/foo.svg`。必须以 / 开头；否则原样（让作者改时一目了然）。
//   - `{{basepath}}` → 返回 BasePath 字符串本身（不带尾斜杠），用于模板里
//     拼动态 path 例如 `<a href="{{basepath}}/{{.Path}}">`——abs 不能拼带
//     变量的 path（template 函数实参不能嵌套表达式）。
//
// templateFuncs 是个全局只读 map，本函数 copy 一份再叠 helper，
// 避免不同 Server 实例（理论上多 BasePath 共存）相互覆盖 funcMap。
func funcsWithBasePath(basePath string) template.FuncMap {
	out := template.FuncMap{}
	for k, v := range templateFuncs {
		out[k] = v
	}
	out["abs"] = func(p string) string {
		if basePath == "" || !strings.HasPrefix(p, "/") {
			return p
		}
		return basePath + p
	}
	out["basepath"] = func() string { return basePath }
	return out
}

// ParsePageTemplates parses html templates contained in the given filesystem in
// order to generate a map of Name->*template.Template.
//
// basePath 形如 "/gogodocs" 或空字符串。空 = 站点挂根（pkg.go.dev 行为）。
// 模板里通过 `{{abs "/static/foo.svg"}}` 输出带 prefix 的绝对 URL。
//
// Separate templates are used so that certain contextual functions (e.g.
// templateName) can be bound independently for each page.
//
// Templates in directories prefixed with an underscore are considered helper
// templates and parsed together with the files in each base directory.
func ParsePageTemplates(fsys template.TrustedFS, basePath string) (map[string]*template.Template, error) {
	funcs := funcsWithBasePath(basePath)
	templates := make(map[string]*template.Template)
	htmlSets := [][]string{
		{"about"},
		{"badge"},
		{"error"},
		{"fetch"},
		{"homepage"},
		{"license-policy"},
		{"search"},
		{"search-help"},
		{"subrepo"},
		{"unit/importedby", "unit"},
		{"unit/imports", "unit"},
		{"unit/licenses", "unit"},
		{"unit/main", "unit"},
		{"unit/versions", "unit"},
		{"vuln"},
		{"vuln/main", "vuln"},
		{"vuln/list", "vuln"},
		{"vuln/entry", "vuln"},
		{"api"},
	}

	for _, set := range htmlSets {
		t, err := template.New("frontend.tmpl").Funcs(funcs).ParseFS(fsys, "frontend/*.tmpl")
		if err != nil {
			return nil, fmt.Errorf("ParseFS: %v", err)
		}
		helperGlob := "shared/*/*.tmpl"
		if _, err := t.ParseFS(fsys, helperGlob); err != nil {
			return nil, fmt.Errorf("ParseFS(%q): %v", helperGlob, err)
		}
		for _, f := range set {
			if _, err := t.ParseFS(fsys, path.Join("frontend", f, "*.tmpl")); err != nil {
				return nil, fmt.Errorf("ParseFS(%v): %v", f, err)
			}
		}
		templates[set[0]] = t
	}

	return templates, nil
}
