// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package licenses defines license metadata types and redistributability policy.
package licenses

import (
	"fmt"
	"sort"
)

// Metadata holds information extracted from a license file.
type Metadata struct {
	// Types is the set of license types, as determined by the licensecheck package.
	Types []string
	// FilePath is the '/'-separated path to the license file in the module zip,
	// relative to the contents directory.
	FilePath string
	Coverage Coverage
}

// Coverage describes how much of a file is covered by recognized licenses.
// Its structure matches github.com/google/licensecheck.Coverage so JSON
// serialization in the database is unchanged.
type Coverage struct {
	Percent float64
	Match   []Match
}

// Match describes how a section of text matches a license.
// Its structure matches github.com/google/licensecheck.Match.
type Match struct {
	ID    string
	Type  uint
	Start int
	End   int
	IsURL bool
}

// A License is a classified license file path and its contents.
type License struct {
	*Metadata
	Contents []byte
}

// RemoveNonRedistributableData methods removes the license contents
// if the license is non-redistributable.
func (l *License) RemoveNonRedistributableData() {
	if !Redistributable(l.Types) {
		l.Contents = nil
	}
}

var (
	FileNames = []string{
		"COPYING",
		"COPYING.md",
		"COPYING.markdown",
		"COPYING.txt",
		"COPYING.LESSER",
		"COPYING.LESSER.md",
		"COPYING.LESSER.markdown",
		"COPYING.LESSER.txt",
		"LICENCE",
		"LICENCE.md",
		"LICENCE.markdown",
		"LICENCE.txt",
		"LICENSE",
		"LICENSE.md",
		"LICENSE.markdown",
		"LICENSE.txt",
		"LICENSE-2.0.txt",
		"LICENCE-2.0.txt",
		"LICENSE-APACHE",
		"LICENCE-APACHE",
		"LICENSE-APACHE-2.0.txt",
		"LICENCE-APACHE-2.0.txt",
		"LICENSE-MIT",
		"LICENCE-MIT",
		"LICENSE.MIT",
		"LICENCE.MIT",
		"LICENSE.code",
		"LICENCE.code",
		"LICENSE.docs",
		"LICENCE.docs",
		"LICENSE.rst",
		"LICENCE.rst",
		"MIT-LICENSE",
		"MIT-LICENCE",
		"MIT-LICENSE.md",
		"MIT-LICENCE.md",
		"MIT-LICENSE.markdown",
		"MIT-LICENCE.markdown",
		"MIT-LICENSE.txt",
		"MIT-LICENCE.txt",
		"MIT_LICENSE",
		"MIT_LICENCE",
		"UNLICENSE",
		"UNLICENCE",
	}

	// standardRedistributableLicenseTypes is the list of license types, as reported by
	// licensecheck, that allow redistribution, and also have a name that is an OSI or SPDX
	// identifier.
	standardRedistributableLicenseTypes = []string{
		// Licenses acceptable by OSI.
		"AFL-3.0",
		"AGPL-3.0",
		"AGPL-3.0-only",
		"AGPL-3.0-or-later",
		"Apache-1.1",
		"Apache-2.0",
		"Artistic-2.0",
		"BlueOak-1.0.0",
		"0BSD",
		"BSD-1-Clause",
		"BSD-2-Clause",
		"BSD-2-Clause-Patent",
		"BSD-2-Clause-Views",
		"BSD-3-Clause",
		"BSD-3-Clause-Clear",
		"BSD-3-Clause-LBNL",
		"BSD-3-Clause-Open-MPI",
		"BSD-4-Clause",
		"BSD-4-Clause-UC",
		"BSL-1.0",
		"CC-BY-3.0",
		"CC-BY-4.0",
		"CC-BY-SA-3.0",
		"CC-BY-SA-4.0",
		"CECILL-2.1",
		"CC0-1.0",
		"EPL-1.0",
		"EPL-2.0",
		"EUPL-1.1",
		"EUPL-1.2",
		"GPL-2.0",
		"GPL-2.0-only",
		"GPL-2.0-or-later",
		"GPL-3.0",
		"GPL-3.0-only",
		"GPL-3.0-or-later",
		"HPND",
		"ICU",
		"ISC",
		"JSON",
		"LGPL-2.1",
		"LGPL-2.1-or-later",
		"LGPL-3.0",
		"LGPL-3.0-or-later",
		"MIT",
		"MIT-0",
		"MPL-2.0",
		"MPL-2.0-no-copyleft-exception",
		"MulanPSL-2.0",
		"NIST-PD",
		"NIST-PD-fallback",
		"NCSA",
		"OpenSSL",
		"OSL-3.0",
		"PostgreSQL", // TODO: ask legal
		"Python-2.0",
		"Unlicense",
		"UPL-1.0",
		"Zlib",
	}

	// licenseURLOverrides provides a mapping from a license identifier to its
	// correct URL. This is used for licenses where the URL has changed or does
	// not follow the standard pattern.
	licenseURLOverrides = map[string]string{
		// HPND is a retired license, and the URL has changed.
		"HPND": "https://opensource.org/license/historical-php",
	}

	// These aren't technically licenses, but they are recognized by
	// licensecheck and safe to ignore.
	ignorableLicenseTypes = map[string]bool{
		"CC-Notice":          true,
		"GooglePatentClause": true,
		"GooglePatentsFile":  true,
		"blessing":           true,
		"OFL-1.1":            true, // concerns fonts only
	}

	// redistributableLicenseTypes is the set of license types, as reported by
	// licensecheck, that allow redistribution. It consists of the standard
	// types along with some exception types.
	redistributableLicenseTypes = map[string]bool{}
)

func init() {
	for _, t := range standardRedistributableLicenseTypes {
		redistributableLicenseTypes[t] = true
	}
	// Add here all other types defined in the exceptions.
	redistributableLicenseTypes["Freetype"] = true

}

// nonOSILicenses lists licenses that are not approved by OSI.
var nonOSILicenses = map[string]bool{
	"AGPL-3.0-only":                 true,
	"AGPL-3.0-or-later":             true,
	"BlueOak-1.0.0":                 true,
	"BSD-2-Clause-Views":            true,
	"BSD-3-Clause-Clear":            true,
	"BSD-3-Clause-LBNL":             true,
	"BSD-3-Clause-Open-MPI":         true,
	"BSD-4-Clause":                  true,
	"BSD-4-Clause-UC":               true,
	"CC-BY-3.0":                     true,
	"CC-BY-4.0":                     true,
	"CC-BY-SA-3.0":                  true,
	"CC-BY-SA-4.0":                  true,
	"CC0-1.0":                       true,
	"GPL-2.0-only":                  true,
	"GPL-2.0-or-later":              true,
	"GPL-3.0-only":                  true, // it is referred in https://opensource.org/licenses/GPL-3.0
	"GPL-3.0-or-later":              true,
	"LGPL-2.1-or-later":             true,
	"LGPL-3.0-or-later":             true,
	"MPL-2.0-no-copyleft-exception": true,
	"NIST-PD":                       true,
	"NIST-PD-fallback":              true,
	"JSON":                          true,
	"OpenSSL":                       true,
	"UPL-1.0":                       true,
}

// AcceptedLicenseInfo describes a license that is accepted by the discovery site.
type AcceptedLicenseInfo struct {
	Name string
	URL  string
}

// AcceptedLicenses returns a sorted slice of license types that are accepted as
// redistributable. Its result is intended to be displayed to users.
func AcceptedLicenses() []AcceptedLicenseInfo {
	var lics []AcceptedLicenseInfo
	for _, identifier := range standardRedistributableLicenseTypes {
		var link string
		if override, ok := licenseURLOverrides[identifier]; ok {
			link = override
		} else if nonOSILicenses[identifier] {
			link = fmt.Sprintf("https://spdx.org/licenses/%s.html", identifier)
		} else {
			link = fmt.Sprintf("https://opensource.org/licenses/%s", identifier)
		}
		lics = append(lics, AcceptedLicenseInfo{identifier, link})
	}
	sort.Slice(lics, func(i, j int) bool { return lics[i].Name < lics[j].Name })
	return lics
}

// Redistributable reports whether the set of license types establishes that a
// module or package is redistributable.
// All the licenses we see that are relevant must be redistributable, and
// we must see at least one such license.
func Redistributable(licenseTypes []string) bool {
	sawRedist := false
	for _, t := range licenseTypes {
		if ignorableLicenseTypes[t] {
			continue
		}
		if !redistributableLicenseTypes[t] {
			return false
		}
		sawRedist = true
	}
	return sawRedist
}
