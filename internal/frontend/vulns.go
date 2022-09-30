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
	"golang.org/x/pkgsite/internal/vulns"
	"golang.org/x/sync/errgroup"
	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

const (
	githubAdvisoryUrlPrefix = "https://github.com/advisories/"
	mitreAdvisoryUrlPrefix  = "https://cve.mitre.org/cgi-bin/cvename.cgi?name="
	nistAdvisoryUrlPrefix   = "https://nvd.nist.gov/vuln/detail/"

	// The vulndb stores vulns in cmd/go under the modulepath toolchain.
	vulnCmdGoModulePath = "toolchain"
	// The vulndb stores vulns under the modulepath stdlib for all other packages
	// in the standard library.
	vulnStdlibModulePath = "stdlib"
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
	if modulePath == stdlib.ModulePath && strings.HasPrefix(packagePath, "cmd/go") {
		modulePath = vulnCmdGoModulePath
	} else if modulePath == stdlib.ModulePath {
		modulePath = vulnStdlibModulePath
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

// VulnListPage holds the information for a page that lists vuln entries.
type VulnListPage struct {
	basePage
	Entries []OSVEntry
}

// VulnPage holds the information for a page that displays a single vuln entry.
type VulnPage struct {
	basePage
	Entry            OSVEntry
	AffectedPackages []*vulns.AffectedPackage
	AliasLinks       []link
	AdvisoryLinks    []link
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
	path := strings.TrimPrefix(r.URL.Path, "/vuln")
	switch path {
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
		vulnListPage.basePage = s.newBasePage(r, "Vulnerability Reports")
		s.servePage(r.Context(), w, "vuln/list", vulnListPage)
	default: // the path should be "/<ID>", e.g. "/GO-2021-0001".
		id := path[1:]
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
		AffectedPackages: vulns.AffectedPackages(entry),
		AliasLinks:       aliasLinks(entry),
		AdvisoryLinks:    advisoryLinks(entry),
	}, nil
}

func newVulnListPage(ctx context.Context, client vulnc.Client) (*VulnListPage, error) {
	entries, err := vulnList(ctx, client)
	if err != nil {
		return nil, err
	}
	// Sort from most to least recent.
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	return &VulnListPage{Entries: entries}, nil
}

func vulnList(ctx context.Context, client vulnc.Client) ([]OSVEntry, error) {
	const concurrency = 4

	ids, err := client.ListIDs(ctx)
	if err != nil {
		return nil, err
	}

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
	return entries, nil
}

// aliasLinks generates links to reference pages for vuln aliases.
func aliasLinks(e *osv.Entry) []link {
	var links []link
	for _, a := range e.Aliases {
		prefix, _, _ := strings.Cut(a, "-")
		switch prefix {
		case "CVE":
			links = append(links, link{Body: a, Href: mitreAdvisoryUrlPrefix + a})
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
