// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/godoc"
)

func TestRenderDoc(t *testing.T) {
	src, err := os.ReadFile("testdata/pkg.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	pf, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	docPkg := godoc.NewPackage(fset, nil)
	docPkg.AddFile(pf, true)
	gpkg, err := docPkg.Encode(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := godoc.DecodePackage(gpkg)
	if err != nil {
		t.Fatal(err)
	}

	dpkg, err := decoded.DocPackage("p", &godoc.ModuleInfo{ModulePath: "p", ResolvedVersion: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	tr := &textRenderer{fset: decoded.Fset, w: &sb}
	if err := renderDoc(dpkg, tr); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(sb.String())
	wantBytes, err := os.ReadFile(filepath.FromSlash("testdata/text.golden"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(wantBytes))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
