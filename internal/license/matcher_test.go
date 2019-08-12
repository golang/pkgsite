// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMatch(t *testing.T) {

	fake := func(path string) *License {
		return &License{
			Metadata: &Metadata{
				FilePath: path,
				Types:    []string{"MIT"},
			},
		}
	}

	tests := []struct {
		label    string
		licenses []*License
		dir      string
		want     []string
	}{
		{
			label:    "matches root license",
			licenses: []*License{fake("LICENSE")},
			dir:      "foo/bar",
			want:     []string{"LICENSE"},
		}, {
			label:    "handles empty dir",
			licenses: []*License{fake("LICENSE"), fake("foo/LICENSE")},
			dir:      "",
			want:     []string{"LICENSE"},
		}, {
			label:    "handles current dir",
			licenses: []*License{fake("LICENSE"), fake("foo/LICENSE")},
			dir:      ".",
			want:     []string{"LICENSE"},
		}, {
			label:    "doesn't allow absolute paths",
			licenses: []*License{fake("LICENSE")},
			dir:      "/foo/bar",
			want:     nil,
		}, {
			label:    "matches nested license",
			licenses: []*License{fake("LICENSE"), fake("foo/LICENSE"), fake("bar/LICENSE")},
			dir:      "foo",
			want:     []string{"LICENSE", "foo/LICENSE"},
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			matcher := NewMatcher(test.licenses)
			matched := matcher.Match(test.dir)
			var got []string
			for _, m := range matched {
				got = append(got, m.FilePath)
			}
			sort.Strings(got)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("matcher.Match(%q) mismatch (-want +got):\n%s", test.dir, diff)
			}
		})
	}
}
