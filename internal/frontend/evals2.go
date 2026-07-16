// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"time"
)

const (
	day   = 24 * time.Hour
	month = 31 * day
	year  = 365 * day
)

// ageString formats a time.Duration into a human-readable age string.
func ageString(dur time.Duration) string {
	pluralize := func(n time.Duration, s string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s", s)
		}
		return fmt.Sprintf("%d %ss", n, s)
	}

	if dur < day {
		return "less than a day"
	}
	if dur < month {
		return pluralize(dur/day, "day")
	}
	if dur < year {
		monthStr := pluralize(dur/month, "month")
		days := (dur % month) / day
		if days == 0 {
			return monthStr
		}
		return monthStr + ", " + pluralize(days, "day")
	}

	yearStr := pluralize(dur/year, "year")
	months := (dur % year) / month
	if months == 0 {
		return yearStr
	}
	return yearStr + ", " + pluralize(months, "month")
}
