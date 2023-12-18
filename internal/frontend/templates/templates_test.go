// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package templates

import (
	"testing"
)

func TestStripScheme(t *testing.T) {
	for _, test := range []struct {
		url, want string
	}{
		{"http://github.com", "github.com"},
		{"https://github.com/path/to/something", "github.com/path/to/something"},
		{"example.com", "example.com"},
		{"chrome-extension://abcd", "abcd"},
		{"nonwellformed.com/path?://query=1", "query=1"},
	} {
		if got := stripScheme(test.url); got != test.want {
			t.Errorf("%q: got %q, want %q", test.url, got, test.want)
		}
	}
}
