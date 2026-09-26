// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/breakings"
)

func TestChanges(t *testing.T) {
	oldSrc := `
package p

const C = 1
var V int
func F(x int) bool { return true }
func G(x int) {}
type S struct {
	Field int
	OldField string
}
func (S) M() {}
type I interface {
	M1()
	Removed()
}
`

	newSrc := `
package p

const C = "hello"
var V string
func F(x string) bool { return true }
func G(x int, y ...int) {}
type S struct {
	Field string
}
func (*S) M() {}
type I interface {
	M1()
	Added()
}
`

	fset := token.NewFileSet()
	oldFile, err := parser.ParseFile(fset, "p.go", oldSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	newFile, err := parser.ParseFile(fset, "p.go", newSrc, 0)
	if err != nil {
		t.Fatal(err)
	}

	oldAPI, err := breakings.NewAPI("p", "v1.0.0", []*ast.File{oldFile})
	if err != nil {
		t.Fatal(err)
	}
	newAPI, err := breakings.NewAPI("p", "v1.1.0", []*ast.File{newFile})
	if err != nil {
		t.Fatal(err)
	}

	got := changes(oldAPI, newAPI)
	want := []string{
		"    C: breaking\n      old: const C untyped int\n      new: const C untyped string",
		"    F: breaking\n      old: func F(int) bool\n      new: func F(string) bool",
		"    G: call-compatible",
		"    I.Added: breaking\n      old: (none)\n      new: I.Added()",
		"    I.Removed: breaking\n      old: I.Removed()\n      new: (none)",
		"    S.Field: breaking\n      old: S.Field int\n      new: S.Field string",
		"    S.M: breaking\n      old: func (S) M()\n      new: func (*S) M()",
		"    S.OldField: breaking\n      old: S.OldField string\n      new: (none)",
		"    V: breaking\n      old: var V int\n      new: var V string",
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("changes mismatch (-want, +got):\n%s", diff)
	}

	t.Run("no breaking changes", func(t *testing.T) {
		ccOldFile, err := parser.ParseFile(fset, "p.go", `package p; func F(x int) {}`, 0)
		if err != nil {
			t.Fatal(err)
		}
		ccNewFile, err := parser.ParseFile(fset, "p.go", `package p; func F(x int, y ...int) {}`, 0)
		if err != nil {
			t.Fatal(err)
		}
		ccOldAPI, err := breakings.NewAPI("p", "v1.0.0", []*ast.File{ccOldFile})
		if err != nil {
			t.Fatal(err)
		}
		ccNewAPI, err := breakings.NewAPI("p", "v1.1.0", []*ast.File{ccNewFile})
		if err != nil {
			t.Fatal(err)
		}
		got := changes(ccOldAPI, ccNewAPI)
		if got != nil {
			t.Errorf("changes with only call-compatible changes = %v, want nil", got)
		}
	})
}

func TestIsInternal(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"example.com/m", false},
		{"example.com/m/internal", true},
		{"example.com/m/internal/p", true},
		{"example.com/m/a/internal/b", true},
		{"example.com/m/internals", false},
		{"example.com/m/notinternal/p", false},
	} {
		if got := isInternal(tc.path); got != tc.want {
			t.Errorf("isInternal(%q) = %t, want %t", tc.path, got, tc.want)
		}
	}
}

func TestPrintChanges(t *testing.T) {
	newPkg := func(path, version, src string) *pkg {
		t.Helper()
		f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		api, err := breakings.NewAPI(path, version, []*ast.File{f})
		if err != nil {
			t.Fatal(err)
		}
		return &pkg{path: path, version: version, api: api}
	}
	const (
		v1 = "v1.0.0"
		v2 = "v1.1.0"
	)
	prev := map[string]*pkg{
		"m/changed":   newPkg("m/changed", v1, "package changed; func F(int) {}"),
		"m/removed":   newPkg("m/removed", v1, "package removed; func F() {}"),
		"m/unchanged": newPkg("m/unchanged", v1, "package unchanged; func F() {}"),
	}
	curr := map[string]*pkg{
		"m/added":     newPkg("m/added", v2, "package added; func F() {}"),
		"m/changed":   newPkg("m/changed", v2, "package changed; func F(string) {}"),
		"m/unchanged": newPkg("m/unchanged", v2, "package unchanged; func F() {}"),
	}

	var buf bytes.Buffer
	if err := printChanges(&buf, v1, v2, prev, curr); err != nil {
		t.Fatal(err)
	}
	want := `v1.0.0 -> v1.1.0:
  m/changed:
    F: breaking
      old: func F(int)
      new: func F(string)
  m/removed: package removed
`
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}

	t.Run("no changes", func(t *testing.T) {
		var buf bytes.Buffer
		if err := printChanges(&buf, v1, v1, prev, prev); err != nil {
			t.Fatal(err)
		}
		if got := buf.String(); got != "" {
			t.Errorf("got %q, want no output", got)
		}
	})
}

func TestBreakablePackagesQuery(t *testing.T) {
	got := breakablePackagesQuery()
	want := `
		SELECT DISTINCT ON (p.path) p.path, d.source
		FROM modules m
		INNER JOIN units u ON u.module_id = m.id
		INNER JOIN paths p ON p.id = u.path_id
		INNER JOIN documentation d ON d.unit_id = u.id
		WHERE m.module_path = $1
		  AND m.version = $2
		  AND u.name != 'main'
		  AND d.source IS NOT NULL
		ORDER BY
			p.path,
			CASE
				WHEN d.goos = 'all' AND d.goarch = 'all' THEN 0
				WHEN d.goos = 'linux' AND d.goarch = 'amd64' THEN 1
				WHEN d.goos = 'windows' AND d.goarch = 'amd64' THEN 2
				WHEN d.goos = 'darwin' AND d.goarch = 'amd64' THEN 3
				WHEN d.goos = 'js' AND d.goarch = 'wasm' THEN 4
				ELSE 5
			END,
			d.goos,
			d.goarch
	`
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("breakablePackagesQuery mismatch (-want, +got):\n%s", diff)
	}
}
