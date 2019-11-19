// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package complete

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEncodeDecode(t *testing.T) {
	completions := []Completion{
		{},
		{Version: "foo"},
		{Importers: 42},
		{Suffix: "foo"},
		{ModulePath: "foo"},
		{PackagePath: "github.com/foo/bar/baz"},
		{
			Suffix:      "github.com/foo",
			ModulePath:  "github.com/foo/bar",
			Version:     "v1.2.3",
			PackagePath: "github.com/foo/bar/baz",
			Importers:   101,
		},
		{
			Suffix:      "fmt",
			ModulePath:  "std",
			Version:     "go1.13",
			PackagePath: "fmt",
			Importers:   1234,
		},
	}
	for _, c := range completions {
		encoded := c.Encode()
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(c, *decoded); diff != "" {
			t.Errorf("[%#v] decoded mismatch (-initial +decoded):\n%s\nencoded: %q", c, diff, encoded)
		}
	}
}

func TestPathCompletions(t *testing.T) {
	partial := Completion{
		ModulePath:  "my.module/foo",
		PackagePath: "my.module/foo/bar",
		Version:     "v1.2.3",
		Importers:   123,
	}
	completions := PathCompletions(partial)
	sort.Slice(completions, func(i, j int) bool {
		return len(completions[i].Suffix) < len(completions[j].Suffix)
	})
	wantSuffixes := []string{"bar", "foo/bar", "my.module/foo/bar"}
	if got, want := len(completions), len(wantSuffixes); got != want {
		t.Fatalf("len(pathCompletions(%v)) = %d, want %d", partial, got, want)
	}
	for i, got := range completions {
		want := partial
		want.Suffix = wantSuffixes[i]
		if diff := cmp.Diff(want, *got); diff != "" {
			t.Errorf("completions[%d] mismatch (-want +got)\n%s", i, diff)
		}
	}
}

func TestPathSuffixes(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"foo/Bar/baz", []string{"foo/bar/baz", "bar/baz", "baz"}},
		{"foo", []string{"foo"}},
		{"BAR", []string{"bar"}},
	}
	for _, test := range tests {
		if got := pathSuffixes(test.path); !cmp.Equal(got, test.want) {
			t.Errorf("prefixes(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}
