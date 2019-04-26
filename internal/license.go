// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import "path"

// RedistributableLicenses defines the set of licenses that are permissive of
// redistribution.
var RedistributableLicenses = map[string]bool{
	"Apache-2.0":           true,
	"Artistic-2.0":         true,
	"BSD-2-Clause-FreeBSD": true,
	"BSD-3-Clause":         true,
	"BSL-1.0":              true,
	"CC-BY-4.0":            true,
	"CC0-1.0":              true,
	"GPL2":                 true,
	"GPL3":                 true,
	"ISC":                  true,
	"JSON":                 true,
	"LGPL-2.1":             true,
	"LGPL-3.0":             true,
	"MIT":                  true,
	"Unlicense":            true,
	"Zlib":                 true,
}

// LicenseFileNames defines the set of filenames to be considered for license
// extraction.
var LicenseFileNames = map[string]bool{
	"LICENSE":     true,
	"LICENSE.md":  true,
	"LICENSE.txt": true,
	"COPYING":     true,
	"COPYING.md":  true,
}

// licensesAreRedistributable determines whether content subject to the given
// licenses should be considered redistributable. The current algorithm for
// this is to ensure that (1) There is at least one license permitting
// redistribution in the root directory, and (2) every directory containing an
// applicable license contains at least one license that is redistributable.
func licensesAreRedistributable(licenses []*LicenseInfo) bool {
	byDir := make(map[string][]string)
	for _, l := range licenses {
		dir := path.Dir(l.FilePath)
		byDir[dir] = append(byDir[dir], l.Type)
	}

	anyRedistributable := func(lics []string) bool {
		for _, l := range lics {
			if RedistributableLicenses[l] {
				return true
			}
		}
		return false
	}

	// There must be a license at the module level, otherwise it's can't be
	// redistributable.  We'll check if any top-level license is redistributable
	// below.
	if len(byDir["."]) == 0 {
		return false
	}

	for _, lics := range byDir {
		if !anyRedistributable(lics) {
			return false
		}
	}

	return true
}
