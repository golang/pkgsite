// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package osv

import (
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

func AffectsSemver(ranges []Range, v string) bool {
	if len(ranges) == 0 {
		// No ranges implies all versions are affected
		return true
	}
	var semverRangePresent bool
	for _, r := range ranges {
		if r.Type != RangeTypeSemver {
			continue
		}
		semverRangePresent = true
		if containsSemver(r, v) {
			return true
		}
	}
	// If there were no semver ranges present we
	// assume that all semvers are affected, similarly
	// to how to we assume all semvers are affected
	// if there are no ranges at all.
	return !semverRangePresent
}

// containsSemver checks if semver version v is in the
// range encoded by ar. If ar is not a semver range,
// returns false.
//
// Assumes that
//   - exactly one of Introduced or Fixed fields is set
//   - ranges in ar are not overlapping
//   - beginning of time is encoded with .Introduced="0"
//   - no-fix is not an event, as opposed to being an
//     event where Introduced="" and Fixed=""
func containsSemver(ar Range, v string) bool {
	if ar.Type != RangeTypeSemver {
		return false
	}
	if len(ar.Events) == 0 {
		return true
	}
	// Strip and then add the semver prefix so we can support bare versions,
	// versions prefixed with 'v', and versions prefixed with 'go'.
	v = CanonicalizeSemver(v)
	// Sort events by semver versions. Event for beginning
	// of time, if present, always comes first.
	sort.SliceStable(ar.Events, func(i, j int) bool {
		e1 := ar.Events[i]
		v1 := e1.Introduced
		if v1 == "0" {
			// -inf case.
			return true
		}
		if e1.Fixed != "" {
			v1 = e1.Fixed
		}
		e2 := ar.Events[j]
		v2 := e2.Introduced
		if v2 == "0" {
			// -inf case.
			return false
		}
		if e2.Fixed != "" {
			v2 = e2.Fixed
		}
		return semver.Compare(CanonicalizeSemver(v1), CanonicalizeSemver(v2)) < 0
	})
	var affected bool
	for _, e := range ar.Events {
		if !affected && e.Introduced != "" {
			affected = e.Introduced == "0" || semver.Compare(v, CanonicalizeSemver(e.Introduced)) >= 0
		} else if affected && e.Fixed != "" {
			affected = semver.Compare(v, CanonicalizeSemver(e.Fixed)) < 0
		}
	}
	return affected
}

// CanonicalizeSemver turns a SEMVER string into the canonical
// representation using the 'v' prefix, as used by the OSV format.
// Input may be a bare SEMVER ("1.2.3"), Go prefixed SEMVER ("go1.2.3"),
// or already canonical SEMVER ("v1.2.3").
func CanonicalizeSemver(s string) string {
	// Remove "go" prefix if needed.
	s = strings.TrimPrefix(s, "go")
	// Add "v" prefix if needed.
	if !strings.HasPrefix(s, "v") {
		s = "v" + s
	}
	return s
}

func LatestFixedVersion(ranges []Range) string {
	var latestFixed string
	for _, r := range ranges {
		if r.Type == "SEMVER" {
			for _, e := range r.Events {
				fixed := e.Fixed
				if fixed != "" && LessSemver(latestFixed, fixed) {
					latestFixed = fixed
				}
			}
			// If the vulnerability was re-introduced after the latest fix
			// we found, there is no latest fix for this range.
			for _, e := range r.Events {
				introduced := e.Introduced
				if introduced != "" && introduced != "0" && LessSemver(latestFixed, introduced) {
					latestFixed = ""
					break
				}
			}
		}
	}
	return latestFixed
}

// LessSemver returns whether v1 < v2, where v1 and v2 are
// semver versions with either a "v", "go" or no prefix.
func LessSemver(v1, v2 string) bool {
	return semver.Compare(CanonicalizeSemver(v1), CanonicalizeSemver(v2)) < 0
}
