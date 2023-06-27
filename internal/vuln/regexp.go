// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import (
	"regexp"
	"strings"
)

const (
	ci    = "(?i)" // case-insensitive
	goRE  = "^GO-[0-9]{4}-[0-9]{4,}$"
	cveRE = "^CVE-[0-9]{4}-[0-9]+$"
	// Regexp adapted from https://github.com/github/advisory-database.
	ghsaRE = "^(GHSA)((-[23456789cfghjmpqrvwx]{4}){3})$"
)

// Case-insensitive regexps for vuln IDs/aliases.
var (
	goID   = regexp.MustCompile(ci + goRE)
	cveID  = regexp.MustCompile(ci + cveRE)
	ghsaID = regexp.MustCompile(ci + ghsaRE)
)

// Canonical returns the canonical form of the given Go ID string
// by correcting the case.
//
// If no canonical form can be found, returns false.
func CanonicalGoID(id string) (_ string, ok bool) {
	if goID.MatchString(id) {
		return strings.ToUpper(id), true
	}
	return "", false
}

// Canonical returns the canonical form of the given alias ID string
// (a CVE or GHSA id) by correcting the case.
//
// If no canonical form can be found, returns false.
func CanonicalAlias(id string) (_ string, ok bool) {
	if cveID.MatchString(id) {
		return strings.ToUpper(id), true
	}
	parts := ghsaID.FindStringSubmatch(id)
	if len(parts) != 4 {
		return "", false
	}
	return strings.ToUpper(parts[1]) + strings.ToLower(parts[2]), true
}
