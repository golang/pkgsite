// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path"
	"strings"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
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
	Name           string
	PackagePath    string
	ModulePath     string
	Synopsis       string
	DisplayVersion string
	Licenses       []string
	CommitTime     string
	NumImportedBy  uint64
	Approximate    bool
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, ds internal.DataSource, query string, pageParams paginationParams) (*SearchPage, error) {

	dbresults, err := ds.Search(ctx, query, pageParams.limit, pageParams.offset())
	if err != nil {
		return nil, err
	}

	var results []*SearchResult
	for _, r := range dbresults {
		results = append(results, &SearchResult{
			Name:           r.Name,
			PackagePath:    r.PackagePath,
			ModulePath:     r.ModulePath,
			Synopsis:       r.Synopsis,
			DisplayVersion: displayVersion(r.Version, r.ModulePath),
			Licenses:       r.Licenses,
			CommitTime:     elapsedTime(r.CommitTime),
			NumImportedBy:  r.NumImportedBy,
		})
	}

	var (
		numResults  int
		approximate bool
	)
	if len(dbresults) > 0 {
		numResults = int(dbresults[0].NumResults)
		if dbresults[0].Approximate {
			// 128 buckets corresponds to a standard error of 10%.
			// http://algo.inria.fr/flajolet/Publications/FlFuGaMe07.pdf
			numResults = approximateNumber(numResults, 0.1)
			approximate = true
		}
	}

	pgs := newPagination(pageParams, len(results), numResults)
	pgs.Approximate = approximate
	return &SearchPage{
		Results:    results,
		Pagination: pgs,
	}, nil
}

// approximateNumber returns an approximation of the estimate, calibrated by
// the statistical estimate of standard error.
// i.e., a number that isn't misleading when we say '1-10 of approximately N
// results', but that is still close to our estimate.
func approximateNumber(estimate int, sigma float64) int {
	expectedErr := sigma * float64(estimate)
	// Compute the unit by rounding the error the logarithmically closest power
	// of 10, so that 300->100, but 400->1000.
	unit := math.Pow(10, math.Round(math.Log10(expectedErr)))
	// Now round the estimate to the nearest unit.
	return int(unit * math.Round(float64(estimate)/unit))
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
		pkg, err := s.ds.GetPackage(ctx, path.Clean(query), internal.UnknownModulePath, internal.LatestVersion)
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/%s", pkg.Path), http.StatusFound)
			return
		} else if !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "error getting package for %s: %v", path.Clean(query), err)
		}
	}

	page, err := fetchSearchPage(ctx, s.ds, query, newPaginationParams(r, defaultSearchLimit))
	if err != nil {
		log.Errorf(ctx, "fetchSearchDetails(ctx, db, %q): %v", query, err)
		s.serveErrorPage(w, r, http.StatusInternalServerError, nil)
		return
	}
	page.basePage = newBasePage(r, query)
	s.servePage(ctx, w, "search.tmpl", page)
}

// searchQuery extracts a search query from the request.
func searchQuery(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("q"))
}
