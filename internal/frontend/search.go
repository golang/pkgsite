// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/xerrors"
)

const defaultSearchLimit = 10

// SearchPage contains all of the data that the search template needs to
// populate.
type SearchPage struct {
	basePage
	Pagination pagination
	Results    []*SearchResult
}

// SearchResult contains data needed to display a single search result.
type SearchResult struct {
	Name          string
	PackagePath   string
	ModulePath    string
	Synopsis      string
	Version       string
	Licenses      []string
	CommitTime    string
	NumImportedBy uint64
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, ds DataSource, query string, pageParams paginationParams) (*SearchPage, error) {
	dbresults, err := ds.LegacySearch(ctx, query, pageParams.limit, pageParams.offset())
	if err != nil {
		return nil, err
	}

	var results []*SearchResult
	for _, r := range dbresults {
		results = append(results, &SearchResult{
			Name:          r.Name,
			PackagePath:   r.PackagePath,
			ModulePath:    r.ModulePath,
			Synopsis:      r.Synopsis,
			Version:       r.Version,
			Licenses:      r.Licenses,
			CommitTime:    elapsedTime(r.CommitTime),
			NumImportedBy: r.NumImportedBy,
		})
	}

	var numResults int
	if len(dbresults) > 0 {
		numResults = int(dbresults[0].NumResults)
	}

	return &SearchPage{
		Results:    results,
		Pagination: newPagination(pageParams, len(results), numResults),
	}, nil
}

// handleSearch applies database data to the search template. Handles endpoint
// /search?q=<query>. If <query> is an exact match for a package path, the user
// will be redirected to the details page.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := searchQuery(r)
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if strings.Contains(query, "/") {
		pkg, err := s.ds.GetPackage(ctx, path.Clean(query), internal.LatestVersion)
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/pkg/%s", pkg.Path), http.StatusFound)
			return
		} else if !xerrors.Is(err, derrors.NotFound) {
			log.Errorf("error getting package for %s: %v", path.Clean(query), err)
		}
	}

	page, err := fetchSearchPage(ctx, s.ds, query, newPaginationParams(r, defaultSearchLimit))
	if err != nil {
		log.Errorf("fetchSearchDetails(ctx, db, %q): %v", query, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}
	page.basePage = newBasePage(r, query)
	s.servePage(w, "search.tmpl", page)
}

// searchQuery extracts a search query from the request.
func searchQuery(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("q"))
}
