// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cron

import (
	"regexp"
	"strings"
	"testing"
)

func TestHashTaskName(t *testing.T) {
	tests := []struct {
		name, wantPreserved string
	}{
		{"github.com/pkg/errors@v1.0.0", "errors"},
		{"github.com/pkg-errors@v1.0.0", "errors"},
		{"github-com-pkg-errors@v1.0.0", "errors"},
		{"A", "A"},
		{"a", "a"},
	}

	seen := make(map[string]bool)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h, err := hashTaskName(test.name)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(h)
			if seen[h] {
				t.Errorf("duplicated hash name %s for %s", h, test.name)
			}
			seen[h] = true
			if !strings.Contains(h, test.wantPreserved) {
				t.Errorf("hashed name %s did not preserve %s", h, test.wantPreserved)
			}

			matched, err := regexp.Match(`^[[0-9a-zA-Z_-]+$`, []byte(h))
			if err != nil {
				t.Fatal(err)
			}
			if !matched {
				t.Errorf("hashed name %s contains invalid characters", h)
			}
		})
	}
}
