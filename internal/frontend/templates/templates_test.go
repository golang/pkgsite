// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package templates

import (
	"testing"
)

func TestStripScheme(t *testing.T) {
	for _, test := range []struct {
		url, want string
	}{
		{"http://github.com", "github.com"},
		{"https://github.com/path/to/something", "github.com/path/to/something"},
		{"example.com", "example.com"},
		{"chrome-extension://abcd", "abcd"},
		{"nonwellformed.com/path?://query=1", "query=1"},
	} {
		if got := stripScheme(test.url); got != test.want {
			t.Errorf("%q: got %q, want %q", test.url, got, test.want)
		}
	}
}

func TestScoreBoxClasses(t *testing.T) {
	for _, test := range []struct {
		score, maxScore int
		want            []string
	}{
		{score: 2, maxScore: 5, want: []string{scoreBoxMarked, scoreBoxMarked, scoreBoxUnmarked, scoreBoxUnmarked, scoreBoxUnmarked}},
		{score: 0, maxScore: 3, want: []string{scoreBoxUnmarked, scoreBoxUnmarked, scoreBoxUnmarked}},
		{score: 3, maxScore: 3, want: []string{scoreBoxMarked, scoreBoxMarked, scoreBoxMarked}},
		{score: 5, maxScore: 3, want: []string{scoreBoxMarked, scoreBoxMarked, scoreBoxMarked}},
		{score: -1, maxScore: 3, want: []string{scoreBoxUnmarked, scoreBoxUnmarked, scoreBoxUnmarked}},
		{score: 2, maxScore: 0, want: nil},
	} {
		got := scoreBoxClasses(test.score, test.maxScore)
		if len(got) != len(test.want) {
			t.Errorf("scoreBoxClasses(%d, %d) len = %d, want %d", test.score, test.maxScore, len(got), len(test.want))
			continue
		}
		for i := range got {
			if got[i] != test.want[i] {
				t.Errorf("scoreBoxClasses(%d, %d)[%d] = %q, want %q", test.score, test.maxScore, i, got[i], test.want[i])
			}
		}
	}
}
