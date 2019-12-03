// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package experiment

import (
	"context"
	"reflect"
	"testing"
)

func TestGetAndSetExperiments(t *testing.T) {
	const testExperiment1 = "test-experiment-1"
	set := map[string]bool{testExperiment1: true}
	ctx := NewContext(context.Background(), set)
	want := &Set{set: set}
	got := FromContext(ctx)
	if !reflect.DeepEqual(got, want) {
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
