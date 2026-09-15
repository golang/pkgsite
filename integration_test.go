// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// These tests are an entry point to our CI code.
// We want LUCI builders that run "go test ./..." to execute our scripts.
//
// [TestLint] needs nothing but a Go toolchain, so it runs on every Linux
// builder. [TestIntegration] stands up our services in containers, so it runs
// only on Go builders with Docker, with PKGSITE_CI_MODE selecting which of its
// stages to run.

package pkgsite

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// modeEnv names the environment variable that selects which [TestIntegration]
// stages to run. On a Go builder with Docker an unset value means modeCore,
// while the "screentest" run mod sets it to modeScreentest.
const modeEnv = "PKGSITE_CI_MODE"

const (
	// modeCore runs the deterministic stages: the NPM lint and tests, all.bash
	// ci (the PostgreSQL-backed test suite), and the search integration tests.
	modeCore = "core"

	// modeScreentest runs only the headless-Chrome visual regression suite,
	// which is prone to rendering and timing flakes.
	modeScreentest = "screentest"
)

// TestLint runs the checks that need nothing but a Go toolchain, so that they
// run on every Linux builder rather than only the few that have Docker.
func TestLint(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux machines")
	}
	if os.Getenv("GO_DISCOVERY_TESTDB") != "" {
		t.Skip("skipping inside all.bash ci container")
	}
	runStage(t, "AllBash", "./all.bash", "lint")
}

// TestIntegration runs the CI stages that stand up our services in containers:
// PostgreSQL for the database tests, headless Chrome for the screenshot tests
// and Node for the JavaScript ones.
//
// It runs on Go builders that have Docker. Elsewhere, including on a developer
// machine running "go test ./...", it skips unless PKGSITE_CI_MODE opts in.
func TestIntegration(t *testing.T) {
	mode := os.Getenv(modeEnv)
	switch mode {
	case "":
		if os.Getenv("GO_BUILDER_NAME") == "" {
			t.Skipf("neither %s nor GO_BUILDER_NAME is set", modeEnv)
		}
		mode = modeCore
	case modeCore, modeScreentest:
		// Valid modes.
	default:
		t.Fatalf("unknown %s=%q, want %q or %q", modeEnv, mode, modeCore, modeScreentest)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		if os.Getenv(modeEnv) != "" || strings.Contains(os.Getenv("GO_BUILDER_NAME"), "docker") {
			t.Fatalf("docker is not installed: %v", err)
		}
		t.Skipf("docker is not installed: %v", err)
	}

	// Tear the compose project down on the way in as well as the way out. A
	// build that is killed part way through leaves containers behind holding
	// the database port, which would otherwise fail every later build that
	// lands on the same reused builder.
	composeDown(t)
	t.Cleanup(func() {
		chownRepo(t)
		composeDown(t)
	})

	if mode == modeScreentest {
		t.Run("Screentest", func(t *testing.T) {
			t.Cleanup(func() {
				if !t.Failed() {
					return
				}
				const outDir = "tests/screentest/output"
				if _, err := os.Stat(outDir); err != nil {
					t.Logf("stat screentest output: %v", err)
					return
				}
				if err := os.CopyFS(t.ArtifactDir(), os.DirFS(outDir)); err != nil {
					t.Logf("copy screentest output: %v", err)
				}
			})
			runCmd(t, "./tests/screentest/run.sh", "-rm", "ci", "-concurrency", "1")
		})
		return
	}

	t.Run("NPM", func(t *testing.T) {
		if !runStage(t, "Install", "./devtools/nodejs.sh", "npm", "ci") {
			return
		}
		runStage(t, "Lint", "./devtools/nodejs.sh", "npm", "run", "lint")
		runStage(t, "Test", "./devtools/nodejs.sh", "npm", "run", "test")
	})
	runStage(t, "AllBash", "./devtools/docker/compose.sh", "run", "--rm", "allbash", "ci")
	runStage(t, "SearchTest", "./tests/search/run.sh")
}

// composeDownTimeout bounds teardown, which must not hang a build.
const composeDownTimeout = 5 * time.Minute

// composeDown removes the containers, volumes and networks of the compose
// project used by the CI stages.
func composeDown(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), composeDownTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "./devtools/docker/compose.sh",
		"down", "--volumes", "--remove-orphans")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("compose down failed; containers may be left behind for the next build on this machine: %v\n%s", err, out)
	}
}

// chownRepo resets ownership of files created by root inside Docker containers
// (such as node_modules) back to the host user so Swarming can delete the workdir.
func chownRepo(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), composeDownTimeout)
	defer cancel()

	ug := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	cmd := exec.CommandContext(ctx, "./devtools/nodejs.sh", "chown", "-R", ug, "/pkgsite")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("chown failed: %v\n%s", err, out)
	}
}

// runStage runs a CI stage as a subtest, streaming its output into the test
// log, and reports whether the subtest passed.
func runStage(t *testing.T, stage, name string, args ...string) bool {
	t.Helper()
	return t.Run(stage, func(t *testing.T) {
		t.Helper()
		runCmd(t, name, args...)
	})
}

// runCmd streams a command's output into the test log. t.Output is line
// buffered, so that the output of a stage is attributed to the test and appears
// as it is produced rather than only once the stage finishes.
func runCmd(t *testing.T, name string, args ...string) {
	t.Helper()
	t.Logf("$ %s %s", name, strings.Join(args, " "))

	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Stdout = t.Output()
	cmd.Stderr = t.Output()
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}
