// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package templates

import (
	"testing"

	"github.com/google/safehtml/template"
	"github.com/jba/templatecheck"
	"golang.org/x/pkgsite/internal/frontend"
	"golang.org/x/pkgsite/internal/frontend/page"
	"golang.org/x/pkgsite/internal/frontend/templates"
	"golang.org/x/pkgsite/internal/frontend/versions"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/static"
)

func TestCheckFrontendTemplates(t *testing.T) {
	// Perform additional checks on parsed templates.
	staticFS := template.TrustedFSFromEmbed(static.FS)
	templates, err := templates.ParsePageTemplates(staticFS)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		subs    []string
		typeval any
	}{
		{"badge", nil, frontend.BadgePage{}},
		// error.tmpl omitted because relies on an associated "message" template
		// that's parsed on demand; see renderErrorPage above.
		{"fetch", nil, page.ErrorPage{}},
		{"homepage", nil, frontend.Homepage{}},
		{"license-policy", nil, frontend.LicensePolicyPage{}},
		{"search", nil, frontend.SearchPage{}},
		{"search-help", nil, page.BasePage{}},
		{"unit/main", nil, frontend.UnitPage{}},
		{
			"unit/main",
			[]string{"unit-outline", "unit-readme", "unit-doc", "unit-files", "unit-directories"},
			frontend.MainDetails{},
		},
		{"unit/importedby", nil, frontend.UnitPage{}},
		{"unit/importedby", []string{"importedby"}, frontend.ImportedByDetails{}},
		{"unit/imports", nil, frontend.UnitPage{}},
		{"unit/imports", []string{"imports"}, frontend.ImportsDetails{}},
		{"unit/licenses", nil, frontend.UnitPage{}},
		{"unit/licenses", []string{"licenses"}, frontend.LicensesDetails{}},
		{"unit/versions", nil, frontend.UnitPage{}},
		{"unit/versions", []string{"versions"}, versions.VersionsDetails{}},
		{"vuln", nil, page.BasePage{}},
		{"vuln/list", nil, frontend.VulnListPage{}},
		{"vuln/entry", nil, frontend.VulnEntryPage{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			tm := templates[c.name]
			if tm == nil {
				t.Fatalf("no template %q", c.name)
			}
			if c.subs == nil {
				if err := templatecheck.CheckSafe(tm, c.typeval); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, n := range c.subs {
					s := tm.Lookup(n)
					if s == nil {
						t.Fatalf("no sub-template %q of %q", n, c.name)
					}
					if err := templatecheck.CheckSafe(s, c.typeval); err != nil {
						t.Fatalf("%s: %v", n, err)
					}
				}
			}
		})
	}
}

var templateFS = template.TrustedFSFromTrustedSource(template.TrustedSourceFromConstant("../../../static"))

func TestCheckDocHTMLTemplates(t *testing.T) {
	dochtml.LoadTemplates(templateFS)
	for _, tm := range dochtml.Templates() {
		if err := templatecheck.CheckSafe(tm, dochtml.TemplateData{}); err != nil {
			t.Fatal(err)
		}
	}
}
