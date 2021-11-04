// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"path"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/vuln/osv"
)

// A Vuln contains information to display about a vulnerability.
type Vuln struct {
	// The vulndb ID.
	ID string
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
// If there is an error, Vulns returns a single Vuln that describes the error.
func Vulns(modulePath, version, packagePath string, getVulnEntries vulnEntriesFunc) []Vuln {
	vs, err := vulns(modulePath, version, packagePath, getVulnEntries)
	if err != nil {
		return []Vuln{{Details: fmt.Sprintf("could not get vulnerability data: %v", err)}}
	}
	return vs
}

func vulns(modulePath, version, packagePath string, getVulnEntries vulnEntriesFunc) (_ []Vuln, err error) {
	defer derrors.Wrap(&err, "vulns(%q, %q, %q)", modulePath, version, packagePath)

	// Get all the vulns for this module.
	entries, err := getVulnEntries(modulePath)
	if err != nil {
		return nil, err
	}
	// Each entry describes a single vuln. Select the ones that apply to this
	// package at this version.
	var vulns []Vuln
	for _, e := range entries {
		if vuln, ok := entryVuln(e, packagePath, version); ok {
			vulns = append(vulns, vuln)
		}
	}
	return vulns, nil
}

func entryVuln(e *osv.Entry, packagePath, version string) (Vuln, bool) {
	for _, a := range e.Affected {
		if (packagePath == "" || a.Package.Name == packagePath) && a.Ranges.AffectsSemver(version) {
			// Choose the latest fixed version, if any.
			var fixed string
			for _, r := range a.Ranges {
				if r.Type == osv.TypeGit {
					continue
				}
				for _, re := range r.Events {
					if re.Fixed != "" && (fixed == "" || semver.Compare(re.Fixed, fixed) > 0) {
						fixed = re.Fixed
					}
				}
			}
			if fixed != "" {
				fixed = "v" + fixed
			}
			return Vuln{
				ID:      e.ID,
				Details: e.Details,
				// TODO(golang/go#48223): handle stdlib versions
				FixedVersion: fixed,
			}, true
		}
	}
	return Vuln{}, false
}

const vulndbURL = "https://go.googlesource.com/vulndb/+/refs/heads/master/reports/"

func (s *Server) serveVuln(w http.ResponseWriter, r *http.Request, _ internal.DataSource) error {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/golang.org/x/vulndb", http.StatusFound)
		return nil
	}
	if r.URL.Path == "/list" {
		http.Redirect(w, r, vulndbURL, http.StatusFound)
		return nil
	}
	// Otherwise, the path should be "/ID".
	http.Redirect(w, r, path.Join(vulndbURL, r.URL.Path+".yaml"), http.StatusFound)
	return nil
}
