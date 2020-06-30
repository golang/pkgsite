// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import "testing"

func TestLicenseAnchors(t *testing.T) {
	for _, test := range []struct {
		in, want []string
	}{
		{[]string{"L.md"}, []string{"lic-0"}},
		// Identifiers are distinguished by the position in the sorted list.
		{[]string{"L.md", "L_md"}, []string{"lic-0", "lic-1"}},
		{[]string{"L_md", "L.md"}, []string{"lic-1", "lic-0"}},
	} {
		gotIDs := licenseAnchors(test.in)
		if len(test.want) != len(gotIDs) {
			t.Errorf("%v: mismatched lengths", test.in)
		} else {
			for i, g := range gotIDs {
				if got, want := g.String(), test.want[i]; got != want {
					t.Errorf("%v, #%d: got %q, want %q", test.in, i, got, want)
				}
			}
		}
	}
}
