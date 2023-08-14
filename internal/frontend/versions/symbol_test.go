// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package versions

import (
	"testing"
)

func TestCompareStringSlices(t *testing.T) {
	a := []string{"a"}
	ab := []string{"a", "b"}
	ac := []string{"a", "c"}
	for _, test := range []struct {
		ss1, ss2 []string
		want     int
	}{
		{nil, nil, 0},
		{nil, a, -1},
		{ab, ab, 0},
		{a, ab, -1},
		{ab, ac, -1},
	} {
		got := compareStringSlices(test.ss1, test.ss2)
		if got != test.want {
			t.Fatalf("%v, %v: got %d, want %d\n", test.ss1, test.ss2, got, test.want)
		}
		if test.want != 0 {
			got := compareStringSlices(test.ss2, test.ss1)
			want := -test.want
			if got != want {
				t.Fatalf("%v, %v: got %d, want %d\n", test.ss2, test.ss1, got, want)
			}
		}
	}
}
