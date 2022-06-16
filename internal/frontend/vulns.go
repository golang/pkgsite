// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/sync/errgroup"
	vulnc "golang.org/x/vuln/client"
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

// VulnsForPackage obtains vulnerability information for the given package.
// If packagePath is empty, it returns all entries for the module at version.
// The getVulnEntries function should retrieve all entries for the given module path.
// It is passed to facilitate testing.
// If there is an error, VulnsForPackage returns a single Vuln that describes the error.
func VulnsForPackage(modulePath, version, packagePath string, getVulnEntries vulnEntriesFunc) []Vuln {
	vs, err := vulnsForPackage(modulePath, version, packagePath, getVulnEntries)
	if err != nil {
		return []Vuln{{Details: fmt.Sprintf("could not get vulnerability data: %v", err)}}
	}
	return vs
}

func vulnsForPackage(modulePath, version, packagePath string, getVulnEntries vulnEntriesFunc) (_ []Vuln, err error) {
	defer derrors.Wrap(&err, "vulns(%q, %q, %q)", modulePath, version, packagePath)

	if getVulnEntries == nil {
		return nil, nil
	}
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

// VulnListPage holds the information for a page that lists all vuln entries.
type VulnListPage struct {
	basePage
	Entries []*osv.Entry
}

// VulnPage holds the information for a page that displays a single vuln entry.
type VulnPage struct {
	basePage
	Entry            *osv.Entry
	AffectedPackages []*AffectedPackage
}

type AffectedPackage struct {
	Path     string // Package.Name in the osv.Entry
	Versions string
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
			fixed = addVersionPrefix(fixed, packagePath)
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

func (s *Server) serveVuln(w http.ResponseWriter, r *http.Request, _ internal.DataSource) error {
	switch r.URL.Path {
	case "/", "/list":
		// Serve a list of all entries.
		vulnListPage, err := newVulnListPage(s.vulnClient)
		if err != nil {
			return &serverError{status: derrors.ToStatus(err)}
		}
		vulnListPage.basePage = s.newBasePage(r, "Go Vulnerabilities List")
		s.servePage(r.Context(), w, "vuln/list", vulnListPage)
	default: // the path should be "/<ID>", e.g. "/GO-2021-0001".
		id := r.URL.Path[1:]
		if !goVulnIDRegexp.MatchString(id) {
			return &serverError{
				status:       http.StatusBadRequest,
				responseText: "invalid Go vuln ID; should be GO-YYYY-NNNN",
			}
		}
		vulnPage, err := newVulnPage(s.vulnClient, id)
		if err != nil {
			return &serverError{status: derrors.ToStatus(err)}
		}
		vulnPage.basePage = s.newBasePage(r, id)
		s.servePage(r.Context(), w, "vuln/entry", vulnPage)
	}
	return nil
}

func newVulnPage(client vulnc.Client, id string) (*VulnPage, error) {
	entry, err := client.GetByID(id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, derrors.NotFound
	}
	affs := affectedPackages(entry)
	return &VulnPage{Entry: entry, AffectedPackages: affs}, nil
}

func newVulnListPage(client vulnc.Client) (*VulnListPage, error) {
	const concurrency = 4

	ids, err := client.ListIDs()
	if err != nil {
		return nil, err
	}
	// Sort from most to least recent.
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	entries := make([]*osv.Entry, len(ids))
	sem := make(chan struct{}, concurrency)
	var g errgroup.Group
	for i, id := range ids {
		i := i
		id := id
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			e, err := client.GetByID(id)
			if err != nil {
				return err
			}
			entries[i] = e
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return &VulnListPage{Entries: entries}, nil
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

func affectedPackages(e *osv.Entry) []*AffectedPackage {
	var affs []*AffectedPackage
	for _, a := range e.Affected {
		pairs := collectRangePairs(a)
		var vs []string
		for _, p := range pairs {
			var s string
			if p.intro == "" {
				s = p.fixed + " and earlier"
			} else if p.fixed == "" {
				s = p.intro + " and later"
			} else {
				s = p.intro + " - " + p.fixed
			}
			vs = append(vs, s)
		}
		affs = append(affs, &AffectedPackage{
			Path:     a.Package.Name,
			Versions: strings.Join(vs, ", "),
		})
	}
	return affs
}

func addVersionPrefix(semver, packagePath string) (res string) {
	if semver == "" {
		return ""
	}
	if packagePath != "" && stdlib.Contains(packagePath) {
		return "go" + semver
	}
	return "v" + semver
}
