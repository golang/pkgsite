// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The evaluations page, which displays signals for
// package and module quality.

package frontend

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/version"
)

type evalType struct {
	Label       string // displayed on the page
	Description string // displayed from "?" icon
	MaxScore    int    // number of bars
}

var (
	licenseEval = &evalType{
		Label: "License for this package or module",
		Description: `The module license.
0: no license
1: non-redistributable license
2: redistributable license`,
		MaxScore: 2,
	}

	moduleVersionEval = &evalType{
		Label: "Module version",
		Description: `Version of this module.
0: untagged
1: tagged, unstable (v0)
2: tagged, stable (v1 or higher)`,
		MaxScore: 2,
	}
)

type eval struct {
	Type  *evalType
	Value string // displayed on the page
	Score int    // number of colored bars
}

type evalsDetails struct {
	Evals []eval
}

func fetchEvalsDetails(ctx context.Context, ds internal.DataSource, um *internal.UnitMeta) (*evalsDetails, error) {
	u, err := ds.GetUnit(ctx, um, internal.WithLicenses, internal.BuildContext{})
	if err != nil {
		return nil, err
	}

	licEval := eval{Type: licenseEval}
	switch {
	case len(u.LicenseContents) == 0:
		licEval.Score = 0
		licEval.Value = "no license"
	case !u.IsRedistributable:
		licEval.Score = 1
		licEval.Value = "non-redistributable license"
	default:
		licEval.Score = 2
		licEval.Value = "redistributable license"
	}

	modEval := eval{Type: moduleVersionEval}
	versionType, err := version.ParseType(um.Version)
	if err != nil {
		return nil, err
	}
	switch {
	case version.IsPseudo(um.Version) || !semver.IsValid(um.Version):
		modEval.Score = 0
		modEval.Value = "untagged"
	case semver.Major(um.Version) == "v0" || versionType == version.TypePrerelease:
		modEval.Score = 1
		modEval.Value = "tagged, unstable"
	default:
		modEval.Score = 2
		modEval.Value = "tagged, stable"
	}

	return &evalsDetails{
		Evals: []eval{licEval, modEval},
	}, nil
}

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
