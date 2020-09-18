// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import "testing"

func TestDecideToShed(t *testing.T) {
	// By default (GO_DISCOVERY_MAX_IN_FLIGHT_ZIP_MI is unset), we should never decide to shed no matter the size of the zip.
	got, d := decideToShed(1e10)
	if want := false; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	d() // reset zipSizeInFlight
	maxZipSizeInFlight = 10 * mib
	got, d = decideToShed(3 * mib)
	if want := false; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	bytesInFlight := func() int {
		return int(GetLoadShedStats().ZipBytesInFlight)
	}

	if got, want := bytesInFlight(), 3*mib; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	got, _ = decideToShed(8 * mib) // 8 + 3 > 10; shed
	if want := true; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	d() // should decrement zipSizeInFlight
	if got, want := bytesInFlight(), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	got, d = decideToShed(8 * mib) // 8 < 10; do not shed
	if want := false; got != want {
		t.Fatalf("got %t, want %t", got, want)
	}
	d()
	if got, want := bytesInFlight(), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}
