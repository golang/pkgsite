// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package importer

import (
	"go/ast"
	"testing"
)

func TestSimpleImporter(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{{
		name: "std",
		path: "math/rand",
	}, {
		name: "std-v2",
		path: "math/rand/v2",
	}, {
		name: "regular",
		path: "example.com/rand",
	}, {
		name: "regular-v2",
		path: "example.com/rand/v2",
	}, {
		name: "go-prefix",
		path: "example.com/go-rand",
	}, {
		name: "go-prefix-v2",
		path: "example.com/go-rand/v2",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			//lint:ignore SA1019 We had a preexisting dependency on ast.Object.
			obj, err := SimpleImporter(make(map[string]*ast.Object), tc.path)
			if err != nil {
				t.Fatalf("SimpleImporter(%q) returned error: %v", tc.path, err)
			}
			if obj.Name != "rand" {
				t.Errorf("SimpleImporter(%q).Name = %s, want = rand", tc.path, obj.Name)
			}
		})
	}
}
