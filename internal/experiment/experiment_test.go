// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package experiment

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGetAndSetExperiments(t *testing.T) {
	const testExperiment1 = "test-experiment-1"
	ctx := NewContext(context.Background(), testExperiment1)
	want := NewSet(testExperiment1)
	got := FromContext(ctx)
	if !cmp.Equal(want, got, cmp.AllowUnexported(Set{})) {
		t.Fatalf("FromContext(ctx) = %v; want = %v", got, want)
	}
	if !IsActive(ctx, testExperiment1) {
		t.Fatalf("s.IsActive(ctx, %q) = false; want = true", testExperiment1)
	}
	const testExperiment2 = "inactive-experiment"
	if IsActive(ctx, testExperiment2) {
		t.Fatalf("s.IsActive(ctx, %q) = true; want = false", testExperiment2)
	}
}
