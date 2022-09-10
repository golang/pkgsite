// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vulns provides utilities to interact with vuln APIs.
package vulns

import (
	"context"
	"fmt"
	"go/token"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/vuln/osv"
)

// A Vuln contains information to display about a vulnerability.
type Vuln struct {
	// The vulndb ID.
	ID string
	// A description of the vulnerability, or the problem in obtaining it.
	Details string
}

type vulnEntriesFunc func(context.Context, string) ([]*osv.Entry, error)

// VulnsForPackage obtains vulnerability information for the given package.
// If packagePath is empty, it returns all entries for the module at version.
// The getVulnEntries function should retrieve all entries for the given module path.
// It is passed to facilitate testing.
// If there is an error, VulnsForPackage returns a single Vuln that describes the error.
func VulnsForPackage(ctx context.Context, modulePath, version, packagePath string, getVulnEntries vulnEntriesFunc) []Vuln {
	vs, err := vulnsForPackage(ctx, modulePath, version, packagePath, getVulnEntries)
	if err != nil {
		return []Vuln{{Details: fmt.Sprintf("could not get vulnerability data: %v", err)}}
	}
	return vs
}

func vulnsForPackage(ctx context.Context, modulePath, version, packagePath string, getVulnEntries vulnEntriesFunc) (_ []Vuln, err error) {
	defer derrors.Wrap(&err, "vulns(%q, %q, %q)", modulePath, version, packagePath)

	if getVulnEntries == nil {
		return nil, nil
	}
	// Get all the vulns for this module.
	entries, err := getVulnEntries(ctx, modulePath)
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

// AffectedPackage holds information about a package affected by a certain vulnerability.
type AffectedPackage struct {
	PackagePath string
	Versions    string
	// List of exported affected symbols. Empty list
	// implies all symbols in the package are affected.
	Symbols []string
}

// OSVEntry holds an OSV entry and provides additional methods.
type OSVEntry struct {
	*osv.Entry
}

// AffectedModulesAndPackages returns a list of names affected by a vuln.
func (e OSVEntry) AffectedModulesAndPackages() []string {
	var affected []string
	for _, a := range e.Affected {
		switch a.Package.Name {
		case "stdlib", "toolchain":
			// Name specific standard library packages and tools.
			for _, p := range a.EcosystemSpecific.Imports {
				affected = append(affected, p.Path)
			}
		default:
			// Outside the standard library, name the module.
			affected = append(affected, a.Package.Name)
		}
	}
	return affected
}

func entryVuln(e *osv.Entry, packagePath, version string) (Vuln, bool) {
	for _, a := range e.Affected {
		if !a.Ranges.AffectsSemver(version) {
			continue
		}
		if packageMatches := func() bool {
			if packagePath == "" {
				return true //  match module only
			}
			if len(a.EcosystemSpecific.Imports) == 0 {
				return true // no package info available, so match on module
			}
			for _, p := range a.EcosystemSpecific.Imports {
				if packagePath == p.Path {
					return true // package matches
				}
			}
			return false
		}(); !packageMatches {
			continue
		}
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
		return Vuln{
			ID:      e.ID,
			Details: e.Details,
		}, true
	}
	return Vuln{}, false
}

// A pair is like an osv.Range, but each pair is a self-contained 2-tuple
// (introduced version, fixed version).
type pair struct {
	intro, fixed string
}

// collectRangePairs turns a slice of osv Ranges into a more manageable slice of
// formatted version pairs.
func collectRangePairs(a osv.Affected) []pair {
	var (
		ps     []pair
		p      pair
		prefix string
	)
	if stdlib.Contains(a.Package.Name) {
		prefix = "go"
	} else {
		prefix = "v"
	}
	for _, r := range a.Ranges {
		isSemver := r.Type == osv.TypeSemver
		for _, v := range r.Events {
			if v.Introduced != "" {
				// We expected Introduced and Fixed to alternate, but if
				// p.intro != "", then they they don't.
				// Keep going in that case, ignoring the first Introduced.
				p.intro = v.Introduced
				if p.intro == "0" {
					p.intro = ""
				}
				if isSemver && p.intro != "" {
					p.intro = prefix + p.intro
				}
			}
			if v.Fixed != "" {
				p.fixed = v.Fixed
				if isSemver && p.fixed != "" {
					p.fixed = prefix + p.fixed
				}
				ps = append(ps, p)
				p = pair{}
			}
		}
	}
	return ps
}

// AffectedPackages extracts information about affected packages from the given osv.Entry.
func AffectedPackages(e *osv.Entry) []*AffectedPackage {
	var affs []*AffectedPackage
	for _, a := range e.Affected {
		pairs := collectRangePairs(a)
		var vs []string
		for _, p := range pairs {
			var s string
			if p.intro == "" && p.fixed == "" {
				// If neither field is set, the vuln applies to all versions.
				// Leave it blank, the template will render it properly.
				s = ""
			} else if p.intro == "" {
				s = "before " + p.fixed
			} else if p.fixed == "" {
				s = p.intro + " and later"
			} else {
				s = "from " + p.intro + " before " + p.fixed
			}
			vs = append(vs, s)
		}
		for _, p := range a.EcosystemSpecific.Imports {
			affs = append(affs, &AffectedPackage{
				PackagePath: p.Path,
				Versions:    strings.Join(vs, ", "),
				Symbols:     exportedSymbols(p.Symbols),
				// TODO(hyangah): where to place GOOS/GOARCH info
			})
		}
	}
	return affs
}

func exportedSymbols(in []string) []string {
	var out []string
	for _, s := range in {
		exported := true
		for _, part := range strings.Split(s, ".") {
			if !token.IsExported(part) {
				exported = false // exported only all parts in the symbol name are exported.
			}
		}
		if exported {
			out = append(out, s)
		}
	}
	return out
}
