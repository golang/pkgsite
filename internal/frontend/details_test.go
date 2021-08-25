// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"
)

func TestStdlibRedirectURL(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{"std", ""},
		{"cmd/go", ""},
		{"github.com/golang/go", "/std"},
		{"github.com/golang/go/src", "/std"},
		{"github.com/golang/go/src", "/std"},
		{"github.com/golang/go/cmd/go", "/cmd/go"},
		{"github.com/golang/go/src/cmd/go", "/cmd/go"},
		{"github.com/golang/gofrontend", ""},
		{"github.com/golang/gofrontend/libgo/misc/cgo/frontend/libgo/misc", ""},
	} {
		if got := stdlibRedirectURL(test.path); got != test.want {
			t.Errorf("stdlibRedirectURL(%q) = %q; want = %q", test.path, got, test.want)
		}
	}
}
