// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testenv

import (
	"os/exec"
	"sync"
	"testing"
)

var execPaths sync.Map // path -> error

// MustHaveExecPath checks that the current system can start the named executable
// using os.StartProcess or (more commonly) exec.Command.
// If not, MustHaveExecPath calls t.Skip with an explanation.
func MustHaveExecPath(t testing.TB, path string) {
	err, found := execPaths.Load(path)
	if !found {
		_, err = exec.LookPath(path)
		err, _ = execPaths.LoadOrStore(path, err)
	}
	if err != nil {
		t.Skipf("skipping test: %s: %s", path, err)
	}
}
