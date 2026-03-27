// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"fmt"
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
	t.Run("examples", func(t *testing.T) {
		testRenderDoc(t, true)
	})
	t.Run("no examples", func(t *testing.T) {
		testRenderDoc(t, false)
	})
}

func testRenderDoc(t *testing.T, examples bool) {
	fset := token.NewFileSet()
	docPkg := godoc.NewPackage(fset, nil)
	for _, file := range []string{"pkg.go", "pkg_test.go"} {
		pf, err := parser.ParseFile(fset, filepath.Join("testdata", file), nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		docPkg.AddFile(pf, true)
	}
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
	check := func(t *testing.T, name string, r renderer) {
		sb.Reset()
		t.Run(name, func(t *testing.T) {
			if err := renderDoc(dpkg, r, examples); err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(sb.String())
			goldenName := name
			if examples {
				goldenName += "-examples"
			}
			goldenFile := filepath.Join("testdata", goldenName+".golden")
			wantBytes, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSpace(string(wantBytes))
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
				fmt.Println(got)
			}
		})
	}

	check(t, "text", &textRenderer{fset: decoded.Fset, w: &sb})
	check(t, "markdown", &markdownRenderer{fset: decoded.Fset, w: &sb})
	check(t, "html", &htmlRenderer{fset: decoded.Fset, w: &sb})
}
