// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/vulns"
	"golang.org/x/sync/errgroup"
	vulnc "golang.org/x/vuln/client"
	"golang.org/x/vuln/osv"
)

const (
	githubAdvisoryUrlPrefix = "https://github.com/advisories/"
	mitreAdvisoryUrlPrefix  = "https://cve.mitre.org/cgi-bin/cvename.cgi?name="
	nistAdvisoryUrlPrefix   = "https://nvd.nist.gov/vuln/detail/"
)

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
		return nil, derrors.VulnDBError
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
		return nil, derrors.VulnDBError
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
		return nil, derrors.VulnDBError
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
