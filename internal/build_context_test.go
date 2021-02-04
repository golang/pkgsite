// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "testing"

func TestCompareBuildContexts(t *testing.T) {
	check := func(c1, c2 BuildContext, want int) {
		t.Helper()
		got := CompareBuildContexts(c1, c2)
		switch want {
		case 0:
			if got != 0 {
				t.Errorf("%v vs. %v: got %d, want 0", c1, c2, got)
			}
		case 1:
			if got <= 0 {
				t.Errorf("%v vs. %v: got %d, want > 0", c1, c2, got)
			}
		case -1:
			if got >= 0 {
				t.Errorf("%v vs. %v: got %d, want < 0", c1, c2, got)
			}
		}
	}

	for i, c1 := range BuildContexts {
		check(c1, c1, 0)
		for _, c2 := range BuildContexts[i+1:] {
			check(c1, c2, -1)
			check(c2, c1, 1)
		}
	}

	// Special cases.
	check(BuildContext{"?", "?"}, BuildContexts[len(BuildContexts)-1], 1) // unknown is last
}
