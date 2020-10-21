// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"flag"
	"go/token"
	"io"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "update goldens instead of checking against them")

func TestGoName(t *testing.T) {
	var r io.Reader
	g := &generator{pkg: "codec"}
	for _, test := range []struct {
		v    interface{}
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
}

func testGenerate(t *testing.T, name string, x interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if err := Generate(&buf, "somepkg", x); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if *update {
		writeGolden(t, name, got)
	} else {
		want := readGolden(t, name)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s: mismatch (-want, +got):\n%s", name, diff)
		}
	}
}

func writeGolden(t *testing.T, name string, data string) {
	filename := filepath.Join("testdata", name+".go")
	if err := ioutil.WriteFile(filename, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", filename)
}

func readGolden(t *testing.T, name string) string {
	data, err := ioutil.ReadFile(filepath.Join("testdata", name+".go"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
