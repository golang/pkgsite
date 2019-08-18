// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNextPrefixAccount(t *testing.T) {
	for _, tc := range []struct {
		path, want1, want2 string
	}{
		{"", "", ""},
		{"github.com", "github.com", ""},
		{"github.com/user", "github.com/user", ""},
		{"github.com/user/repo", "github.com/user/", "github.com/user/repo"},
		{"github.com/user/repo/more", "github.com/user/", "github.com/user/repo/"},
		{"hub.jazz.net/git/user/project/more", "hub.jazz.net/git/user/", "hub.jazz.net/git/user/project/"},
		{"k8s.io/a", "k8s.io/", "k8s.io/a"},
		{"k8s.io/a/b", "k8s.io/", "k8s.io/a/"},
		{"example.com", "example.com", ""},
		{"example.com/foo", "example.com/", "example.com/foo"},
		{"example.com/foo/bar", "example.com/", "example.com/foo/"},
	} {
		got1 := nextPrefixAccount(tc.path, "")
		if got1 != tc.want1 {
			t.Errorf(`nextPrefixAccount(%q, "") = %q, want %q`, tc.path, got1, tc.want1)
			continue
		}
		got2 := nextPrefixAccount(tc.path, got1)
		if got2 != tc.want2 {
			t.Errorf(`nextPrefixAccount(%q, %q) = %q, want %q`, tc.path, got1, got2, tc.want2)
			continue
		}
		if got2 == "" {
			continue
		}
		got3 := nextPrefixAccount(tc.path, got2)
		if got3 != "" {
			t.Errorf(`nextPrefixAccount(%q, %q) = %q, want ""`, tc.path, got2, got3)
		}
	}
}

func TestPrefixSections(t *testing.T) {
	for _, tc := range []struct {
		lines []string
		want  []*Section
	}{
		{
			[]string{"foo.com/a", "bar.com/a", "baz.com/a"},
			[]*Section{
				{"foo.com/a", nil, 0},
				{"bar.com/a", nil, 0},
				{"baz.com/a", nil, 0},
			},
		},
		{
			[]string{"k8s.io/a", "k8s.io/b", "k8s.io/c"},
			[]*Section{
				{
					"k8s.io/",
					[]*Section{
						{"k8s.io/a", nil, 0},
						{"k8s.io/b", nil, 0},
						{"k8s.io/c", nil, 0},
					},
					3,
				},
			},
		},
		{
			[]string{
				"github.com/eliben/gocdkx/blob",
				"github.com/eliben/gocdkx/blob/azureblob",
				"github.com/eliben/gocdkx/blob/fileblob",
				"github.com/eliben/gocdkx/internal/docstore/dynamodocstore",
				"github.com/eliben/gocdkx/internal/testing/octest",
				"github.com/eliben/gocdkx/internal/trace",
				"github.com/eliben/gocdkx/pubsub",
				"github.com/eliben/gocdkx/pubsub/awspubsub",
			},
			[]*Section{
				{
					"github.com/eliben/gocdkx/",
					[]*Section{
						{"github.com/eliben/gocdkx/blob", nil, 0},
						{"github.com/eliben/gocdkx/blob/azureblob", nil, 0},
						{"github.com/eliben/gocdkx/blob/fileblob", nil, 0},
						{"github.com/eliben/gocdkx/internal/docstore/dynamodocstore", nil, 0},
						{"github.com/eliben/gocdkx/internal/testing/octest", nil, 0},
						{"github.com/eliben/gocdkx/internal/trace", nil, 0},
						{"github.com/eliben/gocdkx/pubsub", nil, 0},
						{"github.com/eliben/gocdkx/pubsub/awspubsub", nil, 0},
					},
					8,
				},
			},
		},
	} {
		got := Sections(tc.lines, nextPrefixAccount)
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("%v: mismatch (-want, +got):\n%s", tc.lines, diff)
		}
	}
}
