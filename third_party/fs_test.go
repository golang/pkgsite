// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package thirdparty

import "testing"

func TestFS(t *testing.T) {
	for _, f := range []string{
		"dialog-polyfill/dialog-polyfill.js",
	} {
		if _, err := FS.Open(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
