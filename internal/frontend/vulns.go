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
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/vuln"
	"golang.org/x/sync/errgroup"
)

const (
	githubAdvisoryUrlPrefix = "https://github.com/advisories/"
	mitreAdvisoryUrlPrefix  = "https://www.cve.org/CVERecord?id="
	nistAdvisoryUrlPrefix   = "https://nvd.nist.gov/vuln/detail/"
)

// VulnListPage holds the information for a page that lists vuln entries.
type VulnListPage struct {
	basePage
	Entries []*osv.Entry
}

// VulnPage holds the information for a page that displays a single vuln entry.
type VulnPage struct {
	basePage
	Entry            *osv.Entry
	AffectedPackages []*vuln.AffectedPackage
	AliasLinks       []link
	AdvisoryLinks    []link
}

func (s *Server) serveVuln(w http.ResponseWriter, r *http.Request, _ internal.DataSource) error {
	if s.vulnClient == nil {
		return datasourceNotSupportedErr()
	}
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
		if !vuln.IsGoID(id) {
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

func newVulnPage(ctx context.Context, client *vuln.Client, id string) (*VulnPage, error) {
	entry, err := client.ByID(ctx, id)
	if err != nil {
		return nil, derrors.VulnDBError
	}
	if entry == nil {
		return nil, derrors.NotFound
	}
	return &VulnPage{
		Entry:            entry,
		AffectedPackages: vuln.AffectedPackages(entry),
		AliasLinks:       aliasLinks(entry),
		AdvisoryLinks:    advisoryLinks(entry),
	}, nil
}

func newVulnListPage(ctx context.Context, client *vuln.Client) (*VulnListPage, error) {
	entries, err := vulnList(ctx, client)
	if err != nil {
		return nil, err
	}
	// Sort from most to least recent.
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	return &VulnListPage{Entries: entries}, nil
}

func vulnList(ctx context.Context, client *vuln.Client) ([]*osv.Entry, error) {
	const concurrency = 4

	ids, err := client.IDs(ctx)
	if err != nil {
		return nil, derrors.VulnDBError
	}

	entries := make([]*osv.Entry, len(ids))
	sem := make(chan struct{}, concurrency)
	var g errgroup.Group
	for i, id := range ids {
		i := i
		id := id
		sem <- struct{}{}
		g.Go(func() error {
			defer func() { <-sem }()
			e, err := client.ByID(ctx, id)
			if err != nil {
				return err
			}
			entries[i] = e
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
