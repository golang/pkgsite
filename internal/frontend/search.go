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
	"unicode"
	"unicode/utf8"

	"github.com/google/safehtml/template"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
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
	ChipText       string
	Synopsis       string
	DisplayVersion string
	Licenses       []string
	CommitTime     string
	NumImportedBy  int
	Approximate    bool
	Symbols        *subResult
	SameModule     *subResult // package paths in the same module
	OtherMajor     *subResult // package paths in lower major versions
	SymbolName     string
	SymbolKind     string
	SymbolSynopsis string
	SymbolGOOS     string
	SymbolGOARCH   string
	SymbolLink     string
}

type subResult struct {
	Heading string
	Links   []link
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, db *postgres.DB, query string, pageParams paginationParams, searchSymbols bool) (*SearchPage, error) {
	maxResultCount := maxSearchOffset + pageParams.limit

	offset := pageParams.offset()
	if experiment.IsActive(ctx, internal.ExperimentSearchGrouping) {
		// When using search grouping, do pageless search: always start from the beginning.
		offset = 0
	}
	dbresults, err := db.Search(ctx, query, postgres.SearchOptions{
		MaxResults:     pageParams.limit,
		Offset:         offset,
		MaxResultCount: maxResultCount,
		SearchSymbols:  searchSymbols,
	})
	if err != nil {
		return nil, err
	}

	var results []*SearchResult
	for _, r := range dbresults {
		// For commands, change the name from "main" to the last component of the import path.
		chipText := ""
		name := r.Name
		if name == "main" {
			chipText = "command"
			name = effectiveName(r.PackagePath, r.Name)
		}
		moduleDesc := "Other packages in module " + r.ModulePath
		if r.ModulePath == stdlib.ModulePath {
			moduleDesc = "Related packages in the standard library"
			chipText = "standard library"
		}
		sr := &SearchResult{
			Name:           name,
			PackagePath:    r.PackagePath,
			ModulePath:     r.ModulePath,
			ChipText:       chipText,
			Synopsis:       r.Synopsis,
			DisplayVersion: displayVersion(r.ModulePath, r.Version, r.Version),
			Licenses:       r.Licenses,
			CommitTime:     elapsedTime(r.CommitTime),
			NumImportedBy:  int(r.NumImportedBy),
			SameModule:     packagePaths(moduleDesc+":", r.SameModule),
			// Say "other" instead of "lower" because at some point we may
			// prefer to show a tagged, lower major version over an untagged
			// higher major version.
			OtherMajor: modulePaths("Other major versions:", r.OtherMajor),
		}
		if searchSymbols {
			sr.SymbolName = r.SymbolName
			sr.SymbolKind = strings.ToLower(string(r.SymbolKind))
			sr.SymbolSynopsis = symbolSynopsis(r)
			sr.SymbolGOOS = r.SymbolGOOS
			sr.SymbolGOARCH = r.SymbolGOARCH
			// If the GOOS is "all" or "linux", it doesn't need to be
			// specified as a query param. "linux" is the default GOOS when a
			// package has multiple build contexts, since it is first item
			// listed in internal.BuildContexts.
			if r.SymbolGOOS == internal.All || r.SymbolGOOS == "linux" {
				sr.SymbolLink = fmt.Sprintf("/%s#%s", r.PackagePath, r.SymbolName)
			} else {
				sr.SymbolLink = fmt.Sprintf("/%s?GOOS=%s#%s", r.PackagePath, r.SymbolGOOS, r.SymbolName)
			}
		}
		results = append(results, sr)
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
	sp := &SearchPage{
		Results:    results,
		Pagination: pgs,
	}
	return sp, nil
}

func symbolSynopsis(r *postgres.SearchResult) string {
	switch r.SymbolKind {
	case internal.SymbolKindField:
		return fmt.Sprintf(`
type %s struct {
	%s
}
`, strings.Split(r.SymbolName, ".")[0], r.SymbolSynopsis)
	case internal.SymbolKindMethod:
		if !strings.HasPrefix(r.SymbolSynopsis, "func (") {
			return fmt.Sprintf(`
type %s interface {
	%s
}
`, strings.Split(r.SymbolName, ".")[0], r.SymbolSynopsis)
		}
	}
	return r.SymbolSynopsis
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

func packagePaths(heading string, rs []*postgres.SearchResult) *subResult {
	if len(rs) == 0 {
		return nil
	}
	var links []link
	for _, r := range rs {
		links = append(links, link{Href: r.PackagePath, Body: internal.Suffix(r.PackagePath, r.ModulePath)})
	}
	return &subResult{
		Heading: heading,
		Links:   links,
	}
}

func modulePaths(heading string, mpaths map[string]bool) *subResult {
	if len(mpaths) == 0 {
		return nil
	}
	var mps []string
	for m := range mpaths {
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

	// searchModePackage is the keyword prefix and query param for searching
	// by packages.
	searchModePackage = "package"

	// searchModeSymbol is the keyword prefix and query param for searching
	// by symbols.
	searchModeSymbol = "symbol"
)

var (
	// searchModeSymbolKeyboardShortcuts is the set of allow keyboard shortcuts
	// for symbol search.
	searchModeSymbolKeyboardShortcuts = map[string]bool{
		"s":              true,
		searchModeSymbol: true,
	}

	// searchModePackageKeyboardShortcuts is the set of allow keyboard shortcuts
	// for package search.
	searchModePackageKeyboardShortcuts = map[string]bool{
		"p":               true,
		searchModePackage: true,
	}
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
	query, searchSymbols := searchQuery(r)
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

	page, err := fetchSearchPage(ctx, db, query, pageParams, searchSymbols)
	if err != nil {
		return fmt.Errorf("fetchSearchPage(ctx, db, %q): %v", query, err)
	}
	page.basePage = s.newBasePage(r, fmt.Sprintf("%s - Search Results", query))
	if searchSymbols {
		page.SearchMode = searchModeSymbol
	}
	if s.shouldServeJSON(r) {
		return s.serveJSONPage(w, r, page)
	}
	tmpl := "legacy_search"
	if experiment.IsActive(ctx, internal.ExperimentSearchGrouping) {
		tmpl = "search"
	}
	s.servePage(ctx, w, tmpl, page)
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
	_, err := ds.GetUnitMeta(ctx, requestedPath, internal.UnknownModulePath, version.Latest)
	if err != nil {
		if !errors.Is(err, derrors.NotFound) {
			log.Errorf(ctx, "searchRequestRedirectPath(%q): %v", requestedPath, err)
		}
		return ""
	}
	return fmt.Sprintf("/%s", requestedPath)
}

// searchQuery extracts a search query from the request. It also reports
// whether the search performed should be in symbolSearch mode.
// See TestSearchQuery for examples.
func searchQuery(r *http.Request) (q string, searchSymbols bool) {
	q = strings.TrimSpace(r.FormValue("q"))
	if !experiment.IsActive(r.Context(), internal.ExperimentSymbolSearch) {
		return q, false
	}
	if strings.HasPrefix(q, "#") {
		return strings.TrimPrefix(q, "#"), true
	}
	if strings.Contains(q, ":") {
		parts := strings.SplitN(q, ":", 2)
		if searchModeSymbolKeyboardShortcuts[parts[0]] {
			return parts[1], true
		}
		if searchModePackageKeyboardShortcuts[parts[0]] {
			return parts[1], false
		}
		return q, false
	}
	mode := strings.TrimSpace(r.FormValue("m"))
	if mode == searchModePackage {
		return q, false
	}
	if mode == searchModeSymbol {
		return q, true
	}
	if shouldDefaultToSymbolSearch(q) {
		return q, true
	}
	return q, mode == searchModeSymbol
}

// shouldDefaultToSymbolSearch reports whether the symbol search mode should
// default to symbol search mode based on the input.
func shouldDefaultToSymbolSearch(q string) bool {
	if len(strings.Fields(q)) != 1 {
		return false
	}
	if internal.IsGoPkgInPathElement(q) {
		return false
	}
	parts := strings.Split(q, ".")
	if len(parts) > 1 {
		if len(parts) == 2 && semver.IsValid(parts[1]) {
			// The q has the format <text>.<semver> which is likely a
			// gopkg.in host, such as yaml.v2. Default to package search.
			return false
		}
		return !internal.TopLevelDomains[parts[len(parts)-1]]
	}
	// If a user searches for "Unmarshal", assume that they are searching for
	// the symbol name "Unmarshal", not the package unmarshal.
	return isCapitalized(q)
}

func isCapitalized(s string) bool {
	if len(s) == 0 {
		return false
	}
	return unicode.IsUpper(rune(s[0]))
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
