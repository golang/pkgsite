// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breakings

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"testing"
)

// parseType parses src as a Go type or declaration and returns its syntaxType.
func parseType(t *testing.T, src string) syntaxType {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err == nil {
		return newType(expr, nil)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", src, err)
	}
	switch d := f.Decls[0].(type) {
	case *ast.FuncDecl:
		return newType(d.Type, nil)
	case *ast.GenDecl:
		return newType(d.Specs[0].(*ast.TypeSpec).Type, nil)
	default:
		t.Fatalf("unknown decl %T", d)
		panic("unreachable")
	}
}

// maxChange returns the maximal change between two syntaxTypes.
func maxChange(old, new syntaxType) changeKind {
	res := changeOther
	for _, kind := range old.changes(new) {
		if kind == changeBreaking {
			return changeBreaking
		}
		if kind == changeCallCompatible {
			res = changeCallCompatible
		}
	}
	return res
}

func TestSimpleType(t *testing.T) {
	testCases := []struct {
		name string
		old  syntaxType
		new  syntaxType
		want changeKind
	}{
		{
			name: "same",
			old:  parseType(t, "int"),
			new:  parseType(t, "int"),
			want: changeOther,
		},
		{
			name: "different",
			old:  parseType(t, "int"),
			new:  parseType(t, "string"),
			want: changeBreaking,
		},
		{
			name: "both empty",
			old:  newSimpleType(nil),
			new:  newSimpleType(nil),
			want: changeOther,
		},
	}

	t.Run("change", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				got := maxChange(tc.old, tc.new)
				if got != tc.want {
					t.Errorf("%s: change = %v, want %v", tc.name, got, tc.want)
				}
			})
		}

		// non-*simpleType should return changeBreaking
		ft := &funcType{}
		s := parseType(t, "int")
		if got := maxChange(s, ft); got != changeBreaking {
			t.Errorf("expected change(s, funcType) to be changeBreaking, got %v", got)
		}
	})
}

func TestFuncType(t *testing.T) {
	parseFuncType := func(src string) *ast.FuncType {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", "package p\n"+src, 0)
		if err != nil {
			t.Fatalf("parser.ParseFile(%q): %v", src, err)
		}
		return f.Decls[0].(*ast.FuncDecl).Type
	}

	testCases := []struct {
		name           string
		decl           string
		wantTypeParams []string
		wantParams     []string
		wantResults    []string
		wantVariadic   bool
	}{
		{
			name:           "empty",
			decl:           "func f()",
			wantTypeParams: nil,
			wantParams:     nil,
			wantResults:    nil,
			wantVariadic:   false,
		},
		{
			name:           "basic",
			decl:           "func f(a, b int) (bool, error)",
			wantTypeParams: nil,
			wantParams:     []string{"int", "int"},
			wantResults:    []string{"bool", "error"},
			wantVariadic:   false,
		},
		{
			name:           "variadic",
			decl:           "func f(a int, b ...string)",
			wantTypeParams: nil,
			wantParams:     []string{"int", "...string"},
			wantResults:    nil,
			wantVariadic:   true,
		},
		{
			name:           "generic",
			decl:           "func f[T any, U ~int](x T, y ...U) T",
			wantTypeParams: []string{"any", "~int"},
			wantParams:     []string{"#0", "...#1"},
			wantResults:    []string{"#0"},
			wantVariadic:   true,
		},
		{
			name:           "blank and named",
			decl:           "func f(_, _ int) (err error)",
			wantTypeParams: nil,
			wantParams:     []string{"int", "int"},
			wantResults:    []string{"error"},
			wantVariadic:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ft := newFuncType(parseFuncType(tc.decl))
			if !slices.Equal(ft.typeParams, tc.wantTypeParams) {
				t.Errorf("newFuncType(%q).typeParams = %v, want %v", tc.decl, ft.typeParams, tc.wantTypeParams)
			}
			if !slices.Equal(ft.params, tc.wantParams) {
				t.Errorf("newFuncType(%q).params = %v, want %v", tc.decl, ft.params, tc.wantParams)
			}
			if !slices.Equal(ft.results, tc.wantResults) {
				t.Errorf("newFuncType(%q).results = %v, want %v", tc.decl, ft.results, tc.wantResults)
			}
			if ft.variadic != tc.wantVariadic {
				t.Errorf("newFuncType(%q).variadic = %v, want %v", tc.decl, ft.variadic, tc.wantVariadic)
			}
		})
	}

	t.Run("equal", func(t *testing.T) {
		ft1 := newFuncType(parseFuncType("func f(a, b int) bool"))
		ft2 := newFuncType(parseFuncType("func g(x, y int) bool"))
		ft3 := newFuncType(parseFuncType("func h(x int, y string) bool"))

		if !ft1.equal(ft2) {
			t.Errorf("expected ft1 (%+v) and ft2 (%+v) to be equal", ft1, ft2)
		}
		if ft1.equal(ft3) {
			t.Errorf("expected ft1 (%+v) and ft3 (%+v) to NOT be equal", ft1, ft3)
		}

		// different typeParams should not be equal
		ftDiffTypeParams := &funcType{typeParams: []string{"any"}, params: ft1.params, results: ft1.results}
		if ft1.equal(ftDiffTypeParams) {
			t.Errorf("expected ft1 (%+v) and ftDiffTypeParams (%+v) to NOT be equal", ft1, ftDiffTypeParams)
		}

		// non-*funcType should return false
		st := newSimpleType(nil)
		if ft1.equal(st) {
			t.Errorf("expected ft1.equal(simpleType) to be false")
		}
	})

	t.Run("change", func(t *testing.T) {
		testCases := []struct {
			name    string
			oldDecl string
			newDecl string
			want    changeKind
		}{
			{
				name:    "same",
				oldDecl: "func f(a int, b string) bool",
				newDecl: "func f(a int, b string) bool",
				want:    changeOther,
			},
			{
				name:    "same variadic",
				oldDecl: "func f(a int, b ...string) bool",
				newDecl: "func f(a int, b ...string) bool",
				want:    changeOther,
			},
			{
				name:    "add variadic",
				oldDecl: "func f(a int, b string) bool",
				newDecl: "func f(a int, b string, c ...bool) bool",
				want:    changeCallCompatible,
			},
			{
				name:    "empty to variadic",
				oldDecl: "func f()",
				newDecl: "func f(x ...int)",
				want:    changeCallCompatible,
			},
			{
				name:    "generic add variadic",
				oldDecl: "func f[T any](x T)",
				newDecl: "func f[T any](x T, y ...int)",
				want:    changeCallCompatible,
			},
			{
				name:    "change to variadic",
				oldDecl: "func f(a int, b string) bool",
				newDecl: "func f(a int, b ...string) bool",
				want:    changeBreaking,
			},
			{
				name:    "change from variadic",
				oldDecl: "func f(a int, b ...string) bool",
				newDecl: "func f(a int, b string) bool",
				want:    changeBreaking,
			},
			{
				name:    "add param",
				oldDecl: "func f(a int) bool",
				newDecl: "func f(a int, b int) bool",
				want:    changeBreaking,
			},
			{
				name:    "change result",
				oldDecl: "func f(a int) bool",
				newDecl: "func f(a int) string",
				want:    changeBreaking,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				oldFT := newFuncType(parseFuncType(tc.oldDecl))
				newFT := newFuncType(parseFuncType(tc.newDecl))
				got := maxChange(oldFT, newFT)
				if got != tc.want {
					t.Errorf("(%q).change(%q) = %v, want %v", tc.oldDecl, tc.newDecl, got, tc.want)
				}
			})
		}

		// non-*funcType should return changeBreaking
		st := newSimpleType(nil)
		oldFT := newFuncType(parseFuncType("func f()"))
		if got := maxChange(oldFT, st); got != changeBreaking {
			t.Errorf("expected change(oldFT, simpleType) to be changeBreaking, got %v", got)
		}
	})
}

func TestNewSimpleType(t *testing.T) {
	expr, err := parser.ParseExpr("int")
	if err != nil {
		t.Fatalf("parser.ParseExpr: %v", err)
	}
	st := newSimpleType(expr)
	if got, want := st.typeString, "int"; got != want {
		t.Errorf("newSimpleType: got %q, want %q", got, want)
	}
}

func TestSymbolSetChange(t *testing.T) {
	oldSet := newSymbolSet()
	oldSet.symbols["Removed"] = newSimpleType(ast.NewIdent("int"))
	oldSet.symbols["Unchanged"] = newSimpleType(ast.NewIdent("string"))
	oldSet.symbols["ChangedBreaking"] = newSimpleType(ast.NewIdent("int"))
	oldSet.symbols["ChangedCompatible"] = parseType(t, "func ChangedCompatible(x int)")

	newSet := newSymbolSet()
	newSet.symbols["Unchanged"] = newSimpleType(ast.NewIdent("string"))
	newSet.symbols["ChangedBreaking"] = newSimpleType(ast.NewIdent("bool"))
	newSet.symbols["ChangedCompatible"] = parseType(t, "func ChangedCompatible(x int, y ...string)")
	newSet.symbols["Added"] = newSimpleType(ast.NewIdent("float64"))

	want := map[string]changeKind{
		"Removed":           changeBreaking,
		"ChangedBreaking":   changeBreaking,
		"ChangedCompatible": changeCallCompatible,
	}

	got := maps.Collect(oldSet.changes(newSet))
	if !maps.Equal(got, want) {
		t.Errorf("oldSet.changes(newSet) = %v, want %v", got, want)
	}
}

func TestInterfaceType(t *testing.T) {
	testCases := []struct {
		name string
		old  syntaxType
		new  syntaxType
		want map[string]changeKind
	}{
		{
			name: "empty to empty",
			old:  parseType(t, "interface{}"),
			new:  parseType(t, "interface{}"),
			want: nil,
		},
		{
			name: "same method",
			old:  parseType(t, "interface{ M() }"),
			new:  parseType(t, "interface{ M() }"),
			want: nil,
		},
		{
			name: "add exported method to interface without unexported method",
			old:  parseType(t, "interface{ M() }"),
			new:  parseType(t, "interface{ M(); Added() }"),
			want: map[string]changeKind{"Added": changeBreaking},
		},
		{
			name: "add exported method to interface with unexported method",
			old:  parseType(t, "interface{ M(); m() }"),
			new:  parseType(t, "interface{ M(); Added(); m() }"),
			want: nil,
		},
		{
			name: "add unexported method to interface without unexported method",
			old:  parseType(t, "interface{ A() }"),
			new:  parseType(t, "interface{ A(); m() }"),
			want: map[string]changeKind{"m": changeBreaking},
		},
		{
			name: "add unexported method to interface with unexported method",
			old:  parseType(t, "interface{ A(); m() }"),
			new:  parseType(t, "interface{ A(); m(); m2() }"),
			want: nil,
		},
		{
			name: "call-compatible change to interface without unexported method",
			old:  parseType(t, "interface{ ChangedCompatible(x int) }"),
			new:  parseType(t, "interface{ ChangedCompatible(x int, y ...string) }"),
			want: map[string]changeKind{"ChangedCompatible": changeBreaking},
		},
		{
			name: "call-compatible change to interface with unexported method",
			old:  parseType(t, "interface{ ChangedCompatible(x int); m() }"),
			new:  parseType(t, "interface{ ChangedCompatible(x int, y ...string); m() }"),
			want: map[string]changeKind{"ChangedCompatible": changeCallCompatible},
		},
		{
			name: "remove method",
			old:  parseType(t, "interface{ M(); Removed() }"),
			new:  parseType(t, "interface{ M() }"),
			want: map[string]changeKind{"Removed": changeBreaking},
		},
		{
			name: "change method signature breaking",
			old:  parseType(t, "interface{ M(x int) }"),
			new:  parseType(t, "interface{ M(x string) }"),
			want: map[string]changeKind{"M": changeBreaking},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := maps.Collect(tc.old.changes(tc.new))
			if !maps.Equal(got, tc.want) {
				t.Errorf("%s: changes = %v, want %v", tc.name, got, tc.want)
			}
		})
	}

	t.Run("non-*interfaceType returns changeBreaking", func(t *testing.T) {
		it := parseType(t, "interface{ M() }")
		st := newSimpleType(ast.NewIdent("int"))
		got := maps.Collect(it.changes(st))
		want := map[string]changeKind{"": changeBreaking}
		if !maps.Equal(got, want) {
			t.Errorf("changes(it, simpleType) = %v, want %v", got, want)
		}
	})
}

func TestStructType(t *testing.T) {
	testCases := []struct {
		name    string
		src     string
		wantTop []string
		wantSel []string
	}{
		{
			name:    "empty",
			src:     `type S struct{}`,
			wantTop: nil,
			wantSel: nil,
		},
		{
			name:    "top-level exported fields",
			src:     `type S struct { A int; B string }`,
			wantTop: []string{"A", "B"},
			wantSel: []string{"A", "B"},
		},
		{
			name:    "unexported fields ignored",
			src:     `type S struct { a int; B string; c bool }`,
			wantTop: []string{"B"},
			wantSel: []string{"B"},
		},
		{
			name:    "multiple names in single field",
			src:     `type S struct { A, B int; c, D string }`,
			wantTop: []string{"A", "B", "D"},
			wantSel: []string{"A", "B", "D"},
		},
		{
			name: "simple embedded struct",
			src: `
type E struct { X int; y bool }
type S struct { E; A string }`,
			wantTop: []string{"A", "E"},
			wantSel: []string{"A", "E", "X"},
		},
		{
			name: "embedded unexported struct with exported fields",
			src: `
type e struct { X int }
type S struct { e; A string }`,
			wantTop: []string{"A"},
			wantSel: []string{"A", "X"},
		},
		{
			name: "embedded non-struct type",
			src: `
type MyInt int
type S struct { MyInt; A string }`,
			wantTop: []string{"A", "MyInt"},
			wantSel: []string{"A", "MyInt"},
		},
		{
			name:    "embedded external package type",
			src:     `type S struct { pkg.T; A string }`,
			wantTop: []string{"A", "T"},
			wantSel: []string{"A", "T"},
		},
		{
			name: "shadowing depth 0 over depth 1",
			src: `
type E struct { A int; B int }
type S struct { A string; E }`,
			wantTop: []string{"A", "E"},
			wantSel: []string{"A", "B", "E"},
		},
		{
			name: "shadowing depth 1 over depth 2",
			src: `
type Inner struct { X int; Y int }
type Mid struct { Inner; X string }
type S struct { Mid }`,
			wantTop: []string{"Mid"},
			wantSel: []string{"Inner", "Mid", "X", "Y"},
		},
		{
			name: "duplicate at depth 1",
			src: `
type E1 struct { A int; B int }
type E2 struct { B int; C int }
type S struct { E1; E2 }`,
			wantTop: []string{"E1", "E2"},
			wantSel: []string{"A", "C", "E1", "E2"},
		},
		{
			name: "duplicate at depth 1 invalidates unique at depth 2",
			src: `
type E1 struct { X int }
type E2 struct { X int }
type Inner struct { X int; Y int }
type Mid struct { Inner }
type S struct { E1; E2; Mid }`,
			wantTop: []string{"E1", "E2", "Mid"},
			wantSel: []string{"E1", "E2", "Inner", "Mid", "Y"},
		},
		{
			name: "duplicate at depth 1 does not invalidate depth 0",
			src: `
type E1 struct { A int }
type E2 struct { A int }
type S struct { A string; E1; E2 }`,
			wantTop: []string{"A", "E1", "E2"},
			wantSel: []string{"A", "E1", "E2"},
		},
		{
			name: "duplicate at depth 2",
			src: `
type Inner1 struct { X int }
type Inner2 struct { X int }
type Mid1 struct { Inner1 }
type Mid2 struct { Inner2 }
type S struct { Mid1; Mid2 }`,
			wantTop: []string{"Mid1", "Mid2"},
			wantSel: []string{"Inner1", "Inner2", "Mid1", "Mid2"},
		},
		{
			name: "diamond embedding",
			src: `
type Base struct { A int; B int }
type D1 struct { Base; C int }
type D2 struct { Base; D int }
type S struct { D1; D2 }`,
			wantTop: []string{"D1", "D2"},
			wantSel: []string{"C", "D", "D1", "D2"},
		},
		{
			name: "cycle",
			src: `
type A struct { *B }
type B struct { *A }
type S struct { A }`,
			wantTop: []string{"A"},
			wantSel: []string{"A", "B"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", "package p\n"+tc.src, 0)
			if err != nil {
				t.Fatalf("parser.ParseFile: %v", err)
			}
			defs, err := newDefs([]*ast.File{f})
			if err != nil {
				t.Fatalf("newDefs: %v", err)
			}
			spec, ok := defs.types["S"]
			if !ok {
				t.Fatalf("type S not found")
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("type S is not a struct: %T", spec.Type)
			}
			got := newStructType(st, defs)
			gotTop := symbolNames(got.topLevelFields)
			gotSel := symbolNames(got.selectableFields)
			if !slices.Equal(gotTop, tc.wantTop) {
				t.Errorf("topLevelFields = %v, want %v", gotTop, tc.wantTop)
			}
			if !slices.Equal(gotSel, tc.wantSel) {
				t.Errorf("selectableFields = %v, want %v", gotSel, tc.wantSel)
			}
		})
	}
}

func symbolNames(s *symbolSet) []string {
	if s == nil || len(s.symbols) == 0 {
		return nil
	}
	var names []string
	for name := range s.symbols {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func TestStructTypeChange(t *testing.T) {
	testCases := []struct {
		name string
		old  string
		new  string
		want map[string]changeKind
	}{
		{
			name: "equal",
			old:  "struct{ A int }",
			new:  "struct{ A int }",
			want: nil,
		},
		{
			name: "empty to empty",
			old:  "struct{}",
			new:  "struct{}",
			want: nil,
		},
		{
			name: "field added (empty to A)",
			old:  "struct{}",
			new:  "struct{ A int }",
			want: nil,
		},
		{
			name: "field added (A to AB)",
			old:  "struct{ A int }",
			new:  "struct{ A int; B string }",
			want: nil,
		},
		{
			name: "field removed (AB to A)",
			old:  "struct{ A int; B string }",
			new:  "struct{ A int }",
			want: map[string]changeKind{"B": changeBreaking},
		},
		{
			name: "field removed (A to empty)",
			old:  "struct{ A int }",
			new:  "struct{}",
			want: map[string]changeKind{"A": changeBreaking},
		},
		{
			name: "field type changed (A int to A string)",
			old:  "struct{ A int }",
			new:  "struct{ A string }",
			want: map[string]changeKind{"A": changeBreaking},
		},
		{
			name: "field renamed / replaced (A to B)",
			old:  "struct{ A int }",
			new:  "struct{ B string }",
			want: map[string]changeKind{"A": changeBreaking},
		},
		{
			name: "field call-compatible changed",
			old:  "struct{ F func(x int) }",
			new:  "struct{ F func(x int, y ...string) }",
			want: map[string]changeKind{"F": changeBreaking},
		},
		{
			name: "field call-compatible and breaking changed",
			old:  "struct{ F func(x int); A int }",
			new:  "struct{ F func(x int, y ...string); A string }",
			want: map[string]changeKind{"A": changeBreaking, "F": changeBreaking},
		},
		{
			name: "unexported fields ignored",
			old:  "struct{ a int }",
			new:  "struct{ a string }",
			want: nil,
		},
		{
			name: "embedded field unchanged",
			old:  "struct{ T }",
			new:  "struct{ T }",
			want: nil,
		},
		{
			name: "embedded field added",
			old:  "struct{}",
			new:  "struct{ T }",
			want: nil,
		},
		{
			name: "embedded field removed",
			old:  "struct{ T }",
			new:  "struct{}",
			want: map[string]changeKind{"T": changeBreaking},
		},
		{
			name: "non-*structType",
			old:  "struct{ A int }",
			new:  "int",
			want: map[string]changeKind{"": changeBreaking},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := maps.Collect(parseType(t, tc.old).changes(parseType(t, tc.new)))
			if !maps.Equal(got, tc.want) {
				t.Errorf("%s: changes = %v, want %v", tc.name, got, tc.want)
			}
		})
	}

	t.Run("selectable fields", func(t *testing.T) {
		testCases := []struct {
			name string
			old  string
			new  string
			want map[string]changeKind
		}{
			{
				name: "embedded struct field removed",
				old: `
type E struct { X int }
type S struct { E }
`,
				new: `
type E struct {}
type S struct { E }
`,
				want: map[string]changeKind{"X": changeBreaking},
			},
			{
				name: "embedded struct field type changed",
				old: `
type E struct { X int }
type S struct { E }
`,
				new: `
type E struct { X string }
type S struct { E }
`,
				want: map[string]changeKind{"X": changeBreaking},
			},
			{
				name: "unexported embedded struct field removed",
				old: `
type e struct { X int }
type S struct { e }
`,
				new: `
type e struct {}
type S struct { e }
`,
				want: map[string]changeKind{"X": changeBreaking},
			},
			{
				name: "embedded struct field becomes ambiguous",
				old: `
type E1 struct { X int }
type S struct { E1 }
`,
				new: `
type E1 struct { X int }
type E2 struct { X int }
type S struct { E1; E2 }
`,
				want: map[string]changeKind{"X": changeBreaking},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				oldST := parseStructType(t, tc.old, "S")
				newST := parseStructType(t, tc.new, "S")
				got := maps.Collect(oldST.changes(newST))
				if !maps.Equal(got, tc.want) {
					t.Errorf("%s: changes = %v, want %v", tc.name, got, tc.want)
				}
			})
		}
	})
}

func parseStructType(t *testing.T, src, typeName string) *structType {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parser.ParseFile: %v", err)
	}
	defs, err := newDefs([]*ast.File{f})
	if err != nil {
		t.Fatalf("newDefs: %v", err)
	}
	spec, ok := defs.types[typeName]
	if !ok {
		t.Fatalf("type %s not found", typeName)
	}
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		t.Fatalf("type %s is not a struct: %T", typeName, spec.Type)
	}
	return newStructType(st, defs)
}

func TestBaseTypeName(t *testing.T) {
	testCases := []struct {
		expr    string
		want    string
		wantSel bool
	}{
		// Identifiers
		{"T", "T", false},
		{"t", "t", false},
		{"_", "_", false},

		// Qualified identifiers
		{"pkg.T", "T", true},
		{"pkg.t", "t", true},

		// Pointer types
		{"*T", "T", false},
		{"*pkg.T", "T", true},
		{"**T", "T", false},

		// Generic types with single type argument
		{"T[int]", "T", false},
		{"pkg.T[int]", "T", true},
		{"*T[int]", "T", false},
		{"*pkg.T[int]", "T", true},
		{"T[pkg.U]", "T", false},
		{"T[[]int]", "T", false},
		{"T[*int]", "T", false},
		{"T[map[string]int]", "T", false},

		// Generic types with multiple type arguments
		{"T[int, string]", "T", false},
		{"pkg.T[int, string]", "T", true},
		{"*T[int, string]", "T", false},
		{"*pkg.T[int, string]", "T", true},
		{"T[K, V, any]", "T", false},

		// Parenthesized types
		{"(T)", "T", false},
		{"*(T)", "T", false},
		{"(*T)", "T", false},
		{"(pkg.T)", "T", true},
		{"*(pkg.T)", "T", true},
		{"(*pkg.T)", "T", true},
		{"(T[int])", "T", false},
		{"*(T[int])", "T", false},
		{"(*T[int])", "T", false},
		{"*(pkg.T[int, string])", "T", true},
		{"(*pkg.T[int, string])", "T", true},

		// Non-embedded types (should return "")
		{"[]int", "", false},
		{"[10]int", "", false},
		{"map[string]int", "", false},
		{"chan int", "", false},
		{"<-chan int", "", false},
		{"chan<- int", "", false},
		{"func()", "", false},
		{"func(int) bool", "", false},
		{"interface{}", "", false},
		{"struct{}", "", false},
		{"*[]int", "", false},
		{"*[10]int", "", false},
		{"123", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parser.ParseExpr(%q): %v", tc.expr, err)
			}
			got, gotSel := baseTypeName(expr)
			if got != tc.want || gotSel != tc.wantSel {
				t.Errorf("baseTypeName(%s) = (%q, %t), want (%q, %t)", tc.expr, got, gotSel, tc.want, tc.wantSel)
			}
		})
	}

	t.Run("nil", func(t *testing.T) {
		if got, gotSel := baseTypeName(nil); got != "" || gotSel {
			t.Errorf("baseTypeName(nil) = (%q, %t), want (\"\", false)", got, gotSel)
		}
	})
}
