// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitPathVersion(t *testing.T) {
	tests := []struct {
		in         string
		path, vers string
	}{
		{"encoding/json", "encoding/json", ""},
		{"encoding/json@go1.22.0", "encoding/json", "go1.22.0"},
		{"golang.org/x/text@v0.14.0", "golang.org/x/text", "v0.14.0"},
		{"golang.org/x/text@latest", "golang.org/x/text", "latest"},
		{"golang.org/x/text", "golang.org/x/text", ""},
	}
	for _, tt := range tests {
		path, vers := splitPathVersion(tt.in)
		if path != tt.path || vers != tt.vers {
			t.Errorf("splitPathVersion(%q) = (%q, %q), want (%q, %q)", tt.in, path, vers, tt.path, tt.vers)
		}
	}
}

func TestRunUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), filepath.Base(os.Args[0])) {
		t.Errorf("help output does not contain %q", filepath.Base(os.Args[0]))
	}
}

func TestRunSubcommandHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "-h"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "package [flags] <package>[@version]") {
		t.Errorf("stderr = %q, want to contain 'package [flags] <package>[@version]'", stderr.String())
	}
}

func TestRunPackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(packageResponse{
			Path:              "encoding/json",
			ModulePath:        "std",
			ModuleVersion:     "go1.22.0",
			Synopsis:          "Package json implements encoding and decoding of JSON.",
			IsStandardLibrary: true,
			IsLatest:          true,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "--server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "standard library") {
		t.Errorf("output missing 'standard library':\n%s", stdout.String())
	}
}

func TestRunPackageJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(packageResponse{
			Path:       "encoding/json",
			ModulePath: "std",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "--json", "--server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	var result packageResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if result.Package.Path != "encoding/json" {
		t.Errorf("Path = %q, want encoding/json", result.Package.Path)
	}
}

func TestRunModule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(moduleResponse{
			Path:     "golang.org/x/text",
			Version:  "v0.14.0",
			IsLatest: true,
			HasGoMod: true,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--server=" + srv.URL, "module", "golang.org/x/text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "golang.org/x/text") {
		t.Errorf("output missing module path:\n%s", out)
	}
	if !strings.Contains(out, "v0.14.0 (latest)") {
		t.Errorf("output missing version:\n%s", out)
	}
}

func TestRunSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(paginatedResponse[searchResultResponse]{
			Items: []searchResultResponse{{
				PackagePath: "encoding/json",
				ModulePath:  "std",
				Version:     "go1.22.0",
			}},
			Total: 1,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--server=" + srv.URL, "search", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "encoding/json") {
		t.Errorf("output missing search result:\n%s", stdout.String())
	}
}

func TestRunAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(apiError{Code: 404, Message: "not found"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "--server=" + srv.URL, "nonexistent/pkg"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want to contain 'not found'", stderr.String())
	}
}

func TestRunAPIErrorJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(apiError{Code: 404, Message: "not found"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "--json", "--server=" + srv.URL, "nonexistent/pkg"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	// In JSON mode, error should go to stdout.
	if !strings.Contains(stdout.String(), "not found") {
		t.Errorf("stdout = %q, want to contain 'not found'", stdout.String())
	}
}

func TestRunModuleWithVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/versions/"):
			json.NewEncoder(w).Encode(paginatedResponse[versionResponse]{
				Items: []versionResponse{{Version: "v0.14.0"}, {Version: "v0.13.0"}},
				Total: 2,
			})
		case strings.HasPrefix(r.URL.Path, "/v1/vulns/"):
			json.NewEncoder(w).Encode(paginatedResponse[vulnResponse]{
				Items: []vulnResponse{{ID: "GO-2023-0001", Summary: "Bad thing"}},
				Total: 1,
			})
		default:
			json.NewEncoder(w).Encode(moduleResponse{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			})
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"--server=" + srv.URL, "module", "--versions", "--vulns", "golang.org/x/text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "v0.14.0") {
		t.Errorf("output missing version:\n%s", out)
	}
	if !strings.Contains(out, "GO-2023-0001") {
		t.Errorf("output missing vulnerability:\n%s", out)
	}
}

// TestNoThirdPartyImports verifies that pkginfo only imports the standard
// library, making it easy to migrate to x/tools or another repository
// with controlled dependencies.
func TestNoThirdPartyImports(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", e.Name(), err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, ".") {
				t.Errorf("%s imports third-party package %q", e.Name(), path)
			}
		}
	}
}
