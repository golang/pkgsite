// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"testing"

	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestShouldUpdateSymbolHistory(t *testing.T) {
	testSym := "Foo"
	for _, test := range []struct {
		name       string
		newVersion string
		oldHist    map[string]string
		want       bool
	}{
		{
			name:    "should update when new version is older",
			oldHist: map[string]string{testSym: "v1.2.3"},
			want:    true,
		},
		{
			name:    "should update when symbol does not exist",
			oldHist: map[string]string{},
			want:    true,
		},
		{
			name:    "should update when new version is the same",
			oldHist: map[string]string{testSym: sample.VersionString},
			want:    true,
		},
		{
			name:    "should not update when new version is newer",
			oldHist: map[string]string{testSym: "v0.1.0"},
			want:    false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldUpdateSymbolHistory(testSym, sample.VersionString, test.oldHist); got != test.want {
				t.Errorf("shouldUpdateSymbolHistory(%q, %q, %+v) = %t; want = %t",
					testSym, sample.VersionString, test.oldHist, got, test.want)
			}
		})
	}
}
