// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"
	"time"

	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestAbsoluteTime(t *testing.T) {
	now := sample.NowTruncated()
	testCases := []struct {
		name         string
		date         time.Time
		absoluteTime string
	}{
		{
			name:         "today",
			date:         now.Add(time.Hour),
			absoluteTime: now.Add(time.Hour).Format("Jan _2, 2006"),
		},
		{
			name:         "a_week_ago",
			date:         now.Add(time.Hour * 24 * -5),
			absoluteTime: now.Add(time.Hour * 24 * -5).Format("Jan _2, 2006"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			absoluteTime := absoluteTime(tc.date)

			if absoluteTime != tc.absoluteTime {
				t.Errorf("absoluteTime(%q) = %s, want %s", tc.date, absoluteTime, tc.absoluteTime)
			}
		})
	}
}
