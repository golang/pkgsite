// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vuln

import "regexp"

var (
	goRegexp   = regexp.MustCompile("^GO-[0-9]{4}-[0-9]{4,}$")
	cveRegexp  = regexp.MustCompile("^CVE-[0-9]{4}-[0-9]+$")
	ghsaRegexp = regexp.MustCompile("^GHSA-.{4}-.{4}-.{4}$")
)

// IsGoID returns whether s is a valid Go vulnerability ID.
func IsGoID(s string) bool {
	return goRegexp.MatchString(s)
}

// IsAlias returns whether s is a valid vulnerability alias
// (CVE or GHSA).
func IsAlias(s string) bool {
	return cveRegexp.MatchString(s) || ghsaRegexp.MatchString(s)
}
