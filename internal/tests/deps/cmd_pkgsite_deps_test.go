// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// deps_test tests dependencies of cmd/pkgsite are kept to a limited set.
package deps_test

import (
	"os/exec"
	"strings"
	"testing"
)

// non-test packages are allowed to depend on licensecheck and safehtml, x/ repos, and markdown.
var allowedModDeps = map[string]bool{
	"github.com/google/licensecheck": true,
	"github.com/google/safehtml":     true,
	"golang.org/x/mod":               true,
	"golang.org/x/net":               true,
	"golang.org/x/pkgsite":           true,
	"golang.org/x/sync":              true,
	"golang.org/x/text":              true,
	"golang.org/x/tools":             true,
	"rsc.io/markdown":                true,
}

// test packages are also allowed to depend on go-cmp
var additionalAllowedTestModDeps = map[string]bool{
	"github.com/google/go-cmp": true,
}

func TestCmdPkgsiteDeps(t *testing.T) {
	// First, list all dependencies of pkgsite.
	out, err := exec.Command("go", "list", "-deps", "golang.org/x/pkgsite/cmd/pkgsite").Output()
	if err != nil {
		t.Fatal("running go list: ", err)
	}
	pkgs := strings.Fields(string(out))
	for _, pkg := range pkgs {
		// Filter to only the dependencies that are in the pkgsite module.
		if !strings.HasPrefix(pkg, "golang.org/x/pkgsite") {
			continue
		}

		// Get the test module deps and check them against allowedTestModDeps.
		out, err := exec.Command("go", "list", "-deps", "-test", "-f", "{{if .Module}}{{.Module.Path}}{{end}}", pkg).Output()
		if err != nil {
			t.Fatal(err)
		}
		testmodules := strings.Fields(string(out))
		for _, m := range testmodules {
			if !(allowedModDeps[m] || additionalAllowedTestModDeps[m]) {
				t.Fatalf("disallowed test module dependency %q found through %q", m, pkg)
			}
		}

		// Get the module deps and check them against allowedModDeps
		out, err = exec.Command("go", "list", "-deps", "-f", "{{if .Module}}{{.Module.Path}}{{end}}", pkg).Output()
		if err != nil {
			t.Fatal(err)
		}
		modules := strings.Fields(string(out))
		for _, m := range modules {
			if !allowedModDeps[m] {
				t.Fatalf("disallowed module dependency %q found through %q", m, pkg)
			}
		}
	}
}
