// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This test is an entry point to our CI code.
// We want LUCI builders that run "go test ./..." to execute our scripts.

package pkgsite

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestIntegration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux machines")
	}
	mustRunCommand(t, "./all.bash", "lint")
}

func mustRunCommand(t *testing.T, name string, arg ...string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		cmd := exec.Command(name, arg...)
		t.Logf("running %s...", cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed with %v:\n%s", cmd, err, out)
		}
		t.Logf("done running %s", cmd)
	})
}
