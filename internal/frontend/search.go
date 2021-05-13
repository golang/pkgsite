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
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
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
	NumImportedBy  int
	Approximate    bool
	SameModule     *subResult // package paths in the same module
	LowerMajor     *subResult // package paths in lower major versions
}

type subResult struct {
	Heading string
	Links   []link
	Suffix  string
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, db *postgres.DB, query string, pageParams paginationParams) (*SearchPage, error) {
	maxResultCount := maxSearchOffset + pageParams.limit
	dbresults, err := db.Search(ctx, query, pageParams.limit, pageParams.offset(), maxResultCount, false)
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
			NumImportedBy:  int(r.NumImportedBy),
			SameModule:     packagePaths("Other packages in module "+r.ModulePath+":", r.SameModule, 5),
			LowerMajor:     modulePaths("Lower module versions:", r.LowerMajor),
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

	numPageResults := 0
	for _, r := range dbresults {
		// Grouping will put some results inside others. Each result counts one
		// for itself plus one for each sub-result in the SameModule list,
		// because each of those is removed from the top-level slice. Results in
		// the LowerMajor list are not removed from the top-level slice,
		// so we don't add them up.
		numPageResults += 1 + len(r.SameModule)
	}

	pgs := newPagination(pageParams, numPageResults, numResults)
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

func packagePaths(heading string, rs []*internal.SearchResult, max int) *subResult {
	if len(rs) == 0 {
		return nil
	}
	var links []link
	for i, r := range rs {
		if i >= max {
			break
		}
		links = append(links, link{Href: r.PackagePath, Body: internal.Suffix(r.PackagePath, r.ModulePath)})
	}
	suffix := ""
	if len(rs) > len(links) {
		suffix = fmt.Sprintf("(and %d more)", len(rs)-len(links))
	}
	return &subResult{
		Heading: heading,
		Links:   links,
		Suffix:  suffix,
	}
}

func modulePaths(heading string, rs []*internal.SearchResult) *subResult {
	if len(rs) == 0 {
		return nil
	}
	mpm := map[string]bool{}
	for _, r := range rs {
		mpm[r.ModulePath] = true
	}
	var mps []string
	for m := range mpm {
		mps = append(mps, m)
	}
	sort.Slice(mps, func(i, j int) bool {
		_, v1 := internal.SeriesPathAndMajorVersion(mps[i])
		_, v2 := internal.SeriesPathAndMajorVersion(mps[j])
		return v1 > v2
	})
	links := make([]link, len(mps))
	for i, m := range mps {
		links[i] = link{Href: m, Body: m}
	}
	return &subResult{
		Heading: heading,
		Links:   links,
	}
}

// Search constraints.
const (
	// maxSearchQueryLength represents the max number of characters that a search
	// query can be. For PostgreSQL 11, there is a max length of 2K bytes:
	// https://www.postgresql.org/docs/11/textsearch-limitations.html. No valid
	// searches on pkg.go.dev will need more than the maxSearchQueryLength.
	maxSearchQueryLength = 500

	// maxSearchOffset is the maximum allowed offset into the search results.
	// This prevents some very CPU-intensive queries from running.
	maxSearchOffset = 90

	// maxSearchPageSize is the maximum allowed limit for search results.
	maxSearchPageSize = 100
)

// serveSearch applies database data to the search template. Handles endpoint
// /search?q=<query>. If <query> is an exact match for a package path, the user
// will be redirected to the details page.
func (s *Server) serveSearch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return &serverError{status: http.StatusMethodNotAllowed}
	}
	db, ok := ds.(*postgres.DB)
	if !ok {
		// The proxydatasource does not support the imported by page.
		return proxydatasourceNotSupportedErr()
	}

	ctx := r.Context()
	query := searchQuery(r)
	if !utf8.ValidString(query) {
		return &serverError{status: http.StatusBadRequest}
	}
	if len(query) > maxSearchQueryLength {
		return &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				messageTemplate: template.MakeTrustedTemplate(
					`<h3 class="Error-message">Search query too long.</h3>`),
			},
		}
	}
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return nil
	}
	pageParams := newPaginationParams(r, defaultSearchLimit)
	if pageParams.offset() > maxSearchOffset {
		return &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				messageTemplate: template.MakeTrustedTemplate(
					`<h3 class="Error-message">Search page number too large.</h3>`),
			},
		}
	}
	if pageParams.limit > maxSearchPageSize {
		return &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				messageTemplate: template.MakeTrustedTemplate(
					`<h3 class="Error-message">Search page size too large.</h3>`),
			},
		}
	}

	if path := searchRequestRedirectPath(ctx, ds, query); path != "" {
		http.Redirect(w, r, path, http.StatusFound)
		return nil
	}
	page, err := fetchSearchPage(ctx, db, query, pageParams)
	if err != nil {
		return fmt.Errorf("fetchSearchPage(ctx, db, %q): %v", query, err)
	}
	page.basePage = s.newBasePage(r, fmt.Sprintf("%s - Search Results", query))
	s.servePage(ctx, w, "search.tmpl", page)
	return nil
}

// searchRequestRedirectPath returns the path that a search request should be
// redirected to, or the empty string if there is no such path. If the user
// types an existing package path into the search bar, we will redirect the
// user to the details page. Standard library packages that only contain one
// element (such as fmt, errors, etc.) will not redirect, to allow users to
// search by those terms.
func searchRequestRedirectPath(ctx context.Context, ds internal.DataSource, query string) string {
	urlSchemeIdx := strings.Index(query, "://")
	if urlSchemeIdx > -1 {
		query = query[urlSchemeIdx+3:]
	}
	requestedPath := path.Clean(query)
	if !strings.Contains(requestedPath, "/") {
		return ""
	}
	_, err := ds.GetUnitMeta(ctx, requestedPath, internal.UnknownModulePath, internal.LatestVersion)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "searchRequestRedirectPath(%q): %v", requestedPath, err)
		}
		return ""
	}
	return fmt.Sprintf("/%s", requestedPath)
}

// searchQuery extracts a search query from the request.
func searchQuery(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("q"))
}

// elapsedTime takes a date and returns returns human-readable,
// relative timestamps based on the following rules:
// (1) 'X hours ago' when X < 6
// (2) 'today' between 6 hours and 1 day ago
// (3) 'Y days ago' when Y < 6
// (4) A date formatted like "Jan 2, 2006" for anything further back
func elapsedTime(date time.Time) string {
	elapsedHours := int(time.Since(date).Hours())
	if elapsedHours == 1 {
		return "1 hour ago"
	} else if elapsedHours < 6 {
		return fmt.Sprintf("%d hours ago", elapsedHours)
	}

	elapsedDays := elapsedHours / 24
	if elapsedDays < 1 {
		return "today"
	} else if elapsedDays == 1 {
		return "1 day ago"
	} else if elapsedDays < 6 {
		return fmt.Sprintf("%d days ago", elapsedDays)
	}

	return absoluteTime(date)
}
