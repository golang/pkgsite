// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

var templateSource = template.TrustedSourceFromConstant("../../content/static/html/doc")

func TestRender(t *testing.T) {
	dochtml.LoadTemplates(templateSource)
	ctx := context.Background()
	si := source.NewGitHubInfo(sample.ModulePath, "", "abcde")
	mi := &ModuleInfo{
		ModulePath:      sample.ModulePath,
		ResolvedVersion: sample.VersionString,
		ModulePackages:  nil,
	}

	// Use a Package created locally and without nodes removed as the desired doc.
	p, err := packageForDir(filepath.Join("testdata", "p"), false)
	if err != nil {
		t.Fatal(err)
	}

	wantSyn, wantImports, wantDoc, err := p.Render(ctx, "p", si, mi)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(wantDoc.String(), "return") {
		t.Fatal("doc rendered with function bodies")
	}

	check := func(p *Package) {
		t.Helper()
		gotSyn, gotImports, gotDoc, err := p.Render(ctx, "p", si, mi)
		if err != nil {
			t.Fatal(err)
		}
		if gotSyn != wantSyn {
			t.Errorf("synopsis: got %q, want %q", gotSyn, wantSyn)
		}
		if !cmp.Equal(gotImports, wantImports) {
			t.Errorf("imports: got %v, want %v", gotImports, wantImports)
		}
		if diff := cmp.Diff(wantDoc.String(), gotDoc.String()); diff != "" {
			t.Errorf("doc mismatch (-want, +got):\n%s", diff)
			t.Logf("---- want ----\n%s", wantDoc)
			t.Logf("---- got ----\n%s", gotDoc)
		}
	}

	// Verify that removing AST nodes doesn't change the doc.
	p, err = packageForDir(filepath.Join("testdata", "p"), true)
	if err != nil {
		t.Fatal(err)
	}
	check(p)

	// Verify that encoding then decoding generates the same doc.
	// We can't re-use p to encode because it's been rendered.
	p, err = packageForDir(filepath.Join("testdata", "p"), true)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := p.Encode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := DecodePackage(bytes)
	if err != nil {
		t.Fatal(err)
	}
	check(p2)
}
