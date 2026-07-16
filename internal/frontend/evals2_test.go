// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"
	"time"
)

func TestAgeString(t *testing.T) {
	testCases := []struct {
		d    time.Duration
		want string
	}{
		{0, "less than a day"},
		{12 * time.Hour, "less than a day"},
		{23*time.Hour + 59*time.Minute, "less than a day"},
		{day, "1 day"},
		{2 * day, "2 days"},
		{23 * day, "23 days"},
		{30 * day, "30 days"},
		{31 * day, "1 month"},
		{31*day + 1*day, "1 month, 1 day"},
		{31*day + 5*day, "1 month, 5 days"},
		{2 * month, "2 months"},
		{2*month + 1*day, "2 months, 1 day"},
		{2*month + 3*day, "2 months, 3 days"},
		{11*month + 23*day, "11 months, 23 days"},
		{year, "1 year"},
		{year + 1*day, "1 year"}, // 1 day is 0 months remainder
		{year + 1*month, "1 year, 1 month"},
		{year + 2*month + 5*day, "1 year, 2 months"},
		{2 * year, "2 years"},
		{2*year + 5*month, "2 years, 5 months"},
		{2*year + 1*month + 30*day, "2 years, 1 month"},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got := ageString(tc.d)
			if got != tc.want {
				t.Errorf("ageString(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}
