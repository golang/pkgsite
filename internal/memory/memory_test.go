// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package memory

import (
	"testing"
)

func Test(t *testing.T) {
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
