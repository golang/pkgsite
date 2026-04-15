// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatPackage(t *testing.T) {
	var buf bytes.Buffer
	formatPackage(&buf, packageResult{
		Package: &packageResponse{
			Path:              "encoding/json",
			ModulePath:        "std",
			ModuleVersion:     "go1.22.0",
			Synopsis:          "Package json implements encoding and decoding of JSON.",
			IsStandardLibrary: true,
			IsLatest:          true,
		},
	})
	out := buf.String()
	for _, want := range []string{
		"encoding/json (standard library)",
		"Module:   std",
		"go1.22.0 (latest)",
		"Package json",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatPackageWithExtras(t *testing.T) {
	var buf bytes.Buffer
	formatPackage(&buf, packageResult{
		Package: &packageResponse{
			Path:          "github.com/foo/bar",
			ModulePath:    "github.com/foo/bar",
			ModuleVersion: "v1.0.0",
			Imports:       []string{"fmt", "strings"},
			Licenses:      []licenseResponse{{Types: []string{"MIT"}, FilePath: "LICENSE"}},
		},
		Symbols: &paginatedResponse[symbolResponse]{
			Items: []symbolResponse{
				{Name: "New", Kind: "func", Synopsis: "func New() *Bar"},
			},
			Total: 1,
		},
		ImportedBy: &importedByResponse{
			ImportedBy: paginatedResponse[string]{
				Items: []string{"github.com/baz/qux"},
				Total: 100,
			},
		},
	})
	out := buf.String()
	for _, want := range []string{
		"Imports:",
		"  fmt",
		"Licenses:",
		"  MIT (LICENSE)",
		"Symbols:",
		"  func New() *Bar",
		"Imported by:",
		"  github.com/baz/qux",
		"Showing 1 of 100",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
