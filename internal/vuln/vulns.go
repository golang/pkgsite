// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vulns provides utilities to interact with vuln APIs.
package vuln

import (
	"context"
	"fmt"
	"go/token"
	"strings"

	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/stdlib"
	vers "golang.org/x/pkgsite/internal/version"
)

// A Vuln contains information to display about a vulnerability.
type Vuln struct {
	// The vulndb ID.
	ID string
	// A description of the vulnerability, or the problem in obtaining it.
	Details string
}

// VulnsForPackage obtains vulnerability information for the given package.
// If packagePath is empty, it returns all entries for the module at version.
// If there is an error, VulnsForPackage returns a single Vuln that describes the error.
func VulnsForPackage(ctx context.Context, modulePath, version, packagePath string, vc *Client) []Vuln {
	if vc == nil {
		return nil
	}

	// Handle special module paths.
	if modulePath == stdlib.ModulePath {
		// Stdlib pages requested at master will map to a pseudo version
		// that puts all vulns in range.
		// We can't really tell you're at master so version.IsPseudo
		// is the best we can do. The result is vulns won't be reported for a
		// pseudoversion that refers to a commit that is in a vulnerable range.
		switch {
		case vers.IsPseudo(version):
			return nil
		case strings.HasPrefix(packagePath, "cmd/"):
			modulePath = osv.GoCmdModulePath
		default:
			modulePath = osv.GoStdModulePath
		}
	}

	// Get all the vulns for this package/version.
	entries, err := vc.ByPackage(ctx, &PackageRequest{Module: modulePath, Package: packagePath, Version: version})
	if err != nil {
		return []Vuln{{Details: fmt.Sprintf("could not get vulnerability data: %v", err)}}
	}

	return toVulns(entries)
}

func toVulns(entries []*osv.Entry) []Vuln {
	if len(entries) == 0 {
		return nil
	}

	vulns := make([]Vuln, len(entries))
	for i, e := range entries {
		vulns[i] = Vuln{
			ID:      e.ID,
			Details: e.Summary,
		}
	}

	return vulns
}

// AffectedComponent holds information about a module/package affected by a certain vulnerability.
type AffectedComponent struct {
	Path           string
	Versions       string
	CustomVersions string
	// Lists of affected symbols (for packages).
	// If both of these lists are empty, all symbols in the package are affected.
	ExportedSymbols   []string
	UnexportedSymbols []string
}

// A pair is like an osv.Range, but each pair is a self-contained 2-tuple
// (introduced version, fixed version).
type pair struct {
	intro, fixed string
}

// collectRangePairs turns a slice of osv Ranges into a more manageable slice of
// formatted version pairs.
func collectRangePairs(modulePath string, rs []osv.Range) []pair {
	var (
		ps     []pair
		p      pair
		prefix string
	)
	if stdlib.Contains(modulePath) {
		prefix = "go"
	} else {
		prefix = "v"
	}
	for _, r := range rs {
		isSemver := r.Type == osv.RangeTypeSemver
		for _, v := range r.Events {
			if v.Introduced != "" {
				// We expected Introduced and Fixed to alternate, but if
				// p.intro != "", then they don't.
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

func versionString(modulePath string, rs []osv.Range) string {
	pairs := collectRangePairs(modulePath, rs)
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
	return strings.Join(vs, ", ")
}

// AffectedComponents extracts information about affected packages (and // modules, if there are any with no package information) from the given osv.Entry.
func AffectedComponents(e *osv.Entry) (pkgs, modsNoPkgs []*AffectedComponent) {
	for _, a := range e.Affected {
		vs := versionString(a.Module.Path, a.Ranges)
		cvs := versionString(a.Module.Path, a.EcosystemSpecific.CustomRanges)
		if len(a.EcosystemSpecific.Packages) == 0 {
			modsNoPkgs = append(modsNoPkgs, &AffectedComponent{
				Path:           a.Module.Path,
				Versions:       vs,
				CustomVersions: cvs,
			})
		}
		for _, p := range a.EcosystemSpecific.Packages {
			exported, unexported := affectedSymbols(p.Symbols)
			pkgs = append(pkgs, &AffectedComponent{
				Path:              p.Path,
				Versions:          vs,
				CustomVersions:    cvs,
				ExportedSymbols:   exported,
				UnexportedSymbols: unexported,
				// TODO(hyangah): where to place GOOS/GOARCH info
			})
		}
	}
	return pkgs, modsNoPkgs
}

func affectedSymbols(in []string) (e, u []string) {
	for _, s := range in {
		exported := true
		for _, part := range strings.Split(s, ".") {
			if !token.IsExported(part) {
				exported = false // exported only if all parts of the symbol name are exported.
			}
		}
		if exported {
			e = append(e, s)
		} else {
			u = append(u, s)
		}
	}
	return e, u
}
