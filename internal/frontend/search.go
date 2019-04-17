// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

// SearchPage contains all of the data that the search template needs to
// populate.
type SearchPage struct {
	Query   string
	Results []*SearchResult
}

// SearchResult contains data needed to display a single search result.
type SearchResult struct {
	Name         string
	PackagePath  string
	ModulePath   string
	Synopsis     string
	Version      string
	Licenses     []*internal.LicenseInfo
	CommitTime   string
	NumImporters int64
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, db *postgres.DB, query string) (*SearchPage, error) {
	terms := strings.Fields(query)
	dbresults, err := db.Search(ctx, terms)
	if err != nil {
		return nil, fmt.Errorf("db.Search(%v): %v", terms, err)
	}

	var results []*SearchResult
	for _, r := range dbresults {
		results = append(results, &SearchResult{
			Name:         r.Package.Name,
			PackagePath:  r.Package.Path,
			ModulePath:   r.Package.Version.Module.Path,
			Synopsis:     r.Package.Synopsis,
			Version:      r.Package.Version.Version,
			Licenses:     r.Package.Licenses,
			CommitTime:   elapsedTime(r.Package.Version.CommitTime),
			NumImporters: r.NumImporters,
		})
	}

	return &SearchPage{
		Query:   query,
		Results: results,
	}, nil
}

// HandleSearch applies database data to the search template. Handles endpoint
// /search?q=<query>.
func (c *Controller) HandleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.FormValue("q"))
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	page, err := fetchSearchPage(ctx, c.db, query)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		log.Printf("fetchSearchDetails(ctx, db, %q): %v", query, err)
		return
	}

	c.renderPage(w, "search.tmpl", page)
}
