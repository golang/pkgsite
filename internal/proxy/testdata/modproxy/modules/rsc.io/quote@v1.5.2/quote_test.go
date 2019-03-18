// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quote

import (
	"os"
	"testing"
)

func init() {
	os.Setenv("LC_ALL", "en")
}

func TestHello(t *testing.T) {
	hello := "Hello, world."
	if out := Hello(); out != hello {
		t.Errorf("Hello() = %q, want %q", out, hello)
	}
}

func TestGlass(t *testing.T) {
	glass := "I can eat glass and it doesn't hurt me."
	if out := Glass(); out != glass {
		t.Errorf("Glass() = %q, want %q", out, glass)
	}
}

func TestGo(t *testing.T) {
	go1 := "Don't communicate by sharing memory, share memory by communicating."
	if out := Go(); out != go1 {
		t.Errorf("Go() = %q, want %q", out, go1)
	}
}

func TestOpt(t *testing.T) {
	opt := "If a program is too slow, it must have a loop."
	if out := Opt(); out != opt {
		t.Errorf("Opt() = %q, want %q", out, opt)
	}
}
