// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/vulndb/osv"
)

// A Vuln contains information to display about a vulnerability.
type Vuln struct {
	// A description of the vulnerability, or the problem in obtaining it.
	Details string
	// The version is which the vulnerability has been fixed.
	FixedVersion string
}

type vulnEntriesFunc func(string) ([]*osv.Entry, error)

// Vulns obtains vulnerability information for the given package.
// If packagePath is empty, it returns all entries for the module at version.
// The getVulnEntries function should retrieve all entries for the given module path.
// It is passed to facilitate testing.
func Vulns(modulePath, version, packagePath string, getVulnEntries vulnEntriesFunc) (_ []Vuln, err error) {
	defer derrors.Wrap(&err, "Vulns(%q, %q)", modulePath, version)

	// Get all the vulns for this module.
	entries, err := getVulnEntries(modulePath)
	if err != nil {
		return nil, err
	}
	// Each entry describes a single vuln. Select the ones that apply to this
	// package at this version.
	var vulns []Vuln
	for _, e := range entries {
		if (packagePath == "" || e.Package.Name == packagePath) && e.Affects.AffectsSemver(version) {
			// Choose the latest fixed version, if any.
			var fixed string
			for _, r := range e.Affects.Ranges {
				if r.Fixed != "" && (fixed == "" || semver.Compare(r.Fixed, fixed) > 0) {
					fixed = r.Fixed
				}
			}
			vulns = append(vulns, Vuln{
				Details: e.Details,
				// TODO(golang/go#48223): handle stdlib versions
				FixedVersion: "v" + fixed,
			})
		}
	}
	return vulns, nil
}
