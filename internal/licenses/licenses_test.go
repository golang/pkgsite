// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package licenses

import "testing"

func TestRedistributable(t *testing.T) {
	for _, test := range []struct {
		types []string
		want  bool
	}{
		{nil, false},
		{[]string{"UNKNOWN"}, false},
		{[]string{"MIT"}, true},
		{[]string{"MIT", "Unlicense"}, true},
		{[]string{"MIT", "JSON"}, true},
		{[]string{"MIT", "CommonsClause"}, false},
		{[]string{"MIT", "GPL-2.0", "ISC"}, true},
		{[]string{"MIT", "blessing"}, true},
		{[]string{"blessing"}, false},
		{[]string{"Freetype"}, true}, // appears in exceptions/freetype.lre
	} {
		got := Redistributable(test.types)
		if got != test.want {
			t.Errorf("%v: got %t, want %t", test.types, got, test.want)
		}
	}
}
