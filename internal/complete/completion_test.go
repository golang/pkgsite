// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package complete

import (
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
