// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import "testing"

func TestLicensesAreRedistributable(t *testing.T) {
	tests := []struct {
		label    string
		licenses []*Metadata
		want     bool
	}{
		{
			label: "redistributable license",
			licenses: []*Metadata{
				{Type: "MIT", FilePath: "LICENSE"},
			},
			want: true,
		}, {
			label: "no redistributable license at root",
			licenses: []*Metadata{
				{Type: "MIT", FilePath: "bar/LICENSE"},
			},
			want: false,
		}, {
			label: "no license",
			want:  false,
		}, {
			label: "non-redistributable license",
			licenses: []*Metadata{
				{Type: "AGPL-3.0", FilePath: "LICENSE"},
			},
			want: false,
		}, {
			label: "multiple redistributable",
			licenses: []*Metadata{
				{Type: "BSD-3-Clause", FilePath: "LICENSE"},
				{Type: "MIT", FilePath: "bar/LICENSE"},
			},
			want: true,
		}, {
			label: "not all redistributable",
			licenses: []*Metadata{
				{Type: "BSD-3-Clause", FilePath: "LICENSE"},
				{Type: "AGPL-3.0", FilePath: "foo/LICENSE"},
				{Type: "MIT", FilePath: "foo/bar/LICENSE"},
			},
			want: false,
		}, {
			label: "at least one redistributable per directory",
			licenses: []*Metadata{
				{Type: "BSD-3-Clause", FilePath: "LICENSE"},
				{Type: "BSD-0-Clause", FilePath: "LICENSE.txt"},
				{Type: "AGPL-3.0", FilePath: "foo/LICENSE"},
				{Type: "MIT", FilePath: "foo/COPYING"},
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			if got := AreRedistributable(test.licenses); got != test.want {
				t.Errorf("licensesAreRedistributable([licenses]) = %t, want %t", got, test.want)
			}
		})
	}
}
