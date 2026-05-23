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
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/cmd/internal/pkgsite-cli/client"
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
		json.NewEncoder(w).Encode(client.Package{
			PackageInfo: client.PackageInfo{
				Path:     "encoding/json",
				Synopsis: "Package json implements encoding and decoding of JSON.",
			},
			ModulePath:        "std",
			Version:           "go1.22.0",
			IsStandardLibrary: true,
			IsLatest:          true,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "standard library") {
		t.Errorf("output missing 'standard library':\n%s", stdout.String())
	}
}

func TestRunPackageGOOSGOARCH(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goos := r.URL.Query().Get("goos")
		goarch := r.URL.Query().Get("goarch")
		if goos != "windows" {
			t.Errorf("query param goos = %q, want %q", goos, "windows")
		}
		if goarch != "386" {
			t.Errorf("query param goarch = %q, want %q", goarch, "386")
		}
		json.NewEncoder(w).Encode(client.Package{
			PackageInfo: client.PackageInfo{
				Path: "encoding/json",
			},
			ModulePath: "std",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "-goos=windows", "-goarch=386", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
}

func TestRunPackageJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(client.Package{
			PackageInfo: client.PackageInfo{
				Path: "encoding/json",
			},
			ModulePath: "std",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "-json", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	var got packageResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	want := packageResult{
		Package: &client.Package{
			PackageInfo: client.PackageInfo{
				Path: "encoding/json",
			},
			ModulePath: "std",
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestRunModule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(client.Module{
			Path:     "golang.org/x/text",
			Version:  "v0.14.0",
			IsLatest: true,
			HasGoMod: true,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-server=" + srv.URL, "module", "golang.org/x/text"}, &stdout, &stderr)
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
		token := r.URL.Query().Get("token")
		if token == "" {
			json.NewEncoder(w).Encode(client.PaginatedResponse[client.SearchResult]{
				Items: []client.SearchResult{{
					PackagePath: "encoding/json",
					ModulePath:  "std",
					Version:     "go1.22.0",
				}},
				Total:         2,
				NextPageToken: "next-token-123",
			})
		} else if token == "next-token-123" {
			json.NewEncoder(w).Encode(client.PaginatedResponse[client.SearchResult]{
				Items: []client.SearchResult{{
					PackagePath: "encoding/xml",
					ModulePath:  "std",
					Version:     "go1.22.0",
				}},
				Total:         2,
				NextPageToken: "",
			})
		} else {
			http.Error(w, "invalid token", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	tests := []struct {
		name        string
		args        []string
		wantCode    int
		wantStdout  []string
		limitStdout []string // strings that must NOT be in stdout
	}{
		{
			name:       "all pages",
			args:       []string{"search", "json"},
			wantStdout: []string{"encoding/json", "encoding/xml"},
		},
		{
			name:        "with limit",
			args:        []string{"search", "-limit=1", "json"},
			wantStdout:  []string{"encoding/json"},
			limitStdout: []string{"encoding/xml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"-server=" + srv.URL}, tt.args...)
			code := run(args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			out := stdout.String()
			for _, s := range tt.wantStdout {
				if !strings.Contains(out, s) {
					t.Errorf("output missing %q:\n%s", s, out)
				}
			}
			for _, s := range tt.limitStdout {
				if strings.Contains(out, s) {
					t.Errorf("output contains %q but should not:\n%s", s, out)
				}
			}
		})
	}
}

func TestRunAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(client.Error{Code: 404, Message: "not found"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "-server=" + srv.URL, "nonexistent/pkg"}, &stdout, &stderr)
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
		json.NewEncoder(w).Encode(client.Error{Code: 404, Message: "not found"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"package", "-json", "-server=" + srv.URL, "nonexistent/pkg"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	// In JSON mode, error should go to stdout.
	var got client.Error
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	want := client.Error{Code: 404, Message: "not found"}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(client.Error{})); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestRunModuleWithVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1beta/versions/"):
			json.NewEncoder(w).Encode(client.PaginatedResponse[client.VersionResponse]{
				Items: []client.VersionResponse{{Version: "v0.14.0"}, {Version: "v0.13.0"}},
				Total: 2,
			})
		case strings.HasPrefix(r.URL.Path, "/v1beta/vulns/"):
			json.NewEncoder(w).Encode(client.PaginatedResponse[client.Vulnerability]{
				Items: []client.Vulnerability{{ID: "GO-2023-0001", Summary: "Bad thing"}},
				Total: 1,
			})
		default:
			json.NewEncoder(w).Encode(client.Module{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			})
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-server=" + srv.URL, "module", "-versions", "-vulns", "golang.org/x/text"}, &stdout, &stderr)
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
			if strings.Contains(path, ".") &&
				!strings.HasPrefix(path, "golang.org/x/pkgsite") &&
				path != "golang.org/x/sync/errgroup" &&
				!strings.HasPrefix(path, "golang.org/x/telemetry") {
				t.Errorf("%s imports third-party package %q", e.Name(), path)
			}
		}
	}
}

func TestHelpDocumentationSync(t *testing.T) {
	cmds := commands()
	var pkgCmd, modCmd, searchCmd *command
	for _, c := range cmds {
		switch c.name {
		case "package":
			pkgCmd = c
		case "module":
			modCmd = c
		case "search":
			searchCmd = c
		}
	}

	if pkgCmd == nil {
		t.Fatal("package command not found")
	}
	if modCmd == nil {
		t.Fatal("module command not found")
	}
	if searchCmd == nil {
		t.Fatal("search command not found")
	}

	t.Run("package", func(t *testing.T) {
		checkFields(t, reflect.TypeFor[packageResult](), pkgCmd.description)
		checkFields(t, reflect.TypeFor[client.Package](), pkgCmd.description)
	})

	t.Run("module", func(t *testing.T) {
		checkFields(t, reflect.TypeFor[moduleResult](), modCmd.description)
		checkFields(t, reflect.TypeFor[client.Module](), modCmd.description)
	})

	t.Run("search", func(t *testing.T) {
		checkFields(t, reflect.TypeFor[client.PaginatedResponse[client.SearchResult]](), searchCmd.description)
		checkFields(t, reflect.TypeFor[client.SearchResult](), searchCmd.description)
	})
}

func checkFields(t *testing.T, typ reflect.Type, doc string) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Anonymous {
			checkFields(t, f.Type, doc)
			continue
		}
		if f.PkgPath != "" {
			continue // skip unexported fields
		}
		if !strings.Contains(doc, f.Name) {
			t.Errorf("Documentation missing field %q of type %v", f.Name, typ)
		}
	}
}

func TestRunXFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1beta/package/") {
			json.NewEncoder(w).Encode(client.Package{
				PackageInfo: client.PackageInfo{
					Path: "encoding/json",
				},
				ModulePath: "std",
			})
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/search") {
			json.NewEncoder(w).Encode(client.PaginatedResponse[client.SearchResult]{
				Items: []client.SearchResult{},
			})
		}
	}))
	defer srv.Close()

	t.Run("package", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"package", "-x", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
		}
		errOut := stderr.String()
		expectedURL := srv.URL + "/v1beta/package/encoding/json"
		if !strings.Contains(errOut, expectedURL) {
			t.Errorf("stderr = %q, want to contain %q", errOut, expectedURL)
		}
	})

	t.Run("search", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"search", "-x", "-server=" + srv.URL, "json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
		}
		errOut := stderr.String()
		expectedURLPrefix := srv.URL + "/v1beta/search"
		if !strings.Contains(errOut, expectedURLPrefix) {
			t.Errorf("stderr = %q, want to contain %q", errOut, expectedURLPrefix)
		}
	})
}

func TestRun429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond := func(resp any) {
			token := r.URL.Query().Get("token")
			if token == "" {
				json.NewEncoder(w).Encode(resp)
			} else if token == "token1" {
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(client.Error{Code: 429, Message: "Too Many Requests"})
			}
		}

		if strings.HasPrefix(r.URL.Path, "/v1beta/package/") {
			respond(client.Package{
				PackageInfo: client.PackageInfo{
					Path: "encoding/json",
				},
				ModulePath: "std",
			})
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/symbols/") {
			respond(client.PackageSymbols{
				Symbols: client.PaginatedResponse[client.Symbol]{
					Items: []client.Symbol{
						{Name: "Sym1", Kind: "func"},
					},
					Total:         2,
					NextPageToken: "token1",
				},
			})
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/search") {
			respond(client.PaginatedResponse[client.SearchResult]{
				Items: []client.SearchResult{
					{PackagePath: "pkg1"},
				},
				Total:         2,
				NextPageToken: "token1",
			})
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/module/") {
			json.NewEncoder(w).Encode(client.Module{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			})
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/versions/") {
			respond(client.PaginatedResponse[client.VersionResponse]{
				Items: []client.VersionResponse{
					{Version: "v0.14.0"},
				},
				Total:         2,
				NextPageToken: "token1",
			})
		}
	}))
	defer srv.Close()

	checkMatch := func(t *testing.T, target, re string) {
		t.Helper()
		m, err := regexp.MatchString("(?m:"+re+")", target)
		if err != nil {
			t.Fatal(err)
		}
		if !m {
			t.Errorf("failed to match '%s': %s", re, target)
		}
	}

	t.Run("package json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"package", "-symbols", "-json", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		var got packageResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := packageResult{
			Package: &client.Package{
				PackageInfo: client.PackageInfo{
					Path: "encoding/json",
				},
				ModulePath: "std",
			},
			Symbols: &client.PaginatedResponse[client.Symbol]{
				Items: []client.Symbol{
					{Name: "Sym1", Kind: "func"},
				},
				Total:         2,
				NextPageToken: "token1",
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("package text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"package", "-symbols", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		out := stdout.String()
		errOut := stderr.String()
		checkMatch(t, out, "Sym1(.|\n)*token: token1")
		checkMatch(t, errOut, "Warning: hit rate limit")
	})

	t.Run("search json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"search", "-json", "-server=" + srv.URL, "json"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		var got client.PaginatedResponse[client.SearchResult]
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := client.PaginatedResponse[client.SearchResult]{
			Items: []client.SearchResult{
				{PackagePath: "pkg1"},
			},
			Total:         2,
			NextPageToken: "token1",
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("search text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"search", "-server=" + srv.URL, "json"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		out := stdout.String()
		errOut := stderr.String()
		checkMatch(t, out, "pkg1(.|\n)*")
		checkMatch(t, errOut, "Warning: hit rate limit")
	})

	t.Run("module json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"module", "-versions", "-json", "-server=" + srv.URL, "golang.org/x/text"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		var got moduleResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := moduleResult{
			Module: &client.Module{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			},
			Versions: &client.PaginatedResponse[client.VersionResponse]{
				Items: []client.VersionResponse{
					{Version: "v0.14.0"},
				},
				Total:         2,
				NextPageToken: "token1",
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("module text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"module", "-versions", "-server=" + srv.URL, "golang.org/x/text"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		out := stdout.String()
		errOut := stderr.String()
		checkMatch(t, out, "v0.14.0(.|\n)*token: token1")
		checkMatch(t, errOut, "Warning: hit rate limit")
	})
}

func TestRunPackagePagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1beta/package/") {
			json.NewEncoder(w).Encode(client.Package{
				PackageInfo: client.PackageInfo{
					Path: "encoding/json",
				},
				ModulePath: "std",
			})
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/symbols/") {
			token := r.URL.Query().Get("token")
			if token == "start-token" {
				json.NewEncoder(w).Encode(client.PackageSymbols{
					Symbols: client.PaginatedResponse[client.Symbol]{
						Items: []client.Symbol{
							{Name: "SymFromToken", Kind: "func"},
						},
						Total: 1,
					},
				})
			} else {
				http.Error(w, "missing or invalid token", http.StatusBadRequest)
			}
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/imported-by/") {
			token := r.URL.Query().Get("token")
			if token == "ib-start-token" {
				json.NewEncoder(w).Encode(client.PackageImportedBy{
					ImportedBy: client.PaginatedResponse[string]{
						Items: []string{"pkgFromToken"},
						Total: 1,
					},
				})
			} else {
				http.Error(w, "missing or invalid token", http.StatusBadRequest)
			}
		}
	}))
	defer srv.Close()

	t.Run("symbol-token", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"package", "-symbols", "-symbol-token=start-token", "-json", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
		}
		var got packageResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := packageResult{
			Package: &client.Package{
				PackageInfo: client.PackageInfo{
					Path: "encoding/json",
				},
				ModulePath: "std",
			},
			Symbols: &client.PaginatedResponse[client.Symbol]{
				Items: []client.Symbol{
					{Name: "SymFromToken", Kind: "func"},
				},
				Total: 1,
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("imported-by-token", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"package", "-imported-by", "-imported-by-token=ib-start-token", "-json", "-server=" + srv.URL, "encoding/json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
		}
		var got packageResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := packageResult{
			Package: &client.Package{
				PackageInfo: client.PackageInfo{
					Path: "encoding/json",
				},
				ModulePath: "std",
			},
			ImportedBy: &client.PackageImportedBy{
				ImportedBy: client.PaginatedResponse[string]{
					Items: []string{"pkgFromToken"},
					Total: 1,
				},
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestRunModulePagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1beta/module/") {
			json.NewEncoder(w).Encode(client.Module{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			})
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/versions/") {
			token := r.URL.Query().Get("token")
			if token == "ver-start-token" {
				json.NewEncoder(w).Encode(client.PaginatedResponse[client.VersionResponse]{
					Items: []client.VersionResponse{
						{Version: "v0.14.0"},
					},
					Total: 1,
				})
			} else {
				http.Error(w, "missing or invalid token", http.StatusBadRequest)
			}
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/vulns/") {
			token := r.URL.Query().Get("token")
			if token == "vuln-start-token" {
				json.NewEncoder(w).Encode(client.PaginatedResponse[client.Vulnerability]{
					Items: []client.Vulnerability{
						{ID: "GO-2023-0001"},
					},
					Total: 1,
				})
			} else {
				http.Error(w, "missing or invalid token", http.StatusBadRequest)
			}
		} else if strings.HasPrefix(r.URL.Path, "/v1beta/packages/") {
			token := r.URL.Query().Get("token")
			if token == "pkg-start-token" {
				json.NewEncoder(w).Encode(map[string]any{
					"packages": map[string]any{
						"items": []map[string]any{
							{"path": "golang.org/x/text/language"},
						},
						"total": 1,
					},
				})
			} else {
				http.Error(w, "missing or invalid token", http.StatusBadRequest)
			}
		}
	}))
	defer srv.Close()

	t.Run("versions-token", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"module", "-versions", "-versions-token=ver-start-token", "-json", "-server=" + srv.URL, "golang.org/x/text"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
		}
		var got moduleResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := moduleResult{
			Module: &client.Module{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			},
			Versions: &client.PaginatedResponse[client.VersionResponse]{
				Items: []client.VersionResponse{
					{Version: "v0.14.0"},
				},
				Total: 1,
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("vulns-token", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"module", "-vulns", "-vulns-token=vuln-start-token", "-json", "-server=" + srv.URL, "golang.org/x/text"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
		}
		var got moduleResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := moduleResult{
			Module: &client.Module{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			},
			Vulns: &client.PaginatedResponse[client.Vulnerability]{
				Items: []client.Vulnerability{
					{ID: "GO-2023-0001"},
				},
				Total: 1,
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("packages-token", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"module", "-packages", "-packages-token=pkg-start-token", "-json", "-server=" + srv.URL, "golang.org/x/text"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
		}
		var got moduleResult
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
		}
		want := moduleResult{
			Module: &client.Module{
				Path:    "golang.org/x/text",
				Version: "v0.14.0",
			},
			Packages: &client.PaginatedResponse[client.ModulePackageResponse]{
				Items: []client.ModulePackageResponse{
					{Path: "golang.org/x/text/language"},
				},
				Total: 1,
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}
