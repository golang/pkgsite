// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doc_test

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/godoc/internal/doc"
)

func TestExamples(t *testing.T) {
	dir := filepath.Join("testdata", "examples")
	filenames, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, filename := range filenames {
		t.Run(strings.TrimSuffix(filepath.Base(filename), ".go"), func(t *testing.T) {
			fset := token.NewFileSet()
			astFile, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			goldenFilename := strings.TrimSuffix(filename, ".go") + ".golden"
			golden, err := readSectionFile(goldenFilename)
			if err != nil {
				t.Fatal(err)
			}
			examples := map[string]*doc.Example{}
			unseen := map[string]bool{} // examples we haven't seen yet
			for _, e := range doc.Examples(fset, astFile) {
				examples[e.Name] = e
				unseen[e.Name] = true
			}
			for section, want := range golden {
				words := strings.Split(section, ".")
				if len(words) != 2 {
					t.Fatalf("bad section name %q", section)
				}
				name, kind := words[0], words[1]
				ex := examples[name]
				if ex == nil {
					t.Fatalf("no example named %q", name)
				}
				switch kind {
				case "Play":
					got := strings.TrimSpace(formatFile(t, fset, ex.Play))
					if diff := cmp.Diff(want, got); diff != "" {
						t.Errorf("mismatch (-want, +got):\n%s", diff)
					}
					delete(unseen, name)
				case "Output":
					got := strings.TrimSpace(ex.Output)
					if got != want {
						t.Errorf("%s Output: got\n%q\n---- want ----\n%q", ex.Name, got, want)
					}
				default:
					t.Fatalf("bad section kind %q", kind)
				}
			}
			for name := range unseen {
				t.Errorf("no Play golden for example %q", name)
			}
		})
	}
}

func formatFile(t *testing.T, fset *token.FileSet, n *ast.File) string {
	t.Helper()
	if n == nil {
		return "<nil>"
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, n); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// This example illustrates how to use NewFromFiles
// to compute package documentation with examples.
func ExampleNewFromFiles() {
	// src and test are two source files that make up
	// a package whose documentation will be computed.
	const src = `
// This is the package comment.
package p

import "fmt"

// This comment is associated with the Greet function.
func Greet(who string) {
	fmt.Printf("Hello, %s!\n", who)
}
`
	const test = `
package p_test

// This comment is associated with the ExampleGreet_world example.
func ExampleGreet_world() {
	Greet("world")
}
`

	// Create the AST by parsing src and test.
	fset := token.NewFileSet()
	files := []*ast.File{
		mustParse(fset, "src.go", src),
		mustParse(fset, "src_test.go", test),
	}

	// Compute package documentation with examples.
	p, err := doc.NewFromFiles(fset, files, "example.com/p")
	if err != nil {
		panic(err)
	}

	fmt.Printf("package %s - %s", p.Name, p.Doc)
	fmt.Printf("func %s - %s", p.Funcs[0].Name, p.Funcs[0].Doc)
	fmt.Printf(" ⤷ example with suffix %q - %s", p.Funcs[0].Examples[0].Suffix, p.Funcs[0].Examples[0].Doc)

	// Output:
	// package p - This is the package comment.
	// func Greet - This comment is associated with the Greet function.
	//  ⤷ example with suffix "world" - This comment is associated with the ExampleGreet_world example.
}

func TestClassifyExamples(t *testing.T) {
	const src = `
package p

const Const1 = 0
var   Var1   = 0

type (
	Type1     int
	Type1_Foo int
	Type1_foo int
	type2     int

	Embed struct { Type1 }
	Uembed struct { type2 }
)

func Func1()     {}
func Func1_Foo() {}
func Func1_foo() {}
func func2()     {}

func (Type1) Func1() {}
func (Type1) Func1_Foo() {}
func (Type1) Func1_foo() {}
func (Type1) func2() {}

func (type2) Func1() {}

type (
	Conflict          int
	Conflict_Conflict int
	Conflict_conflict int
)

func (Conflict) Conflict() {}
`
	const test = `
package p_test

func ExampleConst1() {} // invalid - no support for consts and vars
func ExampleVar1()   {} // invalid - no support for consts and vars

func Example()               {}
func Example_()              {} // invalid - suffix must start with a lower-case letter
func Example_suffix()        {}
func Example_suffix_xX_X_x() {}
func Example_世界()           {} // invalid - suffix must start with a lower-case letter
func Example_123()           {} // invalid - suffix must start with a lower-case letter
func Example_BadSuffix()     {} // invalid - suffix must start with a lower-case letter

func ExampleType1()               {}
func ExampleType1_()              {} // invalid - suffix must start with a lower-case letter
func ExampleType1_suffix()        {}
func ExampleType1_BadSuffix()     {} // invalid - suffix must start with a lower-case letter
func ExampleType1_Foo()           {}
func ExampleType1_Foo_suffix()    {}
func ExampleType1_Foo_BadSuffix() {} // invalid - suffix must start with a lower-case letter
func ExampleType1_foo()           {}
func ExampleType1_foo_suffix()    {}
func ExampleType1_foo_Suffix()    {} // matches Type1, instead of Type1_foo
func Exampletype2()               {} // invalid - cannot match unexported

func ExampleFunc1()               {}
func ExampleFunc1_()              {} // invalid - suffix must start with a lower-case letter
func ExampleFunc1_suffix()        {}
func ExampleFunc1_BadSuffix()     {} // invalid - suffix must start with a lower-case letter
func ExampleFunc1_Foo()           {}
func ExampleFunc1_Foo_suffix()    {}
func ExampleFunc1_Foo_BadSuffix() {} // invalid - suffix must start with a lower-case letter
func ExampleFunc1_foo()           {}
func ExampleFunc1_foo_suffix()    {}
func ExampleFunc1_foo_Suffix()    {} // matches Func1, instead of Func1_foo
func Examplefunc1()               {} // invalid - cannot match unexported

func ExampleType1_Func1()               {}
func ExampleType1_Func1_()              {} // invalid - suffix must start with a lower-case letter
func ExampleType1_Func1_suffix()        {}
func ExampleType1_Func1_BadSuffix()     {} // invalid - suffix must start with a lower-case letter
func ExampleType1_Func1_Foo()           {}
func ExampleType1_Func1_Foo_suffix()    {}
func ExampleType1_Func1_Foo_BadSuffix() {} // invalid - suffix must start with a lower-case letter
func ExampleType1_Func1_foo()           {}
func ExampleType1_Func1_foo_suffix()    {}
func ExampleType1_Func1_foo_Suffix()    {} // matches Type1.Func1, instead of Type1.Func1_foo
func ExampleType1_func2()               {} // matches Type1, instead of Type1.func2

func ExampleEmbed_Func1()         {} // invalid - no support for forwarded methods from embedding exported type
func ExampleUembed_Func1()        {} // methods from embedding unexported types are OK
func ExampleUembed_Func1_suffix() {}

func ExampleConflict_Conflict()        {} // ambiguous with either Conflict or Conflict_Conflict type
func ExampleConflict_conflict()        {} // ambiguous with either Conflict or Conflict_conflict type
func ExampleConflict_Conflict_suffix() {} // ambiguous with either Conflict or Conflict_Conflict type
func ExampleConflict_conflict_suffix() {} // ambiguous with either Conflict or Conflict_conflict type
`

	// Parse literal source code as a *doc.Package.
	fset := token.NewFileSet()
	files := []*ast.File{
		mustParse(fset, "src.go", src),
		mustParse(fset, "src_test.go", test),
	}
	p, err := doc.NewFromFiles(fset, files, "example.com/p")
	if err != nil {
		t.Fatalf("doc.NewFromFiles: %v", err)
	}

	// Collect the association of examples to top-level identifiers.
	got := map[string][]string{}
	got[""] = exampleNames(p.Examples)
	for _, f := range p.Funcs {
		got[f.Name] = exampleNames(f.Examples)
	}
	for _, t := range p.Types {
		got[t.Name] = exampleNames(t.Examples)
		for _, f := range t.Funcs {
			got[f.Name] = exampleNames(f.Examples)
		}
		for _, m := range t.Methods {
			got[t.Name+"."+m.Name] = exampleNames(m.Examples)
		}
	}

	want := map[string][]string{
		"": {"", "suffix", "suffix_xX_X_x"}, // Package-level examples.

		"Type1":     {"", "foo_Suffix", "func2", "suffix"},
		"Type1_Foo": {"", "suffix"},
		"Type1_foo": {"", "suffix"},

		"Func1":     {"", "foo_Suffix", "suffix"},
		"Func1_Foo": {"", "suffix"},
		"Func1_foo": {"", "suffix"},

		"Type1.Func1":     {"", "foo_Suffix", "suffix"},
		"Type1.Func1_Foo": {"", "suffix"},
		"Type1.Func1_foo": {"", "suffix"},

		"Uembed.Func1": {"", "suffix"},

		// These are implementation dependent due to the ambiguous parsing.
		"Conflict_Conflict": {"", "suffix"},
		"Conflict_conflict": {"", "suffix"},
	}

	for id := range got {
		if !reflect.DeepEqual(got[id], want[id]) {
			t.Errorf("classification mismatch for %q:\ngot  %q\nwant %q", id, got[id], want[id])
		}
	}
}

func exampleNames(exs []*doc.Example) (out []string) {
	for _, ex := range exs {
		out = append(out, ex.Suffix)
	}
	return out
}

func mustParse(fset *token.FileSet, filename, src string) *ast.File {
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	return f
}

// readSectionFile reads a file that is divided into sections, and returns
// a map from section name to contents.
//
// A section begins with a line starting with at least four hyphens. Any number
// of additional hyphens may follow. After the last hyphen is the section name,
// which may be followed by more hyphens. The contents of the section are the
// lines that follow, until the next section start or EOF. For example, here is
// a section with name Foo and contents "hello":
//
//    ---- Foo
//    hello
//
// Lines before the first section are ignored.
//
// There is no way to represent a section that contains a line beginning with
// four or more hyphens.
func readSectionFile(filename string) (map[string]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readSections(filename, f)
}

func readSections(filename string, r io.Reader) (map[string]string, error) {
	scan := bufio.NewScanner(r)
	sections := map[string]string{}
	var name string
	var lines []string
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "----") {
			if name != "" {
				sections[name] = strings.Join(lines, "\n")
				lines = nil
			}
			name = strings.Trim(line, "- \t")
			if name == "" {
				return nil, fmt.Errorf("%s: empty name on line: %q", filename, line)
			}
			if _, ok := sections[name]; ok {
				return nil, fmt.Errorf("%s: duplicate section name %q", filename, name)
			}
		} else if name != "" {
			lines = append(lines, line)
		}
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	if name != "" {
		sections[name] = strings.Join(lines, "\n")
	}
	return sections, nil
}

const sectionFileContents = `
---- S1 ----
hello, world
---------------- empty ----------------
---- S2
two
lines
`

func TestReadSections(t *testing.T) {
	r := strings.NewReader(sectionFileContents)
	got, err := readSections("test", r)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"S1":    "hello, world",
		"S2":    "two\nlines",
		"empty": "",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
