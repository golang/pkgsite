// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package breakings

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
)

func TestTypeString(t *testing.T) {
	testCases := []struct {
		expr string
		want string
	}{
		{"int", "int"},
		{"string", "string"},
		{"[]byte", "[]byte"},
		{"*MyType", "*MyType"},
		{"[5]*int", "[5]*int"},
		{"map[string]int", "map[string]int"},
		{"chan int", "chan int"},
		{"<-chan int", "<-chan int"},
		{"func()", "func()"},
		{"func(int, int) bool", "func(int, int) bool"},
		{"func(a, b int) (c bool, _ int)", "func(int, int) (bool, int)"},
		{"struct{}", "struct{}"},
		{"struct{ X int }", "struct{X int}"},
		{"struct{ X, Y int }", "struct{X int; Y int}"},
		{"struct{ X, Y int; Z string }", "struct{X int; Y int; Z string}"},
		{"struct{ X int `json:\"x\"`; Y string }", "struct{X int `json:\"x\"`; Y string}"},
		{"struct{ T; U }", "struct{T; U}"},
		{"interface{}", "interface{}"},
		{"interface{ Close() error; Read([]byte) (int, error) }", "interface{Close() error; Read([]byte) (int, error)}"},
		{"interface{ Read([]byte) (int, error); Close() error }", "interface{Close() error; Read([]byte) (int, error)}"},
		{"interface{ io.Reader; Close() error }", "interface{Close() error; io.Reader}"},
		{"interface{ B(x, y int) bool; A() }", "interface{A(); B(int, int) bool}"},
		{"interface{ Close() error; interface{ Read([]byte) (int, error) } }", "interface{Close() error; Read([]byte) (int, error)}"},
		{"interface{ Close() error; interface{ Close() error } }", "interface{Close() error}"},
		{"*struct{ X, Y int }", "*struct{X int; Y int}"},
		{"[]struct{ X, Y int }", "[]struct{X int; Y int}"},
		{"[5]*struct{ X, Y int }", "[5]*struct{X int; Y int}"},
		{"map[string]struct{ X, Y int }", "map[string]struct{X int; Y int}"},
		{"chan struct{ X, Y int }", "chan struct{X int; Y int}"},
		{"<-chan func(a, b int) bool", "<-chan func(int, int) bool"},
		{"chan<- interface{ Read([]byte) (int, error); Close() error }", "chan<- interface{Close() error; Read([]byte) (int, error)}"},
		{"chan (<-chan int)", "chan (<-chan int)"},
		{"(int)", "(int)"},
		{"chan<- int", "chan<- int"},
		{"T[int]", "T[int]"},
		{"T[int, string]", "T[int, string]"},
		{"~int", "~int"},
		{"int | string", "int | string"},
		{"~int | ~string | ~float64", "~int | ~string | ~float64"},
		{"~struct{ X, Y int }", "~struct{X int; Y int}"},
		{"~struct{ X, Y int } | ~[]byte", "~struct{X int; Y int} | ~[]byte"},
	}

	for _, tc := range testCases {
		t.Run(tc.expr, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parser.ParseExpr(%q): %v", tc.expr, err)
			}
			got := typeString(expr)
			if got != tc.want {
				t.Errorf("typeString(%q) = %q, want %q", tc.expr, got, tc.want)
			}
		})
	}

	t.Run("generic func", func(t *testing.T) {
		testCases := []struct {
			decl string
			want string
		}{
			{
				"func _[T any, U ~int](x int, y U, z T) bool",
				"func[any, ~int](int, #1, #0) bool",
			},
			{
				"func _[T any, U any](m map[T][]U, ch <-chan T, f func(*T) U) (T, *U, error)",
				"func[any, any](map[#0][]#1, <-chan #0, func(*#0) #1) (#0, *#1, error)",
			},
			{
				"func _[T any](s struct{ a, b T })",
				"func[any](struct{a #0; b #0})",
			},
			{
				"func _[T any](s struct{ T })",
				"func[any](struct{#0})",
			},
			{
				"func _[S ~[]E, E any](s S, e E)",
				"func[~[]#1, any](#0, #1)",
			},
		}
		for _, tc := range testCases {
			prog := "package p\n" + tc.decl
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", prog, 0)
			if err != nil {
				t.Fatalf("parser.ParseFile(%q): %v", tc.decl, err)
			}
			fnType := f.Decls[0].(*ast.FuncDecl).Type
			got := typeString(fnType)
			if got != tc.want {
				t.Errorf("typeString() for %q = %q, want %q", tc.decl, got, tc.want)
			}
		}
	})

	t.Run("ast nodes", func(t *testing.T) {
		ellipsis := &ast.Ellipsis{Elt: &ast.Ident{Name: "int"}}
		if got, want := typeString(ellipsis), "...int"; got != want {
			t.Errorf("typeString(ellipsis) = %q, want %q", got, want)
		}

		chanRecv := &ast.ChanType{
			Dir: ast.SEND | ast.RECV,
			Value: &ast.ChanType{
				Dir:   ast.RECV,
				Value: &ast.Ident{Name: "int"},
			},
		}
		if got, want := typeString(chanRecv), "chan (<-chan int)"; got != want {
			t.Errorf("typeString(chanRecv) = %q, want %q", got, want)
		}
	})

	if got, want := typeString(nil), "?"; got != want {
		t.Errorf("typeString(nil) = %q, want %q", got, want)
	}
}

func TestDefs(t *testing.T) {
	ar, err := txtar.ParseFile(filepath.Join("testdata", "defs.txtar"))
	if err != nil {
		t.Fatal(err)
	}

	want, files := parseDefsTxtar(t, ar)

	d, err := newDefs(files)
	if err != nil {
		t.Fatal(err)
	}

	var gotTypes []string
	for name := range d.types {
		gotTypes = append(gotTypes, name)
	}
	slices.Sort(gotTypes)

	var gotMethods []string
	for typeName, fns := range d.methods {
		for _, fn := range fns {
			gotMethods = append(gotMethods, typeName+"."+fn.Name.Name)
		}
	}
	slices.Sort(gotMethods)

	got := fmt.Sprintf("types: %s\nmethods: %s\n", strings.Join(gotTypes, " "), strings.Join(gotMethods, " "))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	for _, name := range gotTypes {
		if ts := d.typeFor(name); ts == nil {
			t.Errorf("typeFor(%q) = nil, want non-nil", name)
		}
	}
	if ts := d.typeFor("NonExistent"); ts != nil {
		t.Errorf("typeFor(\"NonExistent\") = %v, want nil", ts)
	}
	var nilDefs *defs
	if ts := nilDefs.typeFor("A"); ts != nil {
		t.Errorf("nilDefs.typeFor(\"A\") = %v, want nil", ts)
	}

	if ms := d.methodsFor("A"); len(ms) != 2 {
		t.Errorf("methodsFor(\"A\") returned %d methods, want 2", len(ms))
	}
	if ms := d.methodsFor("NonExistent"); ms != nil {
		t.Errorf("methodsFor(\"NonExistent\") = %v, want nil", ms)
	}
	if ms := nilDefs.methodsFor("A"); ms != nil {
		t.Errorf("nilDefs.methodsFor(\"A\") = %v, want nil", ms)
	}
}

// parseDefsTxtar parses a txtar archive for TestDefs.
//
// The archive is expected to have the following file organization:
//   - A "want" file containing the expected types and methods:
//     types: <space-separated list of expected type names>
//     methods: <space-separated list of expected methods in Type.Method format>
//   - One or more Go source files (with names ending in ".go") representing
//     the package files to be parsed into ASTs.
func parseDefsTxtar(t *testing.T, ar *txtar.Archive) (string, []*ast.File) {
	t.Helper()
	var (
		want  string
		files []*ast.File
		fset  = token.NewFileSet()
	)
	for _, f := range ar.Files {
		switch {
		case f.Name == "want":
			want = string(f.Data)
		case strings.HasSuffix(f.Name, ".go"):
			file, err := parser.ParseFile(fset, f.Name, f.Data, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", f.Name, err)
			}
			files = append(files, file)
		}
	}
	if want == "" {
		t.Fatal("archive missing \"want\" file")
	}
	return want, files
}

func TestAPI(t *testing.T) {
	ar, err := txtar.ParseFile(filepath.Join("testdata", "api.txtar"))
	if err != nil {
		t.Fatal(err)
	}

	var (
		want  = map[string]string{} // symbol name to kind
		files []*ast.File
		fset  = token.NewFileSet()
	)

	for _, f := range ar.Files {
		switch {
		case f.Name == "want":
			f.Data = f.Data[:len(f.Data)-1] // drop final newline
			for line := range strings.SplitSeq(string(f.Data), "\n") {
				fields := strings.Fields(line)
				if len(fields) != 2 {
					t.Fatalf("bad want line %q: got %d fields, want 2", line, len(fields))
				}
				want[fields[1]] = fields[0]
			}
		case strings.HasSuffix(f.Name, ".go"):
			file, err := parser.ParseFile(fset, f.Name, f.Data, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", f.Name, err)
			}
			files = append(files, file)
		}
	}

	api, err := NewAPI("p", "v1.0.0", files)
	if err != nil {
		t.Fatal(err)
	}

	gotNames := symbolNames(api.symbols)
	wantNames := slices.Collect(maps.Keys(want))
	slices.Sort(wantNames)
	if !slices.Equal(gotNames, wantNames) {
		t.Errorf("API.symbols: got %v, want %v", gotNames, wantNames)
	}

	for name, wantKind := range want {
		if got := api.kinds[name].String(); got != wantKind {
			t.Errorf("API.kinds[%q] = %v, want %v", name, got, wantKind)
		}
	}
}
func TestAPIChanges(t *testing.T) {
	oldFiles, newFiles, want := parseCombinedTxtar(t, filepath.Join("testdata", "changes.txtar"))

	oldSet, err := NewAPI("p", "v1.0.0", oldFiles)
	if err != nil {
		t.Fatalf("newAPI(old): %v", err)
	}
	newSet, err := NewAPI("p", "v1.1.0", newFiles)
	if err != nil {
		t.Fatalf("newAPI(new): %v", err)
	}
	changes := oldSet.Changes(newSet)
	var got []string
	for k, v := range changes {
		got = append(got, fmt.Sprintf("%s: %v", k, v))
	}
	slices.Sort(got)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func parseCombinedTxtar(t *testing.T, filename string) (oldFiles, newFiles []*ast.File, want []string) {
	t.Helper()
	ar, err := txtar.ParseFile(filename)
	if err != nil {
		t.Fatalf("txtar.ParseFile(%q): %v", filename, err)
	}
	fset := token.NewFileSet()
	for _, f := range ar.Files {
		if !strings.HasSuffix(f.Name, ".go") {
			continue
		}
		var oldLines, newLines []string
		mode := "both"
		for line := range strings.SplitSeq(string(f.Data), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			// Directive lines begin "//LETTER".
			if strings.HasPrefix(fields[0], "//") && len(fields[0]) > 2 && unicode.IsLetter(rune(fields[0][2])) {
				switch fields[0] {
				case "//old":
					mode = "old"
				case "//new":
					mode = "new"
				case "//both":
					mode = "both"
				case "//breaking":
					for _, sym := range fields[1:] {
						want = append(want, fmt.Sprintf("%s: %v", sym, changeBreaking))
					}
				case "//cc":
					for _, sym := range fields[1:] {
						want = append(want, fmt.Sprintf("%s: %v", sym, changeCallCompatible))
					}
				default:
					t.Fatalf("unrecognized directive: %q", line)
				}
				continue
			}

			switch mode {
			case "old":
				oldLines = append(oldLines, line)
			case "new":
				newLines = append(newLines, line)
			case "both":
				oldLines = append(oldLines, line)
				newLines = append(newLines, line)
			}
		}
		oldFile, err := parser.ParseFile(fset, f.Name, strings.Join(oldLines, "\n"), 0)
		if err != nil {
			t.Fatalf("parser.ParseFile(%q) (old): %v", f.Name, err)
		}
		newFile, err := parser.ParseFile(fset, f.Name, strings.Join(newLines, "\n"), 0)
		if err != nil {
			t.Fatalf("parser.ParseFile(%q) (new): %v", f.Name, err)
		}
		oldFiles = append(oldFiles, oldFile)
		newFiles = append(newFiles, newFile)
	}
	slices.Sort(want)
	return oldFiles, newFiles, want
}
