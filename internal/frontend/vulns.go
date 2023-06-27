// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/osv"
	"golang.org/x/pkgsite/internal/vuln"
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

// VulnEntryPage holds the information for a page that displays a single
// vuln entry.
type VulnEntryPage struct {
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

	vp, err := newVulnPage(r.Context(), r.URL, s.vulnClient)
	if err != nil {
		var serr *serverError
		if !errors.As(err, &serr) {
			serr = &serverError{status: derrors.ToStatus(err), err: err}
		}
		return serr
	}

	page := vp.page
	page.setBasePage(s.newBasePage(r, vp.title))
	s.servePage(r.Context(), w, vp.template, page)
	return nil
}

type vulnPage struct {
	page     interface{ setBasePage(basePage) }
	template string
	title    string
}

func newVulnPage(ctx context.Context, url *url.URL, vc *vuln.Client) (*vulnPage, error) {
	path := strings.TrimPrefix(url.Path, "/vuln")
	switch path {
	case "/":
		// Serve a list of the 5 most recent entries.
		page, err := newVulnListPage(ctx, vc, 5)
		if err != nil {
			return nil, err
		}
		return &vulnPage{
			page:     page,
			template: "vuln/main",
			title:    "Go Vulnerability Database"}, nil
	case "/list":
		// Serve a list of all entries.
		page, err := newVulnListPage(ctx, vc, -1)
		if err != nil {
			return nil, err
		}
		return &vulnPage{
			page:     page,
			template: "vuln/list",
			title:    "Vulnerability Reports"}, nil
	default: // the path should be "/<ID>", e.g. "/GO-2021-0001".
		id, ok := vuln.CanonicalGoID(strings.TrimPrefix(path, "/"))
		if !ok {
			if url.Query().Has("q") {
				return nil, derrors.NotFound
			}
			return nil, &serverError{
				status:       http.StatusBadRequest,
				responseText: "invalid Go vuln ID; should be GO-YYYY-NNNN",
			}
		}
		page, err := newVulnEntryPage(ctx, vc, id)
		if err != nil {
			return nil, err
		}
		return &vulnPage{
			page:     page,
			template: "vuln/entry",
			title:    id}, nil
	}
}

func newVulnEntryPage(ctx context.Context, client *vuln.Client, id string) (*VulnEntryPage, error) {
	entry, err := client.ByID(ctx, id)
	if err != nil {
		return nil, derrors.VulnDBError
	}
	if entry == nil {
		return nil, derrors.NotFound
	}
	return &VulnEntryPage{
		Entry:            entry,
		AffectedPackages: vuln.AffectedPackages(entry),
		AliasLinks:       aliasLinks(entry),
		AdvisoryLinks:    advisoryLinks(entry),
	}, nil
}

func newVulnListPage(ctx context.Context, client *vuln.Client, n int) (*VulnListPage, error) {
	entries, err := client.Entries(ctx, n)
	if err != nil {
		return nil, derrors.VulnDBError
	}
	return &VulnListPage{Entries: entries}, nil
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
