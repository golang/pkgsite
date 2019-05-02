// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
)

const defaultSearchLimit = 10

// SearchPage contains all of the data that the search template needs to
// populate.
type SearchPage struct {
	basePageData
	Results    []*SearchResult
	NumPages   int
	NumResults int
	Offset     int
	Page       int
	Prev       int
	Next       int
}

// SearchResult contains data needed to display a single search result.
type SearchResult struct {
	Name          string
	PackagePath   string
	ModulePath    string
	Synopsis      string
	Version       string
	Licenses      []*internal.LicenseInfo
	CommitTime    string
	NumImportedBy uint64
}

func offset(page, limit int) int {
	if page < 2 {
		return 0
	}
	return (page - 1) * limit
}

func prev(page int) int {
	if page < 2 {
		return 0
	}
	return page - 1
}

func next(page, limit, numResults int) int {
	if page >= numPages(limit, numResults) {
		return 0
	}
	return page + 1
}

// numPages is the total number of pages needed to display all the results,
// given the specified limit.
func numPages(limit, numResults int) int {
	return int(math.Ceil(float64(numResults) / float64(limit)))
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, db *postgres.DB, query string, limit, page int) (*SearchPage, error) {
	terms := strings.Fields(query)
	dbresults, err := db.Search(ctx, terms, limit, offset(page, limit))
	if err != nil {
		return nil, fmt.Errorf("db.Search(%v): %v", terms, err)
	}

	var results []*SearchResult
	for _, r := range dbresults {
		results = append(results, &SearchResult{
			Name:          r.Package.Name,
			PackagePath:   r.Package.Path,
			ModulePath:    r.Package.VersionInfo.ModulePath,
			Synopsis:      r.Package.Package.Synopsis,
			Version:       r.Package.VersionInfo.Version,
			Licenses:      r.Package.Licenses,
			CommitTime:    elapsedTime(r.Package.VersionInfo.CommitTime),
			NumImportedBy: r.NumImportedBy,
		})
	}

	var numResults int
	if len(dbresults) > 0 {
		numResults = int(dbresults[0].NumResults)
	}

	return &SearchPage{
		basePageData: basePageData{
			Title: query,
			Query: query,
		},
		Results:    results,
		NumPages:   numPages(limit, numResults),
		NumResults: numResults,
		Page:       page,
		Offset:     offset(page, limit),
		Prev:       prev(page),
		Next:       next(page, limit, numResults),
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

	var (
		limit, pageNum int
		err            error
	)

	l := r.URL.Query().Get("limit")
	if l != "" {
		limit, err = strconv.Atoi(l)
		if err != nil {
			log.Printf("strconv.Atoi(%q) for limit: %v", l, err)
		}
	}
	if limit < 1 {
		limit = defaultSearchLimit
	}

	p := r.URL.Query().Get("page")
	if p != "" {
		pageNum, err = strconv.Atoi(p)
		if err != nil {
			log.Printf("strconv.Atoi(%q) for page: %v", l, err)
		}
	}
	if pageNum <= 1 {
		pageNum = 1
	}

	page, err := fetchSearchPage(ctx, c.db, query, limit, pageNum)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		log.Printf("fetchSearchDetails(ctx, db, %q): %v", query, err)
		return
	}

	c.renderPage(w, "search.tmpl", page)
}
