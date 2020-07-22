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

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
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
	NumImportedBy  uint64
	Approximate    bool
}

// fetchSearchPage fetches data matching the search query from the database and
// returns a SearchPage.
func fetchSearchPage(ctx context.Context, db *postgres.DB, query string, pageParams paginationParams) (*SearchPage, error) {
	dbresults, err := db.Search(ctx, query, pageParams.limit, pageParams.offset())
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

// serveSearch applies database data to the search template. Handles endpoint
// /search?q=<query>. If <query> is an exact match for a package path, the user
// will be redirected to the details page.
func (s *Server) serveSearch(w http.ResponseWriter, r *http.Request, ds internal.DataSource) error {
	if r.Method != http.MethodGet {
		return &serverError{status: http.StatusMethodNotAllowed}
	}
	db, ok := ds.(*postgres.DB)
	if !ok {
		// The proxydatasource does not support the imported by page.
		return proxydatasourceNotSupportedErr()
	}

	ctx := r.Context()
	query := searchQuery(r)
	if query == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return nil
	}

	if path := searchRequestRedirectPath(ctx, ds, query); path != "" {
		http.Redirect(w, r, path, http.StatusFound)
		return nil
	}
	page, err := fetchSearchPage(ctx, db, query, newPaginationParams(r, defaultSearchLimit))
	if err != nil {
		return fmt.Errorf("fetchSearchPage(ctx, db, %q): %v", query, err)
	}
	page.basePage = s.newBasePage(r, query)
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
	requestedPath := path.Clean(query)
	if !strings.Contains(requestedPath, "/") {
		return ""
	}
	if experiment.IsActive(ctx, internal.ExperimentUsePathInfo) {
		modulePath, _, isPackage, err := ds.GetPathInfo(ctx, requestedPath, internal.UnknownModulePath, internal.LatestVersion)
		if err != nil {
			if !errors.Is(err, derrors.NotFound) {
				log.Errorf(ctx, "searchRequestRedirectPath(%q): %v", requestedPath, err)
			}
			return ""
		}
		if isPackage || modulePath != requestedPath {
			return fmt.Sprintf("/%s", requestedPath)
		}
		return fmt.Sprintf("/mod/%s", requestedPath)
	}

	pkg, err := ds.LegacyGetPackage(ctx, requestedPath, internal.UnknownModulePath, internal.LatestVersion)
	if err == nil {
		return fmt.Sprintf("/%s", pkg.Path)
	} else if !errors.Is(err, derrors.NotFound) {
		log.Errorf(ctx, "error getting package for %s: %v", requestedPath, err)
		return ""
	}
	mi, err := ds.LegacyGetModuleInfo(ctx, requestedPath, internal.LatestVersion)
	if err == nil {
		return fmt.Sprintf("/mod/%s", mi.ModulePath)
	} else if !errors.Is(err, derrors.NotFound) {
		log.Errorf(ctx, "error getting module for %s: %v", requestedPath, err)
		return ""
	}
	dir, err := ds.LegacyGetDirectory(ctx, requestedPath, internal.UnknownModulePath, internal.LatestVersion, internal.AllFields)
	if err == nil {
		return fmt.Sprintf("/%s", dir.Path)
	} else if !errors.Is(err, derrors.NotFound) {
		log.Errorf(ctx, "error getting directory for %s: %v", requestedPath, err)
		return ""
	}
	return ""
}

// searchQuery extracts a search query from the request.
func searchQuery(r *http.Request) string {
	return strings.TrimSpace(r.FormValue("q"))
}
