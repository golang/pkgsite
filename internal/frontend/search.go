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
	"path"
	"strconv"
	"strings"

	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/middleware"
	"golang.org/x/discovery/internal/postgres"
)

const defaultSearchLimit = 10

// SearchPage contains all of the data that the search template needs to
// populate.
type SearchPage struct {
	basePageData
	Results    []*SearchResult
	Pages      []int
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
	Licenses      []string
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

// pagesToLink returns the page numbers that will be displayed. Given a
// page, it returns a slice containing numPagesToLink integers in ascending
// order and optimizes for page to be in the middle of that range. The max
// value of an integer in the return slice will be less than numPages.
func pagesToLink(page, numPages, numPagesToLink int) []int {
	var pages []int
	start := page - (numPagesToLink / 2)
	if (numPages - start) < numPagesToLink {
		start = numPages - numPagesToLink + 1
	}
	if start < 1 {
		start = 1
	}

	for i := start; (i < start+numPagesToLink) && (i <= numPages); i++ {
		pages = append(pages, i)
	}
	return pages
}

const defaultNumPagesToLink = 7

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, db *postgres.DB, query string, limit, page int) (*SearchPage, error) {
	dbresults, err := db.Search(ctx, query, limit, offset(page, limit))
	if err != nil {
		return nil, fmt.Errorf("db.Search(%v, %d, %d): %v", query, limit, offset(page, limit), err)
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
		basePageData: basePageData{
			Title: query,
			Query: query,
		},
		Results:    results,
		NumPages:   numPages(limit, numResults),
		Pages:      pagesToLink(page, numPages(limit, numResults), defaultNumPagesToLink),
		NumResults: numResults,
		Page:       page,
		Offset:     offset(page, limit),
		Prev:       prev(page),
		Next:       next(page, limit, numResults),
	}, nil
}

// handleSearch applies database data to the search template. Handles endpoint
// /search?q=<query>. If <query> is an exact match for a package path, the user
// will be redirected to the details page.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.FormValue("q"))
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if strings.Contains(query, "/") {
		pkg, err := s.db.GetLatestPackage(ctx, path.Clean(query))
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/pkg/%s", pkg.Path), http.StatusFound)
			return
		} else if !derrors.IsNotFound(err) {
			log.Printf("error getting package for %s: %v", path.Clean(query), err)
		}
	}

	var (
		limit, pageNum int
		err            error
	)
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, err = strconv.Atoi(l)
		if err != nil {
			log.Printf("strconv.Atoi(%q) for limit: %v", l, err)
		}
	}
	if limit < 1 {
		limit = defaultSearchLimit
	}

	if p := r.URL.Query().Get("page"); p != "" {
		pageNum, err = strconv.Atoi(p)
		if err != nil {
			log.Printf("strconv.Atoi(%q) for page: %v", p, err)
		}
	}
	if pageNum <= 1 {
		pageNum = 1
	}

	page, err := fetchSearchPage(ctx, s.db, query, limit, pageNum)
	if err != nil {
		log.Printf("fetchSearchDetails(ctx, db, %q): %v", query, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}

	nonce, ok := middleware.GetNonce(ctx)
	if !ok {
		log.Printf("middleware.GetNonce(r.Context()): nonce was not set")
	}
	page.Nonce = nonce
	s.servePage(w, "search.tmpl", page)
}
