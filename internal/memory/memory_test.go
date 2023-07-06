// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package memory

import (
	"runtime"
	"testing"
)

func TestRead(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("don't assume /proc/meminfo exists on non-linux platforms")
	}
	_, err := ReadSystemStats()
	if err != nil {
		t.Fatal(err)
	}
	_, err = ReadProcessStats()
	if err != nil {
		t.Fatal(err)
	}

	// We can't really test ReadCgroupStats, because we may or may not be in a cgroup.
}

func TestFormat(t *testing.T) {
	for _, test := range []struct {
		m    uint64
		want string
	}{
		{0, "0 B"},
		{1022, "1022 B"},
		{2500, "2.44 K"},
		{4096, "4.00 K"},
		{2_000_000, "1.91 M"},
		{18_000_000_000, "16.76 G"},
	} {
		got := Format(test.m)
		if got != test.want {
			t.Errorf("%d: got %q, want %q", test.m, got, test.want)
		}
	}
}
