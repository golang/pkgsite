// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCandidateModulePaths(t *testing.T) {
	for _, test := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{".", nil},
		{"///foo", nil},
		{"github.com/google", nil},
		{"std", []string{"std"}},
		{"encoding/json", []string{"std"}},
		{
			"example.com/green/eggs/and/ham",
			[]string{
				"example.com/green/eggs/and/ham",
				"example.com/green/eggs/and",
				"example.com/green/eggs",
				"example.com/green",
				"example.com",
			},
		},
		{
			"github.com/google/go-cmp/cmp",
			[]string{"github.com/google/go-cmp/cmp", "github.com/google/go-cmp"},
		},
		{
			"bitbucket.org/ok/sure/no$dollars/allowed",
			[]string{"bitbucket.org/ok/sure"},
		},
		{
			// A module path cannot end in "v1".
			"k8s.io/klog/v1",
			[]string{"k8s.io/klog", "k8s.io"},
		},
		{
			// launchpad.net allows two-element module paths (go-get meta-tag discovery).
			"launchpad.net/goose",
			[]string{"launchpad.net/goose", "launchpad.net"},
		},
		{
			"launchpad.net/project/series",
			[]string{
				"launchpad.net/project/series",
				"launchpad.net/project",
				"launchpad.net",
			},
		},
		{
			"launchpad.net/~user/project",
			[]string{
				"launchpad.net/~user/project",
				"launchpad.net/~user",
				"launchpad.net",
			},
		},
	} {
		got := CandidateModulePaths(test.in)
		if !cmp.Equal(got, test.want) {
			t.Errorf("%q: got %v, want %v", test.in, got, test.want)
		}
	}
}
