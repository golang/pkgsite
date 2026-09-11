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
		return newType(expr)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", src, err)
	}
	switch d := f.Decls[0].(type) {
	case *ast.FuncDecl:
		return newType(d.Type)
	case *ast.GenDecl:
		return newType(d.Specs[0].(*ast.TypeSpec).Type)
	default:
		t.Fatalf("unknown decl %T", d)
		panic("unreachable")
	}
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
				got := tc.old.change(tc.new)
				if got != tc.want {
					t.Errorf("%s: change = %v, want %v", tc.name, got, tc.want)
				}
			})
		}

		// non-*simpleType should return changeBreaking
		ft := &funcType{}
		s := parseType(t, "int")
		if got := s.change(ft); got != changeBreaking {
			t.Errorf("expected s.change(funcType) to be changeBreaking, got %v", got)
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
				got := oldFT.change(newFT)
				if got != tc.want {
					t.Errorf("(%q).change(%q) = %v, want %v", tc.oldDecl, tc.newDecl, got, tc.want)
				}
			})
		}

		// non-*funcType should return changeBreaking
		st := newSimpleType(nil)
		oldFT := newFuncType(parseFuncType("func f()"))
		if got := oldFT.change(st); got != changeBreaking {
			t.Errorf("expected oldFT.change(simpleType) to be changeBreaking, got %v", got)
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
	oldSet := newSymbolSet("T")
	oldSet.symbols["Removed"] = newSimpleType(ast.NewIdent("int"))
	oldSet.symbols["Unchanged"] = newSimpleType(ast.NewIdent("string"))
	oldSet.symbols["ChangedBreaking"] = newSimpleType(ast.NewIdent("int"))
	oldSet.symbols["ChangedCompatible"] = parseType(t, "func ChangedCompatible(x int)")

	newSet := newSymbolSet("T")
	newSet.symbols["Unchanged"] = newSimpleType(ast.NewIdent("string"))
	newSet.symbols["ChangedBreaking"] = newSimpleType(ast.NewIdent("bool"))
	newSet.symbols["ChangedCompatible"] = parseType(t, "func ChangedCompatible(x int, y ...string)")
	newSet.symbols["Added"] = newSimpleType(ast.NewIdent("float64"))

	want := map[string]changeKind{
		"T.Removed":           changeBreaking,
		"T.ChangedBreaking":   changeBreaking,
		"T.ChangedCompatible": changeCallCompatible,
	}

	got := oldSet.changes(newSet)
	if !maps.Equal(got, want) {
		t.Errorf("oldSet.change(newSet) = %v, want %v", got, want)
	}
}

func TestInterfaceType(t *testing.T) {
	testCases := []struct {
		name string
		old  syntaxType
		new  syntaxType
		want changeKind
	}{
		{
			name: "empty to empty",
			old:  parseType(t, "interface{}"),
			new:  parseType(t, "interface{}"),
			want: changeOther,
		},
		{
			name: "same method",
			old:  parseType(t, "interface{ M() }"),
			new:  parseType(t, "interface{ M() }"),
			want: changeOther,
		},
		{
			name: "add exported method to interface without unexported method",
			old:  parseType(t, "interface{ M() }"),
			new:  parseType(t, "interface{ M(); Added() }"),
			want: changeBreaking,
		},
		{
			name: "add exported method to interface with unexported method",
			old:  parseType(t, "interface{ M(); m() }"),
			new:  parseType(t, "interface{ M(); Added(); m() }"),
			want: changeOther,
		},
		{
			name: "add unexported method to interface without unexported method",
			old:  parseType(t, "interface{ A() }"),
			new:  parseType(t, "interface{ A(); m() }"),
			want: changeBreaking,
		},
		{
			name: "add unexported method to interface with unexported method",
			old:  parseType(t, "interface{ A(); m() }"),
			new:  parseType(t, "interface{ A(); m(); m2() }"),
			want: changeOther,
		},
		{
			name: "call-compatible change to interface without unexported method",
			old:  parseType(t, "interface{ ChangedCompatible(x int) }"),
			new:  parseType(t, "interface{ ChangedCompatible(x int, y ...string) }"),
			want: changeBreaking,
		},
		{
			name: "call-compatible change to interface with unexported method",
			old:  parseType(t, "interface{ ChangedCompatible(x int); m() }"),
			new:  parseType(t, "interface{ ChangedCompatible(x int, y ...string); m() }"),
			want: changeCallCompatible,
		},
		{
			name: "remove method",
			old:  parseType(t, "interface{ M(); Removed() }"),
			new:  parseType(t, "interface{ M() }"),
			want: changeBreaking,
		},
		{
			name: "change method signature breaking",
			old:  parseType(t, "interface{ M(x int) }"),
			new:  parseType(t, "interface{ M(x string) }"),
			want: changeBreaking,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.old.change(tc.new)
			if got != tc.want {
				t.Errorf("%s: change = %v, want %v", tc.name, got, tc.want)
			}
		})
	}

	t.Run("non-*interfaceType returns changeBreaking", func(t *testing.T) {
		it := parseType(t, "interface{ M() }")
		st := newSimpleType(ast.NewIdent("int"))
		if got := it.change(st); got != changeBreaking {
			t.Errorf("it.change(simpleType) = %v, want %v", got, changeBreaking)
		}
	})
}

func TestStructType(t *testing.T) {
	testCases := []struct {
		name string
		old  string
		new  string
		want changeKind
	}{
		{
			name: "equal",
			old:  "struct{ A int }",
			new:  "struct{ A int }",
			want: changeOther,
		},
		{
			name: "empty to empty",
			old:  "struct{}",
			new:  "struct{}",
			want: changeOther,
		},
		{
			name: "field added (empty to A)",
			old:  "struct{}",
			new:  "struct{ A int }",
			want: changeOther,
		},
		{
			name: "field added (A to AB)",
			old:  "struct{ A int }",
			new:  "struct{ A int; B string }",
			want: changeOther,
		},
		{
			name: "field removed (AB to A)",
			old:  "struct{ A int; B string }",
			new:  "struct{ A int }",
			want: changeBreaking,
		},
		{
			name: "field removed (A to empty)",
			old:  "struct{ A int }",
			new:  "struct{}",
			want: changeBreaking,
		},
		{
			name: "field type changed (A int to A string)",
			old:  "struct{ A int }",
			new:  "struct{ A string }",
			want: changeBreaking,
		},
		{
			name: "field renamed / replaced (A to B)",
			old:  "struct{ A int }",
			new:  "struct{ B string }",
			want: changeBreaking,
		},
		{
			name: "field call-compatible changed",
			old:  "struct{ F func(x int) }",
			new:  "struct{ F func(x int, y ...string) }",
			want: changeBreaking,
		},
		{
			name: "field call-compatible and breaking changed",
			old:  "struct{ F func(x int); A int }",
			new:  "struct{ F func(x int, y ...string); A string }",
			want: changeBreaking,
		},
		{
			name: "unexported fields ignored",
			old:  "struct{ a int }",
			new:  "struct{ a string }",
			want: changeOther,
		},
		{
			name: "embedded field unchanged",
			old:  "struct{ T }",
			new:  "struct{ T }",
			want: changeOther,
		},
		{
			name: "embedded field added",
			old:  "struct{}",
			new:  "struct{ T }",
			want: changeOther,
		},
		{
			name: "embedded field removed",
			old:  "struct{ T }",
			new:  "struct{}",
			want: changeBreaking,
		},
		{
			name: "non-*structType",
			old:  "struct{ A int }",
			new:  "int",
			want: changeBreaking,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseType(t, tc.old).change(parseType(t, tc.new))
			if got != tc.want {
				t.Errorf("%s: change = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestEmbeddedFieldName(t *testing.T) {
	testCases := []struct {
		expr string
		want string
	}{
		// Identifiers
		{"T", "T"},
		{"t", "t"},
		{"_", "_"},

		// Qualified identifiers
		{"pkg.T", "T"},
		{"pkg.t", "t"},

		// Pointer types
		{"*T", "T"},
		{"*pkg.T", "T"},
		{"**T", "T"},

		// Generic types with single type argument
		{"T[int]", "T"},
		{"pkg.T[int]", "T"},
		{"*T[int]", "T"},
		{"*pkg.T[int]", "T"},
		{"T[pkg.U]", "T"},
		{"T[[]int]", "T"},
		{"T[*int]", "T"},
		{"T[map[string]int]", "T"},

		// Generic types with multiple type arguments
		{"T[int, string]", "T"},
		{"pkg.T[int, string]", "T"},
		{"*T[int, string]", "T"},
		{"*pkg.T[int, string]", "T"},
		{"T[K, V, any]", "T"},

		// Parenthesized types
		{"(T)", "T"},
		{"*(T)", "T"},
		{"(*T)", "T"},
		{"(pkg.T)", "T"},
		{"*(pkg.T)", "T"},
		{"(*pkg.T)", "T"},
		{"(T[int])", "T"},
		{"*(T[int])", "T"},
		{"(*T[int])", "T"},
		{"*(pkg.T[int, string])", "T"},
		{"(*pkg.T[int, string])", "T"},

		// Non-embedded types (should return "")
		{"[]int", ""},
		{"[10]int", ""},
		{"map[string]int", ""},
		{"chan int", ""},
		{"<-chan int", ""},
		{"chan<- int", ""},
		{"func()", ""},
		{"func(int) bool", ""},
		{"interface{}", ""},
		{"struct{}", ""},
		{"*[]int", ""},
		{"*[10]int", ""},
		{"123", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parser.ParseExpr(%q): %v", tc.expr, err)
			}
			got := embeddedFieldName(expr)
			if got != tc.want {
				t.Errorf("embeddedFieldName(%s) = %q, want %q", tc.expr, got, tc.want)
			}
		})
	}

	t.Run("nil", func(t *testing.T) {
		if got := embeddedFieldName(nil); got != "" {
			t.Errorf("embeddedFieldName(nil) = %q, want \"\"", got)
		}
	})
}
