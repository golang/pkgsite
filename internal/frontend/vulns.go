// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
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

const (
	githubAdvisoryUrlPrefix = "https://github.com/advisories/"
	mitreAdvisoryUrlPrefix  = "https://cve.mitre.org/cgi-bin/cvename.cgi?name="
	nistAdvisoryUrlPrefix   = "https://nvd.nist.gov/vuln/detail/"
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

// VulnListPage holds the information for a page that lists all vuln entries.
type VulnListPage struct {
	basePage
	Entries []OSVEntry
}

// VulnPage holds the information for a page that displays a single vuln entry.
type VulnPage struct {
	basePage
	Entry            OSVEntry
	AffectedPackages []*AffectedPackage
	AliasLinks       []link
	AdvisoryLinks    []link
}

type AffectedPackage struct {
	PackagePath string
	Versions    string
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

func (s *Server) serveVuln(w http.ResponseWriter, r *http.Request, _ internal.DataSource) error {
	switch r.URL.Path {
	case "/":
		// Serve a list of most recent entries.
		vulnListPage, err := newVulnListPage(r.Context(), s.vulnClient)
		if err != nil {
			return &serverError{status: derrors.ToStatus(err)}
		}
		if len(vulnListPage.Entries) > 5 {
			vulnListPage.Entries = vulnListPage.Entries[:5]
		}
		vulnListPage.basePage = s.newBasePage(r, "Go Vulnerability Database")
		s.servePage(r.Context(), w, "vuln/main", vulnListPage)
	case "/list":
		// Serve a list of all entries.
		vulnListPage, err := newVulnListPage(r.Context(), s.vulnClient)
		if err != nil {
			return &serverError{status: derrors.ToStatus(err)}
		}
		vulnListPage.basePage = s.newBasePage(r, "Go Vulnerabilities List")
		s.servePage(r.Context(), w, "vuln/list", vulnListPage)
	default: // the path should be "/<ID>", e.g. "/GO-2021-0001".
		id := r.URL.Path[1:]
		if !goVulnIDRegexp.MatchString(id) {
			if r.URL.Query().Has("q") {
				return &serverError{status: derrors.ToStatus(derrors.NotFound)}
			}
			return &serverError{
				status:       http.StatusBadRequest,
				responseText: "invalid Go vuln ID; should be GO-YYYY-NNNN",
			}
		}
		vulnPage, err := newVulnPage(r.Context(), s.vulnClient, id)
		if err != nil {
			return &serverError{status: derrors.ToStatus(err)}
		}
		vulnPage.basePage = s.newBasePage(r, id)
		s.servePage(r.Context(), w, "vuln/entry", vulnPage)
	}
	return nil
}

func newVulnPage(ctx context.Context, client vulnc.Client, id string) (*VulnPage, error) {
	entry, err := client.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, derrors.NotFound
	}
	return &VulnPage{
		Entry:            OSVEntry{entry},
		AffectedPackages: affectedPackages(entry),
		AliasLinks:       aliasLinks(entry),
		AdvisoryLinks:    advisoryLinks(entry),
	}, nil
}

func newVulnListPage(ctx context.Context, client vulnc.Client) (*VulnListPage, error) {
	const concurrency = 4

	ids, err := client.ListIDs(ctx)
	if err != nil {
		return nil, err
	}
	// Sort from most to least recent.
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })

	entries := make([]OSVEntry, len(ids))
	sem := make(chan struct{}, concurrency)
	var g errgroup.Group
	for i, id := range ids {
		i := i
		id := id
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			e, err := client.GetByID(ctx, id)
			if err != nil {
				return err
			}
			entries[i] = OSVEntry{e}
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
		for _, p := range a.EcosystemSpecific.Imports {
			affs = append(affs, &AffectedPackage{
				PackagePath: p.Path,
				Versions:    strings.Join(vs, ", "),
			})
		}
	}
	return affs
}

// aliasLinks generates links to reference pages for vuln aliases.
func aliasLinks(e *osv.Entry) []link {
	var cveRef string
	for _, ref := range e.References {
		if strings.HasPrefix(ref.URL, nistAdvisoryUrlPrefix) || strings.HasPrefix(ref.URL, mitreAdvisoryUrlPrefix) {
			cveRef = ref.URL
			break
		}
	}
	var links []link
	for _, a := range e.Aliases {
		prefix, _, _ := strings.Cut(a, "-")
		switch prefix {
		case "CVE":
			links = append(links, link{Body: a, Href: cveRef})
		case "GHSA":
			links = append(links, link{Body: a, Href: githubAdvisoryUrlPrefix + a})
		default:
			links = append(links, link{Body: a})
		}
	}
	return links
}

func advisoryLinks(e *osv.Entry) []link {
	var links []link
	for _, r := range e.References {
		if r.Type == "ADVISORY" {
			links = append(links, link{Body: r.URL, Href: r.URL})
		}
	}
	return links
}
