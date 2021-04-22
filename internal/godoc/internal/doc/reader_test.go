// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doc

import (
	"testing"
)

func TestIsDeprecated(t *testing.T) {
	for _, test := range []struct {
		text string
		want bool
	}{
		{"A comment", false},
		{"Deprecated: foo", true},
		{" A comment\n   Deprecated: foo", true},
		{"This is\n Deprecated.", false},
		{"line 1\nDeprecated:\nline 2\n", true},
	} {
		got := isDeprecated(test.text)
		if got != test.want {
			t.Errorf("%q: got %t, want %t", test.text, got, test.want)
		}
	}
}
