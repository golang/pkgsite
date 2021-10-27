// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package static

import "testing"

func TestFS(t *testing.T) {
	for _, f := range []string{
		"shared/reset.css",
		"frontend/_modals.tmpl",
		"frontend/homepage/homepage.tmpl",
		"shared/logo/go-blue.svg",
		"frontend/unit/unit.css",
		"frontend/unit/_header.tmpl",
	} {
		if _, err := FS.Open(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
