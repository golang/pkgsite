// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCollectPaths(t *testing.T) {
	got := collectPaths([]string{"a", "b,c2,d3", "e4", "f,g"})
	want := []string{"a", "b", "c2", "d3", "e4", "f", "g"}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
