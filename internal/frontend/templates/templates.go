// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package templates

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/google/safehtml"
	"github.com/google/safehtml/template"
	"github.com/google/safehtml/uncheckedconversions"
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
//     返回 [safehtml.TrustedResourceURL]——safehtml/template 不会校验，允许
//     模板里跟 `?version={{.AppVersionLabel}}` 等动态 query 拼接（普通
//     string return 会被 safehtml 拒绝，认为 ?version= 不是合法 URL prefix）。
//   - `{{basepath}}` → 返回 BasePath 字符串本身（不带尾斜杠），用于模板里
//     拼动态 path 例如 `<a href="{{basepath}}/{{.Path}}">`——abs 不能拼带
//     变量的 path（template 函数实参不能嵌套表达式）。返回 [safehtml.URL]
//     让 safehtml 信任这是已校验的 URL 段。
//
// 安全说明：用 [uncheckedconversions] 绕过 safehtml 的 TrustedResourceURL
// 校验是 deliberate——abs/basepath 的入参在 fork 内是模板里的字面常量
// （已经过 git review），不是用户输入；basePath 来自 -base-path flag 被
// validateBasePath 校验形如 "/foo"，不会注入恶意内容。
//
// templateFuncs 是个全局只读 map，本函数 copy 一份再叠 helper，
// 避免不同 Server 实例（理论上多 BasePath 共存）相互覆盖 funcMap。
func funcsWithBasePath(basePath string) template.FuncMap {
	out := template.FuncMap{}
	for k, v := range templateFuncs {
		out[k] = v
	}
	// abs 返回普通 string——safehtml/template 会按上下文自动 escape：
	//   - <a href="{{abs ...}}"> → URL escape
	//   - <script>"{{abs ...}}"</script> → JS string escape
	//   - {{abs ...}} 在文本节点 → HTML escape
	// 用 string 而非 safehtml.TrustedResourceURL 是因为后者无法在 <script>
	// 内 inline string literal context 通过校验。<link href> 跟 dynamic
	// query 拼接（"?version="）的场景由专门的 [asset] helper 处理。
	out["abs"] = func(p string) string {
		if basePath == "" || !strings.HasPrefix(p, "/") {
			return p
		}
		return basePath + p
	}
	out["basepath"] = func() string { return basePath }
	// asset 拼接 base path + 静态资源路径 + 可选 ?version=<v> query。
	// safehtml/template 不允许在 <link href> / <script src> 等 TrustedResourceURL
	// 上下文里把 ?version= 后面接 dynamic template action（"?version=" 不是合法
	// TrustedResourceURL prefix），所以 query 必须在 helper 里跟 path 一起合成
	// 单一 trusted segment——模板里写：
	//   {{asset `/static/foo.min.css` .AppVersionLabel}}
	// 而不是：
	//   {{abs `/static/foo.min.css`}}?version={{.AppVersionLabel}}
	out["asset"] = func(p, version string) safehtml.TrustedResourceURL {
		full := p
		if basePath != "" && strings.HasPrefix(p, "/") {
			full = basePath + p
		}
		if version != "" {
			full += "?version=" + url.QueryEscape(version)
		}
		return uncheckedconversions.TrustedResourceURLFromStringKnownToSatisfyTypeContract(full)
	}
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
