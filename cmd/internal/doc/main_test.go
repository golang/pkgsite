// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/pkgsite/cmd/internal/pkgsite"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

// TestServe checks that the server serves the pages that "go doc -http"
// opens: /std when the user is outside a module, and the current module's
// page when inside one.
//
// cmd/go/internal/doc.doPkgsite runs the command without setting cmd.Dir, so
// it inherits the directory the user ran "go doc" in and takes GOROOT from
// -gorepo. The setup below mirrors that.
func TestServe(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("skipping: %v", err)
	}
	goRepoPath := internal.GOROOT()
	if goRepoPath == "" {
		t.Skip("skipping: cannot determine GOROOT")
	}
	// main calls this before BuildServer.
	stdlib.SetGoRepoPath(goRepoPath)

	// These pages come from the current directory and from GOROOT. An empty
	// cache and an empty proxy make every download fail, so a getter that
	// stops answering fails the test instead of passing on a published copy.
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOPROXY", fileProxy(t.TempDir()))
	t.Setenv("GOTOOLCHAIN", "local") // a toolchain switch downloads too

	t.Chdir(writeModule(t, "example.com/testmod", "testmod",
		"// Package testmod is served from the current directory."))

	server, err := pkgsite.BuildServer(t.Context(), serverConfig(goRepoPath))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	server.Install(mux.Handle, nil, nil)

	for _, test := range []struct {
		name string
		path string
		want string // text the page must contain
	}{
		{
			"stdlib index",
			"/std",
			"archive/tar",
		},
		{
			// Resolves through the module path "std", so it needs the
			// same getter as /std.
			"stdlib package",
			"/errors",
			"Package errors",
		},
		{
			"current module",
			"/example.com/testmod",
			"Package testmod",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s: got status %d, want %d", test.path, w.Code, http.StatusOK)
			}
			if body := w.Body.String(); !strings.Contains(body, test.want) {
				t.Errorf("GET %s: page does not contain %q", test.path, test.want)
			}
		})
	}
}

// fileProxy returns dir as a GOPROXY value. A Windows path needs one more
// slash, so that C:\x becomes file:///C:/x; see cmd/go/internal/web.
func fileProxy(dir string) string {
	if filepath.VolumeName(dir) != "" {
		return "file:///" + filepath.ToSlash(dir)
	}
	return "file://" + dir
}

// writeModule writes a module with one package to a new temporary directory
// and returns that directory.
//
// It avoids t.TempDir, which fails the test when removal fails. BuildServer
// leaves goroutines running after the server is gone, and on Windows they can
// still hold the directory, so the test would be flaky for a reason it does
// not control.
//
// TODO(hyangah): use t.TempDir once BuildServer takes a context that stops
// those goroutines, and a pkgsite release carries the fix.
func writeModule(t *testing.T, modulePath, pkgName, doc string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pkgsite-doc-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("leaving %s behind: %v", dir, err)
		}
	})
	for name, content := range map[string]string{
		"go.mod":        "module " + modulePath + "\n\ngo 1.26.0\n",
		pkgName + ".go": doc + "\npackage " + pkgName + "\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
