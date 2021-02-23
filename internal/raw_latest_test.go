// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"testing"

	"golang.org/x/mod/modfile"
)

func TestIsDeprecated(t *testing.T) {
	for _, test := range []struct {
		name        string
		in          string
		wantIs      bool
		wantComment string
	}{
		{"no comment", `module m`, false, ""},
		{"valid comment",
			`
			// Deprecated: use v2
			module m
		`, true, "use v2"},
		{"take first",
			`
			// Deprecated: use v2
			// Deprecated: use v3
			module m
		`, true, "use v2"},
		{"ignore others",
			`
			// c1
			// Deprecated: use v2
			// c2
			module m
		`, true, "use v2"},
		{"must be capitalized",
			`
			// c1
			// deprecated: use v2
			// c2
			module m
		`, false, ""},
		{"suffix",
			`
			// c1
			module m // Deprecated: use v2
		`, true, "use v2",
		},
	} {
		mf, err := modfile.Parse("test", []byte(test.in), nil)
		if err != nil {
			t.Fatal(err)
		}
		gotIs, gotComment := isDeprecated(mf)
		if gotIs != test.wantIs || gotComment != test.wantComment {
			t.Errorf("%s: got (%t, %q), want(%t, %q)", test.name, gotIs, gotComment, test.wantIs, test.wantComment)
		}
	}
}

func TestIsRetracted(t *testing.T) {
	for _, test := range []struct {
		name          string
		file          string
		wantIs        bool
		wantRationale string
	}{
		{"no retract", "module M", false, ""},
		{"retracted", "module M\nretract v1.2.3", true, ""},
		{"retracted with comment", "module M\nretract v1.2.3 // bad  ", true, "bad"},
		{"retracted range", "module M\nretract [v1.2.0, v1.3.0] // bad", true, "bad"},
		{
			"not retracted", `
				module M
				retract [v1.2.0, v1.2.2]
				retract [v1.4.0, v1.99.0]
			`,
			false, "",
		},
	} {
		mf, err := modfile.Parse("test", []byte(test.file), nil)
		if err != nil {
			t.Fatal(err)
		}
		gotIs, gotRationale := isRetracted(mf, "v1.2.3")
		if gotIs != test.wantIs || gotRationale != test.wantRationale {
			t.Errorf("%s: got (%t, %q), want(%t, %q)", test.name, gotIs, gotRationale, test.wantIs, test.wantRationale)
		}
	}
}
