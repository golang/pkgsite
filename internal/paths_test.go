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
	} {
		got := CandidateModulePaths(test.in)
		if !cmp.Equal(got, test.want) {
			t.Errorf("%q: got %v, want %v", test.in, got, test.want)
		}
	}
}
