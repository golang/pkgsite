// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"flag"
	"go/ast"
	"go/token"
	"io"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

var update = flag.Bool("update", false, "update goldens instead of checking against them")

func TestGoName(t *testing.T) {
	var r io.Reader
	g := &generator{pkg: "codec"}
	for _, test := range []struct {
		v    any
		want string
	}{
		{0, "int"},
		{uint(0), "uint"},
		{token.Pos(0), "token.Pos"},
		{Encoder{}, "Encoder"},
		{[][]Encoder{}, "[][]Encoder"},
		{bytes.Buffer{}, "bytes.Buffer"},
		{&r, "*io.Reader"},
		{[]int(nil), "[]int"},
		{map[*Decoder][]io.Writer{}, "map[*Decoder][]io.Writer"},
	} {
		got := g.goName(reflect.TypeOf(test.v))
		if got != test.want {
			t.Errorf("%T: got %q, want %q", test.v, got, test.want)
		}
	}
}

func TestGenerate(t *testing.T) {
	testGenerate(t, "slice", [][]int(nil))
	testGenerate(t, "map", map[string]bool(nil))
	testGenerate(t, "struct", ast.BasicLit{})
}

func testGenerate(t *testing.T, name string, x any) {
	t.Helper()
	var buf bytes.Buffer
	if err := generate(&buf, "somepkg", nil, x); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	testhelper.CompareWithGolden(t, got, name+".go", *update)
}

func TestExportedFields(t *testing.T) {
	type ef struct {
		A int
		B bool
		I int `codec:"-"` // this field will be ignored
		C string
	}

	check := func(want, got []field) {
		t.Helper()
		diff := cmp.Diff(want, got,
			cmp.Comparer(func(t1, t2 reflect.Type) bool { return t1 == t2 }))
		if diff != "" {
			t.Errorf("mismatch (-want, +got):\n%s", diff)
		}
	}

	// First time we see ef, no previous fields.
	got := exportedFields(reflect.TypeFor[ef](), nil)
	want := []field{
		{"A", reflect.TypeFor[int](), "0"},
		{"B", reflect.TypeFor[bool](), "false"},
		{"C", reflect.TypeFor[string](), `""`},
	}
	check(want, got)

	// Imagine that the previous ef had fields C and A in that order, but not B.
	// We should preserve the existing ordering and add B at the end.
	got = exportedFields(reflect.TypeFor[ef](), []string{"C", "A"})
	want = []field{
		{"C", reflect.TypeFor[string](), `""`},
		{"A", reflect.TypeFor[int](), "0"},
		{"B", reflect.TypeFor[bool](), "false"},
	}
	check(want, got)

	// Imagine instead that there had been a field D that was removed.
	// We still keep the names, but the entry for "D" has a nil type.
	got = exportedFields(reflect.TypeFor[ef](), []string{"A", "D", "B", "C"})
	want = []field{
		{"A", reflect.TypeFor[int](), "0"},
		{"D", nil, ""},
		{"B", reflect.TypeFor[bool](), "false"},
		{"C", reflect.TypeFor[string](), `""`},
	}
	check(want, got)
}
