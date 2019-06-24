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
				{Types: []string{"MIT"}, FilePath: "LICENSE"},
			},
			want: true,
		}, {
			label: "no redistributable license at root",
			licenses: []*Metadata{
				{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
			},
			want: false,
		}, {
			label: "no license",
			want:  false,
		}, {
			label: "non-redistributable license",
			licenses: []*Metadata{
				{Types: []string{"BADLICENSE"}, FilePath: "LICENSE"},
			},
			want: false,
		}, {
			label: "multiple redistributable",
			licenses: []*Metadata{
				{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
				{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
			},
			want: true,
		}, {
			label: "not all redistributable",
			licenses: []*Metadata{
				{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
				{Types: []string{"BADLICENSE"}, FilePath: "foo/LICENSE"},
				{Types: []string{"MIT"}, FilePath: "foo/bar/LICENSE"},
			},
			want: false,
		}, {
			label: "multiple redistributable in a single file",
			licenses: []*Metadata{
				{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
				{Types: []string{"AGPL-3.0", "MIT"}, FilePath: "foo/LICENSE"},
			},
			want: true,
		}, {
			label: "single file with one bad license",
			licenses: []*Metadata{
				{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
				{Types: []string{"AGPL-3.0", "BADLICENSE"}, FilePath: "foo/LICENSE"},
			},
			want: false,
		}, {
			label: "single file with no license",
			licenses: []*Metadata{
				{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
				{FilePath: "foo/LICENSE"},
			},
			want: false,
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
