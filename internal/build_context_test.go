// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "testing"

func TestCompareBuildContexts(t *testing.T) {
	for i, c1 := range BuildContexts {
		if got := CompareBuildContexts(c1, c1); got != 0 {
			t.Errorf("%v: got %d, want 0", c1, got)
		}
		for _, c2 := range BuildContexts[i+1:] {
			if got := CompareBuildContexts(c1, c2); got >= 0 {
				t.Errorf("%v, %v: got %d, want < 0", c1, c2, got)
			}
			if got := CompareBuildContexts(c2, c1); got <= 0 {
				t.Errorf("%v, %v: got %d, want > 0", c2, c1, got)
			}
		}
	}
	got := CompareBuildContexts(BuildContext{"?", "?"}, BuildContexts[len(BuildContexts)-1])
	if got <= 0 {
		t.Errorf("unknown vs. last: got %d, want > 0", got)
	}
}
