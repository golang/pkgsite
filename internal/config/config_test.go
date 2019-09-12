// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import "testing"

func TestValidateAppVersion(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantErr bool
	}{
		{"", true},
		{"20190912t130708", false},
		{"20190912t130708x", true},
		{"2019-09-12t13-07-0400", false},
		{"2019-09-12t13070400", true},
		{"2019-09-11t22-14-0400-2f4680648b319545c55c6149536f0a74527901f6", false},
	} {
		err := ValidateAppVersion(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateAppVersion(%q) = %v, want error = %t", tc.in, err, tc.wantErr)
		}
	}
}
