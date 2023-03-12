// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/safehtml/template"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/pkgsite/internal/vuln"
	"golang.org/x/text/message"
)

// serveSearch applies database data to the search template. Handles endpoint
// /search?q=<query>. If <query> is an exact match for a package path, the user
// will be redirected to the details page.
func (s *Server) serveSearch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	action, err := determineSearchAction(r, ds, s.vulnClient)
	if err != nil {
		return err
	}
	if action.redirectURL != "" {
		http.Redirect(w, r, action.redirectURL, http.StatusFound)
		return nil
	}
	action.page.setBasePage(s.newBasePage(r, action.title))
	if s.shouldServeJSON(r) {
		return s.serveJSONPage(w, r, action.page)
	}
	s.servePage(r.Context(), w, action.template, action.page)
	return nil
}

type searchAction struct {
	redirectURL string
	title       string
	template    string
	page        interface{ setBasePage(basePage) }
}

func determineSearchAction(r *http.Request, ds internal.DataSource, vulnClient *vuln.Client) (*searchAction, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return nil, &serverError{status: http.StatusMethodNotAllowed}
	}

	searchSupport := ds.SearchSupport()
	if searchSupport == internal.NoSearch {
		// The proxydatasource does not support the imported by page.
		return nil, datasourceNotSupportedErr()
	}

	ctx := r.Context()
	cq, filters := searchQueryAndFilters(r)
	if !utf8.ValidString(cq) {
		return nil, &serverError{status: http.StatusBadRequest}
	}
	if len(filters) > 1 {
		return nil, &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				messageTemplate: template.MakeTrustedTemplate(
					`<h3 class="Error-message">Search query contains more than one symbol.</h3>`),
			},
		}
	}
	if len(cq) > maxSearchQueryLength {
		return nil, &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				messageTemplate: template.MakeTrustedTemplate(
					`<h3 class="Error-message">Search query too long.</h3>`),
			},
		}
	}
	if cq == "" {
		return &searchAction{redirectURL: "/"}, nil
	}
	pageParams := newPaginationParams(r, defaultSearchLimit)
	if pageParams.offset() > maxSearchOffset {
		return nil, &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				messageTemplate: template.MakeTrustedTemplate(
					`<h3 class="Error-message">Search page number too large.</h3>`),
			},
		}
	}
	if pageParams.limit > maxSearchPageSize {
		return nil, &serverError{
			status: http.StatusBadRequest,
			epage: &errorPage{
				messageTemplate: template.MakeTrustedTemplate(
					`<h3 class="Error-message">Search page size too large.</h3>`),
			},
		}
	}
	var mode string
	if searchSupport == internal.BasicSearch {
		mode = searchModePackage
	} else {
		mode = searchMode(r)
	}
	if path := searchRequestRedirectPath(ctx, ds, cq, mode, vulnClient != nil); path != "" {
		return &searchAction{redirectURL: path}, nil
	}
	action, err := searchVulnAlias(ctx, mode, cq, vulnClient)
	if action != nil || err != nil {
		return action, err
	}
	action, err = searchVulnModule(ctx, mode, cq, vulnClient)
	if action != nil || err != nil {
		return action, err
	}
	var symbol string
	if len(filters) > 0 {
		symbol = filters[0]
	}
	page, err := fetchSearchPage(ctx, ds, cq, symbol, pageParams, mode == searchModeSymbol, vulnClient)
	if err != nil {
		// Instead of returning a 500, return a 408, since symbol searches may
		// timeout for very popular symbols.
		if mode == searchModeSymbol && strings.Contains(err.Error(), "i/o timeout") {
			return nil, &serverError{
				status: http.StatusRequestTimeout,
				epage: &errorPage{
					messageTemplate: template.MakeTrustedTemplate(
						`<h3 class="Error-message">Request timed out. Please try again!</h3>`),
				},
			}
		}
		return nil, fmt.Errorf("fetchSearchPage(ctx, db, %q): %v", cq, err)
	}
	page.SearchMode = mode
	return &searchAction{
		title:    fmt.Sprintf("%s - Search Results", cq),
		template: "search",
		page:     page,
	}, nil
}

const (
	// defaultSearchLimit is the default number of items that appears on the
	// search results page if limit is not specified.
	defaultSearchLimit = 25

	// maxSearchQueryLength represents the max number of characters that a search
	// query can be. For PostgreSQL 11, there is a max length of 2K bytes:
	// https://www.postgresql.org/docs/11/textsearch-limitations.html. No valid
	// searches on pkg.go.dev will need more than the maxSearchQueryLength.
	maxSearchQueryLength = 500

	// maxSearchOffset is the maximum allowed offset into the search results.
	// This prevents some very CPU-intensive queries from running.
	maxSearchOffset = 100

	// maxSearchPageSize is the maximum allowed limit for search results.
	maxSearchPageSize = 100

	// searchModePackage is the keyword prefix and query param for searching
	// by packages.
	searchModePackage = "package"

	// searchModeSymbol is the keyword prefix and query param for searching
	// by symbols.
	searchModeSymbol = "symbol"

	// searchModeVuln is the query param for searching by vuln id.
	searchModeVuln = "vuln"

	// symbolSearchFilter is a filter that can be used to indicate that the query
	// contains a symbol. For example, searching for "#unmarshal json" indicates
	// that unmarshal is a symbol.
	symbolSearchFilter = "#"
)

// SearchPage contains all of the data that the search template needs to
// populate.
type SearchPage struct {
	basePage

	// PackageTabQuery is the search query, stripped of any filters.
	// This is used if the user clicks on the package tab.
	PackageTabQuery string

	Pagination pagination
	Results    []*SearchResult
}

// SearchResult contains data needed to display a single search result.
type SearchResult struct {
	Name           string
	PackagePath    string
	ModulePath     string
	Version        string
	ChipText       string
	Synopsis       string
	DisplayVersion string
	Licenses       []string
	CommitTime     string
	NumImportedBy  string
	Symbols        *subResult
	SameModule     *subResult // package paths in the same module
	OtherMajor     *subResult // package paths in lower major versions
	SymbolName     string
	SymbolKind     string
	SymbolSynopsis string
	SymbolGOOS     string
	SymbolGOARCH   string
	SymbolLink     string
	Vulns          []vuln.Vuln
}

type subResult struct {
	Heading string
	Links   []link
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, ds internal.DataSource, cq, symbol string,
	pageParams paginationParams, searchSymbols bool, vulnClient *vuln.Client) (*SearchPage, error) {
	maxResultCount := maxSearchOffset + pageParams.limit

	// Pageless search: always start from the beginning.
	offset := 0
	dbresults, err := ds.Search(ctx, cq, postgres.SearchOptions{
		MaxResults:     pageParams.limit,
		Offset:         offset,
		MaxResultCount: maxResultCount,
		SearchSymbols:  searchSymbols,
		SymbolFilter:   symbol,
	})
	if err != nil {
		return nil, err
	}

	var results []*SearchResult
	for _, r := range dbresults {
		sr := newSearchResult(r, searchSymbols, message.NewPrinter(middleware.LanguageTag(ctx)))
		results = append(results, sr)
	}

	if vulnClient != nil {
		addVulns(ctx, results, vulnClient)
	}

	var numResults int
	if len(dbresults) > 0 {
		numResults = int(dbresults[0].NumResults)
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
	sp := &SearchPage{
		PackageTabQuery: cq,
		Results:         results,
		Pagination:      pgs,
	}
	return sp, nil
}

func newSearchResult(r *postgres.SearchResult, searchSymbols bool, pr *message.Printer) *SearchResult {
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
		Version:        r.Version,
		ChipText:       chipText,
		Synopsis:       r.Synopsis,
		DisplayVersion: displayVersion(r.ModulePath, r.Version, r.Version),
		Licenses:       r.Licenses,
		CommitTime:     elapsedTime(r.CommitTime),
		NumImportedBy:  pr.Sprint(r.NumImportedBy),
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
	return sr
}

// A regexp that matches Go vuln IDs.
var goVulnIDRegexp = regexp.MustCompile("^GO-[0-9]{4}-[0-9]{4}$")

// searchRequestRedirectPath returns the path that a search request should be
// redirected to, or the empty string if there is no such path.
//
// If the user types an existing package path into the search bar, we will
// redirect the user to the details page. Standard library packages that only
// contain one element (such as fmt, errors, etc.) will not redirect, to allow
// users to search by those terms.
//
// If the user types a name that is in the form of a Go vulnerability ID, we will
// redirect to the page for that ID (whether or not it exists).
func searchRequestRedirectPath(ctx context.Context, ds internal.DataSource, query, mode string, vulnSupport bool) string {
	urlSchemeIdx := strings.Index(query, "://")
	if urlSchemeIdx > -1 {
		query = query[urlSchemeIdx+3:]
	}
	if vulnSupport && goVulnIDRegexp.MatchString(query) {
		return fmt.Sprintf("/vuln/%s?q", query)
	}
	requestedPath := path.Clean(query)
	if !strings.Contains(requestedPath, "/") || mode == searchModeVuln {
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

func searchVulnModule(ctx context.Context, mode, cq string, client *vuln.Client) (_ *searchAction, err error) {
	if mode != searchModeVuln || client == nil {
		return nil, nil
	}
	allEntries, err := vulnList(ctx, client)
	if err != nil {
		return nil, err
	}
	prefix := cq + "/"
	var entries []OSVEntry
EntryLoop:
	for _, entry := range allEntries {
		for _, aff := range entry.Affected {
			for _, imp := range aff.EcosystemSpecific.Imports {
				if imp.Path == cq || strings.HasPrefix(imp.Path, prefix) {
					entries = append(entries, entry)
					continue EntryLoop
				}
			}
		}
	}
	// Sort from most to least recent.
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	return &searchAction{
		title:    fmt.Sprintf("%s - Vulnerability Reports", cq),
		template: "vuln/list",
		page:     &VulnListPage{Entries: entries},
	}, nil
}

func searchVulnAlias(ctx context.Context, mode, cq string, vc *vuln.Client) (_ *searchAction, err error) {
	defer derrors.Wrap(&err, "searchVulnAlias(%q, %q)", mode, cq)

	if mode != searchModeVuln || !isVulnAlias(cq) || vc == nil {
		return nil, nil
	}
	aliasEntries, err := vc.ByAlias(ctx, cq)
	if err != nil {
		return nil, err
	}
	switch len(aliasEntries) {
	case 0:
		return nil, &serverError{status: http.StatusNotFound}
	case 1:
		return &searchAction{redirectURL: "/vuln/" + aliasEntries[0].ID}, nil
	default:
		var entries []OSVEntry
		for _, e := range aliasEntries {
			entries = append(entries, OSVEntry{e})
		}
		return &searchAction{
			title:    fmt.Sprintf("%s - Vulnerability Reports", cq),
			template: "vuln/list",
			page:     &VulnListPage{Entries: entries},
		}, nil
	}
}

// Regexps that match aliases for Go vuln.
var (
	cveRegexp  = regexp.MustCompile("^CVE-[0-9]{4}-[0-9]+$")
	ghsaRegexp = regexp.MustCompile("^GHSA-.{4}-.{4}-.{4}$")
)

func isVulnAlias(s string) bool {
	return cveRegexp.MatchString(s) || ghsaRegexp.MatchString(s)
}

// searchMode reports whether the search performed should be in package or
// symbol search mode.
func searchMode(r *http.Request) string {
	q, filters := searchQueryAndFilters(r)
	if len(filters) > 0 {
		return searchModeSymbol
	}
	switch rawSearchMode(r) {
	case searchModePackage:
		return searchModePackage
	case searchModeSymbol:
		return searchModeSymbol
	case searchModeVuln:
		return searchModeVuln
	default:
		if isVulnAlias(q) {
			return searchModeVuln
		}
		if shouldDefaultToSymbolSearch(q) {
			return searchModeSymbol
		}
		return searchModePackage
	}
}

// searchQueryAndFilters returns the search query, trimmed of any filters, and
// the array of words that had a filter prefix.
func searchQueryAndFilters(r *http.Request) (string, []string) {
	words := strings.Fields(rawSearchQuery(r))
	var filters []string
	for i := range words {
		if strings.HasPrefix(words[i], symbolSearchFilter) {
			words[i] = strings.TrimLeft(words[i], symbolSearchFilter)
			filters = append(filters, words[i])
		}
	}
	return strings.Join(words, " "), filters
}

// rawSearchQuery returns the exact search query by the user.
func rawSearchQuery(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("q"))
}

// rawSearchMode returns the exact search mode from the URL request.
func rawSearchMode(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("m"))
}

// shouldDefaultToSymbolSearch reports whether the search mode should
// default to symbol based on the input.
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

// symbolSynopsis returns the string to be displayed in the code snippet
// section for a symbol search result.
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

func modulePaths(heading string, modulePathToMajor map[string]int) *subResult {
	if len(modulePathToMajor) == 0 {
		return nil
	}

	type mm struct {
		Path  string
		Major int
	}

	var mms []mm
	for m, v := range modulePathToMajor {
		mms = append(mms, mm{m, v})
	}
	sort.Slice(mms, func(i, j int) bool { return mms[i].Major > mms[j].Major })
	links := make([]link, len(mms))
	for i, m := range mms {
		links[i] = link{Href: m.Path, Body: fmt.Sprintf("v%d", m.Major)}
	}
	return &subResult{
		Heading: heading,
		Links:   links,
	}
}

// isCapitalized reports whether the first letter of s is capitalized.
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

// addVulns adds vulnerability information to search results by consulting the
// vulnerability database.
func addVulns(ctx context.Context, rs []*SearchResult, vc *vuln.Client) {
	// Get all vulns concurrently.
	var wg sync.WaitGroup
	// TODO(golang/go#48223): throttle concurrency?
	for _, r := range rs {
		r := r
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Vulns = vuln.VulnsForPackage(ctx, r.ModulePath, r.Version, r.PackagePath, vc)
		}()
	}
	wg.Wait()

}
