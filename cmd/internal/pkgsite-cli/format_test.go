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

func TestFormatModule(t *testing.T) {
	var buf bytes.Buffer
	formatModule(&buf, moduleResult{
		Module: &moduleResponse{
			Path:              "golang.org/x/text",
			Version:           "v0.14.0",
			IsLatest:          true,
			HasGoMod:          true,
			IsRedistributable: true,
			RepoURL:           "https://github.com/golang/text",
		},
	})
	out := buf.String()
	for _, want := range []string{
		"golang.org/x/text",
		"v0.14.0 (latest)",
		"https://github.com/golang/text",
		"Has go.mod:       yes",
		"Redistributable:  yes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatModuleWithExtras(t *testing.T) {
	var buf bytes.Buffer
	formatModule(&buf, moduleResult{
		Module: &moduleResponse{
			Path:    "golang.org/x/text",
			Version: "v0.14.0",
			Readme:  &readmeResponse{Filepath: "README.md", Contents: "# text"},
		},
		Versions: &paginatedResponse[versionResponse]{
			Items: []versionResponse{{Version: "v0.14.0"}, {Version: "v0.13.0"}},
			Total: 2,
		},
		Vulns: &paginatedResponse[vulnResponse]{
			Items: []vulnResponse{{ID: "GO-2023-0001", Summary: "Bad thing", FixedVersion: "v0.14.0"}},
			Total: 1,
		},
		Packages: &paginatedResponse[modulePackageResponse]{
			Items: []modulePackageResponse{{Path: "golang.org/x/text/language", Synopsis: "BCP 47 tags"}},
			Total: 1,
		},
	})
	out := buf.String()
	for _, want := range []string{
		"README (README.md):",
		"# text",
		"Versions:",
		"  v0.14.0",
		"Vulnerabilities:",
		"  GO-2023-0001",
		"    Bad thing",
		"    Fixed in: v0.14.0",
		"Packages:",
		"golang.org/x/text/language",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatSearch(t *testing.T) {
	var buf bytes.Buffer
	formatSearch(&buf, &paginatedResponse[searchResultResponse]{
		Items: []searchResultResponse{{
			PackagePath: "encoding/json",
			ModulePath:  "std",
			Version:     "go1.22.0",
			Synopsis:    "Package json.",
		}},
		Total: 1,
	})
	out := buf.String()
	if !strings.Contains(out, "encoding/json") {
		t.Errorf("output missing package path:\n%s", out)
	}
	if !strings.Contains(out, "std@go1.22.0") {
		t.Errorf("output missing module@version:\n%s", out)
	}
}

func TestFormatSearchEmpty(t *testing.T) {
	var buf bytes.Buffer
	formatSearch(&buf, &paginatedResponse[searchResultResponse]{})
	if !strings.Contains(buf.String(), "No results") {
		t.Errorf("expected 'No results' message, got:\n%s", buf.String())
	}
}
