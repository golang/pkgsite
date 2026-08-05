// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/tools/txtar"
)

func TestEvaldocServer(t *testing.T) {
	testModules := []*proxytest.Module{
		{
			ModulePath: "example.com/mod",
			Version:    "v1.0.0",
			Files: map[string]string{
				"root.go":                   "package mod",
				"foo/foo.go":                "package foo\n\n// DocFunc has doc.\nfunc DocFunc() {}\n\nfunc UndocFunc() {}\n",
				"foo/foo_test.go":           "package foo\n\nfunc TestFoo(t *testing.T) {}\n",
				"internal/secret/secret.go": "package secret",
				"testdata/ignored.go":       "package testdata",
				"vendor/bar/bar.go":         "package bar",
				".ignored/ignored.go":       "package ignored",
				"nodirs/doc.txt":            "just text",
			},
		},
	}

	proxyClient, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	ctx := context.Background()
	zipReader, err := proxyClient.Zip(ctx, "example.com/mod", "v1.0.0")
	if err != nil {
		t.Fatalf("proxyClient.Zip: %v", err)
	}

	contentDir, err := fs.Sub(zipReader, "example.com/mod@v1.0.0")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	s, err := newServer("example.com/mod", "v1.0.0", contentDir)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	handler := s

	testCases := []struct {
		name        string
		path        string
		wantStatus  int
		wantSubstrs []string
	}{
		{
			name:       "styles.css returns stylesheet",
			path:       "/styles.css",
			wantStatus: http.StatusOK,
			wantSubstrs: []string{
				`body { font-family: sans-serif;`,
				`.has-doc { color: #1a7f37;`,
				`.need-doc { color: #cf222e;`,
			},
		},
		{
			name:       "root path lists import paths with links and symbol counts",
			path:       "/",
			wantStatus: http.StatusOK,
			wantSubstrs: []string{
				`<link rel="stylesheet" href="/styles.css">`,
				`<h1>Module example.com/mod@v1.0.0
	    <span class="has-doc">1</span>
	    <span class="need-doc">1</span>
	    <span class="need-pct">50%</span>
	</h1>`,
				`<a href="/example.com/mod/foo">example.com/mod/foo</a>
				<span class="has-doc">1</span>
				<span class="need-doc">1</span>
				<span class="need-pct">50%</span>`,
			},
		},
		{
			name:       "directory view lists package files with links and symbol counts",
			path:       "/example.com/mod/foo",
			wantStatus: http.StatusOK,
			wantSubstrs: []string{
				`<link rel="stylesheet" href="/styles.css">`,
				`<a href="/example.com/mod/foo/foo.go">foo.go</a>
				<span class="has-doc">1</span>
				<span class="need-doc">1</span>
				<span class="need-pct">50%</span>`,
			},
		},
		{
			name:       "file view shows file content with symbol highlighting",
			path:       "/example.com/mod/foo/foo.go",
			wantStatus: http.StatusOK,
			wantSubstrs: []string{
				`<link rel="stylesheet" href="/styles.css">`,
				`package foo`,
				`<span class="has-doc">DocFunc</span>`,
				`<span class="need-doc">UndocFunc</span>`,
			},
		},
		{
			name:       "internal directory 404",
			path:       "/example.com/mod/internal/secret",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "test file 404",
			path:       "/example.com/mod/foo/foo_test.go",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "non-go file 404",
			path:       "/example.com/mod/nodirs/doc.txt",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "non-existent directory 404",
			path:       "/example.com/mod/nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "non-existent file 404",
			path:       "/example.com/mod/foo/nonexistent.go",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("HTTP status = %d, want %d (body: %s)", w.Code, tc.wantStatus, w.Body.String())
			}

			body := w.Body.String()
			for _, want := range tc.wantSubstrs {
				if !strings.Contains(body, want) {
					t.Errorf("body does not contain %q; body:\n%s", want, body)
				}
			}
		})
	}
}

func TestLocalDirectoryFS(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/localpkg\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}
	mainGo := "package main\n\n// Main is main.\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	s, err := newServer("example.com/localpkg", "local", os.DirFS(dir))
	if err != nil {
		t.Fatalf("newServer local dir: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", w.Code, http.StatusOK)
	}

	if !strings.Contains(w.Body.String(), "example.com/localpkg") {
		t.Errorf("body does not contain example.com/localpkg; body:\n%s", w.Body.String())
	}
}

func TestResolveLocalPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GOOS is not linux")
	}
	t.Setenv("HOME", "/fake/home")

	got, err := resolveLocalPath(".")
	if err != nil {
		t.Fatalf("resolveLocalPath(.): %v", err)
	}
	want, _ := filepath.Abs(".")
	if got != want {
		t.Errorf("resolveLocalPath(.) = %q, want %q", got, want)
	}

	gotHome, err := resolveLocalPath("~/sub")
	if err != nil {
		t.Fatalf("resolveLocalPath(~/sub): %v", err)
	}
	wantHome := filepath.Join("/fake/home", "sub")
	if gotHome != wantHome {
		t.Errorf("resolveLocalPath(~/sub) = %q, want %q", gotHome, wantHome)
	}
}

func TestGetContentDirEmpty(t *testing.T) {
	_, _, _, err := getContentDir("")
	if err == nil {
		t.Fatal("getContentDir(\"\") expected error, got nil")
	}
}

func TestSortingOrder(t *testing.T) {
	testModules := []*proxytest.Module{
		{
			ModulePath: "example.com/sortmod",
			Version:    "v1.0.0",
			Files: map[string]string{
				"pkgA/a.go":  "package pkgA\n\n// A1 doc\nfunc A1() {}\nfunc A2() {}\n",    // 1 red
				"pkgB/b.go":  "package pkgB\n\nfunc B1() {}\nfunc B2() {}\nfunc B3() {}\n", // 3 red
				"pkgC/c1.go": "package pkgC\n\nfunc C1() {}\n",                             // 1 red
				"pkgC/c2.go": "package pkgC\n\nfunc C2() {}\nfunc C3() {}\n",               // 2 red (total pkgC: 3 red)
			},
		},
	}

	proxyClient, teardown := proxytest.SetupTestClient(t, testModules)
	defer teardown()

	ctx := context.Background()
	zipReader, err := proxyClient.Zip(ctx, "example.com/sortmod", "v1.0.0")
	if err != nil {
		t.Fatalf("proxyClient.Zip: %v", err)
	}

	contentDir, err := fs.Sub(zipReader, "example.com/sortmod@v1.0.0")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}

	s, err := newServer("example.com/sortmod", "v1.0.0", contentDir)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	wantImportPaths := []string{
		"example.com/sortmod/pkgB",
		"example.com/sortmod/pkgC",
		"example.com/sortmod/pkgA",
	}
	var gotImportPaths []string
	for _, item := range s.importPaths {
		gotImportPaths = append(gotImportPaths, item.ImportPath)
	}
	if !slices.Equal(gotImportPaths, wantImportPaths) {
		t.Errorf("importPaths sorting mismatch: got %v, want %v", gotImportPaths, wantImportPaths)
	}

	wantFiles := []string{"c2.go", "c1.go"}
	var gotFiles []string
	for _, item := range s.dirFiles["example.com/sortmod/pkgC"] {
		gotFiles = append(gotFiles, item.Filename)
	}
	if !slices.Equal(gotFiles, wantFiles) {
		t.Errorf("file sorting mismatch in pkgC: got %v, want %v", gotFiles, wantFiles)
	}
}

func TestPackageDirs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "package_dirs.txtar"))
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	archive := txtar.Parse(data)
	fsys, err := txtar.FS(archive)
	if err != nil {
		t.Fatalf("txtar.FS: %v", err)
	}

	got, err := packageDirs(fsys)
	if err != nil {
		t.Fatalf("packageDirs: %v", err)
	}

	want := []string{".", "foo"}
	if !slices.Equal(got, want) {
		t.Errorf("packageDirs() = %v, want %v", got, want)
	}
}
