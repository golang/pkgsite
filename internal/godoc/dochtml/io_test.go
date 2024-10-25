// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dochtml

import (
	"bytes"
	"testing"
)

func TestLimitBuffer(t *testing.T) {
	const limit = 10
	for _, tc := range []struct {
		sizes []int
		want  int
	}{
		{[]int{1, 2, 3}, 4},
		{[]int{21}, -11},
		{[]int{1, 2, 3, 3, 5}, -4},
	} {
		lb := &limitBuffer{new(bytes.Buffer), limit}
		for _, n := range tc.sizes {
			_, _ = lb.Write(make([]byte, n))
		}
		if got := lb.Remain; got != int64(tc.want) {
			t.Errorf("%v: got %d, want %d", tc.sizes, got, tc.want)
		}
	}
}
