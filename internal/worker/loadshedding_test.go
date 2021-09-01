// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package worker

import (
	"math"
	"testing"
)

func TestDecideToShed(t *testing.T) {
	// With a large maxSizeInFlight, we should never decide to shed no matter
	// the size.
	ls := loadShedder{maxSizeInFlight: math.MaxUint64}
	got, d := ls.shouldShed(1e10)
	if want := false; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	d() // reset sizeInFlight

	// If nothing else is in flight, accept something too large.
	ls.maxSizeInFlight = 10 * mib
	got, d = ls.shouldShed(20 * mib)
	if want := false; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	d()

	got, d = ls.shouldShed(3 * mib)
	if want := false; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}

	bytesInFlight := func() int {
		return int(ls.stats().SizeInFlight)
	}

	if got, want := bytesInFlight(), 3*mib; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	got, d2 := ls.shouldShed(8 * mib) // 8 + 3 > 10; shed
	d2()
	if want := true; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	d() // should decrement zipSizeInFlight
	if got, want := bytesInFlight(), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	got, d = ls.shouldShed(8 * mib) // 8 < 10; do not shed
	if want := false; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	d()
	if got, want := bytesInFlight(), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}
