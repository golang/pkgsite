// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/xerrors"
)

var (
	// SearchLatency holds observed latency in individual search queries.
	SearchLatency = stats.Float64(
		"go-discovery/search/latency",
		"Latency of a search query.",
		stats.UnitMilliseconds,
	)
	// SearchSource is a census tag for search query types.
	SearchSource = tag.MustNewKey("search.source")
	// SearchLatencyDistribution aggregates search request latency by search
	// query type.
	SearchLatencyDistribution = &view.View{
		Name:        "go-discovery/search/latency",
		Measure:     SearchLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Search latency, by result source query type.",
		TagKeys:     []tag.Key{SearchSource},
	}
	// SearchResponseCount counts search responses by search query type.
	SearchResponseCount = &view.View{
		Name:        "go-discovery/search/count",
		Measure:     SearchLatency,
		Aggregation: view.Count(),
		Description: "Search count, by result source query type.",
		TagKeys:     []tag.Key{SearchSource},
	}

	errIncompleteResults = errors.New("incomplete results")
)

// SearchResult represents a single search result from SearchDocuments.
type SearchResult struct {
	Name        string
	PackagePath string
	ModulePath  string
	Version     string
	Synopsis    string
	Licenses    []string

	CommitTime time.Time
	// Score is used to sort items in an array of SearchResult.
	Score float64

	// NumImportedBy is the number of packages that import Package.
	NumImportedBy uint64

	// NumResults is the total number of packages that were returned for this search.
	NumResults uint64
	// Approximate reports whether NumResults is an approximate count. NumResults
	// can be approximate if search scanned only a subset of documents, and
	// result count is estimated using the hyperloglog algorithm.
	Approximate bool
}

// searchResponse is used for internal bookkeeping when fanning-out search
// request to multiple different search queries.
type searchResponse struct {
	// source is a unique identifier for the search query type (e.g. 'deep',
	// 'popular-8'), to be used in logging and reporting.
	source string
	// results are partially filled out from only the search_documents table.
	results []*SearchResult
	// err indicates a technical failure of the search query, or that results are
	// not provably complete.
	err error
	// latency is recorded by the orchestrator of the search query.
	latency time.Duration
	// uncounted reports whether this response is missing total result counts. If
	// uncounted is true, search will wait for either the hyperloglog count
	// estimate, or for an alternate search method to return with
	// uncounted=false.
	uncounted bool
}

// A searcher is used to execute a single search request.
type searcher func(ctx context.Context, q string, limit, offset int) searchResponse

// FastSearch executes two search requests concurrently:
//   - a sequential scan of packages in descending order of popularity.
//   - all packages ("deep" search) using an inverted index to filter to search
//     terms.
//
// The sequential scan takes significantly less time when searching for very
// common terms (e.g. "errors", "cloud", or "kubernetes"), due to its ability
// to exit early once the requested page of search results is provably
// complete.
//
// Because 0 <= ts_rank() <= 1, we know that the highest score of any unscanned
// package is ln(e+N), where N is imported_by_count of the package we are
// currently considering.  Therefore if the lowest scoring result of popular
// search is greater than ln(e+N), we know that we haven't missed any results
// and can return the search result immediately, cancelling other searches.
//
// On the other hand, if the popular search is slow, it is likely that the
// search term is infrequent, and deep search will be fast due to our inverted
// gin index on search tokens.
//
// The gap in this optimization is search terms that are very frequent, but
// rarely relevant: "int" or "package", for example. In these cases we'll pay
// the penalty of a deep search that scans nearly every package.
func (db *DB) FastSearch(ctx context.Context, q string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.FastSearch(ctx, %q, %d, %d)", q, limit, offset)
	searchers := []searcher{db.popularSearch, db.deepSearch}
	return db.hedgedSearch(ctx, q, limit, offset, searchers, nil)
}

// PartialFastSearch implements a hedged search using partial indexes of
// popular packages.
// TODO(b/141182438) delete this once a testing period is over.
func (db *DB) PartialFastSearch(ctx context.Context, q string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.PartialFastSearch(ctx, %q, %d, %d)", q, limit, offset)
	searchers := []searcher{db.popularSearcher(50), db.popularSearcher(8), db.deepSearch}
	return db.hedgedSearch(ctx, q, limit, offset, searchers, nil)
}

// hedgedSearch executes multiple search methods and returns the first
// available result.
// The optional guardTestResult func may be used to allow tests to control the
// order in which search results are returned.
func (db *DB) hedgedSearch(ctx context.Context, q string, limit, offset int, searchers []searcher, guardTestResult func(string) func()) ([]*SearchResult, error) {
	responses := make(chan searchResponse, len(searchers))
	// cancel all unfinished searches when a result (or error) is returned. The
	// effectiveness of this depends on the database driver.
	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Asynchronously query for the estimated result count.
	estimateChan := make(chan estimateResponse, 1)
	go func() {
		estimateResp := db.estimateResultsCount(searchCtx, q)
		if guardTestResult != nil {
			defer guardTestResult("estimate")()
		}
		estimateChan <- estimateResp
	}()

	// Fan out our search requests.
	for _, s := range searchers {
		s := s
		go func() {
			start := time.Now()
			resp := s(searchCtx, q, limit, offset)
			resp.latency = time.Since(start)
			if guardTestResult != nil {
				defer guardTestResult(resp.source)()
			}
			responses <- resp
		}()
	}
	var resp searchResponse
	for range searchers {
		resp = <-responses
		if resp.err == nil {
			break
		}
	}
	if resp.err != nil {
		return nil, fmt.Errorf("all searchers failed: %v", resp.err)
	}
	if resp.uncounted {
		// Since the response is uncounted, we should wait for either the count
		// estimate to return, or for the first counted response.
	loop:
		for {
			select {
			case nextResp := <-responses:
				if nextResp.err == nil && !nextResp.uncounted {
					// use this response since it is counted.
					resp = nextResp
					break loop
				}
			case estr := <-estimateChan:
				if estr.err != nil {
					return nil, fmt.Errorf("error getting estimated result count: %v", estr.err)
				}
				for _, r := range resp.results {
					// TODO(b/141182438): this is a hack: once search has been fully
					// replaced with fastsearch, change the return signature of this
					// function to separate result-level data from this metadata.
					r.NumResults = estr.estimate
					r.Approximate = true
				}
				break loop
			//case <-responses:
			case <-ctx.Done():
				return nil, fmt.Errorf("context deadline exceeded while waiting for estimated result count")
			}
		}
	}
	// cancel proactively here: we've got the search result we need.
	cancel()
	// latency is only recorded for valid search results, as fast failures could
	// skew the latency distribution.
	latency := float64(resp.latency) / float64(time.Millisecond)
	stats.RecordWithTags(ctx, []tag.Mutator{
		tag.Upsert(SearchSource, resp.source),
	}, SearchLatency.M(latency))
	// To avoid fighting with the query planner, our searches only hit the
	// search_documents table and we enrich after getting the results. In the
	// future, we may want to fully denormalize and put all search data in the
	// search_documents table.
	if err := db.addPackageDataToSearchResults(ctx, resp.results); err != nil {
		return nil, err
	}
	return resp.results, nil
}

const hllRegisterCount = 128

// hllQuery estimates search result counts using the hyperloglog algorithm.
// https://en.wikipedia.org/wiki/HyperLogLog
//
// Here's how this works:
//   1) Search documents have been partitioned ~evenly into hllRegisterCount
//   registers, using the hll_register column. For each hll_register, compute
//   the maximum number of leading zeros of any element in the register
//   matching our search query. This is the slowest part of the query, but
//   since we have an index on (hll_register, hll_leading_zeros desc), we can
//   parallelize this and it should be very quick if the density of search
//   results is high.  To achieve this parallelization, we use a trick of
//   selecting a subselected value from generate_series(0, hllRegisterCount-1).
//
//   If there are NO search results in a register, the 'zeros' column will be
//   NULL.
//
//   2) From the results of (1), proceed following the 'Practical
//   Considerations' in the wikipedia page above:
//     https://en.wikipedia.org/wiki/HyperLogLog#Practical_Considerations
//   Specifically, use linear counting when E < (5/2)m and there are empty
//   registers.
//
//   This should work for any register count >= 128. If we are to decrease this
//   register count, we should adjust the estimate for a_m below according to
//   the formulas in the wikipedia article above.
var hllQuery = fmt.Sprintf(`
	WITH hll_data AS (
		SELECT (
			SELECT * FROM (
				SELECT hll_leading_zeros
				FROM search_documents
				WHERE (
					ts_rank(tsv_search_tokens, websearch_to_tsquery($1)) *
					ln(exp(1)+imported_by_count) *
					CASE WHEN redistributable then 1 else 0.5 end *
					CASE WHEN tsv_search_tokens @@ websearch_to_tsquery($1) THEN 1 ELSE 0 END
				) > 0.1
				AND hll_register=generate_series
				ORDER BY hll_leading_zeros DESC
			) t
			LIMIT 1
		) zeros
		FROM generate_series(0,%[1]d-1)
	),
	nonempty_registers as (SELECT zeros FROM hll_data WHERE zeros IS NOT NULL)
	SELECT
		-- use linear counting when there are not enough results, and there is at
		-- least one empty register, per 'Practical Considerations'.
		CASE WHEN result_count < 2.5 * %[1]d AND empty_register_count > 0
		THEN ((0.7213 / (1 + 1.079 / %[1]d)) * (%[1]d *
				log(2, (%[1]d::numeric) / empty_register_count)))::int
		ELSE result_count END AS approx_count
	FROM (
		SELECT
			(
				(0.7213 / (1 + 1.079 / %[1]d)) *  -- estimate for a_m
				pow(%[1]d, 2) *                   -- m^2
				(1/((%[1]d - count(1)) + SUM(POW(2, -1 * (zeros+1)))))  -- Z
			)::int AS result_count,
			%[1]d - count(1) AS empty_register_count
		FROM nonempty_registers
	) d`, hllRegisterCount)

type estimateResponse struct {
	estimate uint64
	err      error
}

// EstimateResultsCount uses the hyperloglog algorithm to estimate the number
// of results for the given search term.
func (db *DB) estimateResultsCount(ctx context.Context, q string) estimateResponse {
	row := db.queryRow(ctx, hllQuery, q)
	var estimate sql.NullInt64
	if err := row.Scan(&estimate); err != nil {
		return estimateResponse{err: fmt.Errorf("row.Scan(): %v", err)}
	}
	// If estimate is NULL, then we didn't find *any* results, so should return
	// zero (the default).
	return estimateResponse{estimate: uint64(estimate.Int64)}
}

// deepSearch searches all packages for the query. It is slower, but results
// are always valid.
func (db *DB) deepSearch(ctx context.Context, q string, limit, offset int) searchResponse {
	query := `
		SELECT *, COUNT(*) OVER() AS total
		FROM (
			SELECT
				package_path,
				version,
				module_path,
				commit_time,
				imported_by_count,
				(
					ts_rank(tsv_search_tokens, websearch_to_tsquery($1)) *
					ln(exp(1)+imported_by_count) *
					CASE WHEN redistributable THEN 1 ELSE 0.5 END
				) AS score
				FROM
					search_documents
				WHERE tsv_search_tokens @@ websearch_to_tsquery($1)
				ORDER BY
					score DESC,
					commit_time DESC,
					package_path
		) r
		WHERE r.score > 0.1
		LIMIT $2
		OFFSET $3`
	var results []*SearchResult
	collect := func(rows *sql.Rows) error {
		var r SearchResult
		if err := rows.Scan(&r.PackagePath, &r.Version, &r.ModulePath, &r.CommitTime,
			&r.NumImportedBy, &r.Score, &r.NumResults); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		results = append(results, &r)
		return nil
	}
	err := db.runQuery(ctx, query, collect, q, limit, offset)
	if err != nil {
		results = nil
	}
	return searchResponse{
		source:  "deep",
		results: results,
		err:     err,
	}
}

func (db *DB) popularSearch(ctx context.Context, searchQuery string, limit, offset int) searchResponse {
	query := `
		SELECT
			package_path,
			version,
			module_path,
			commit_time,
			imported_by_count,
			score
		FROM popular_search($1, $2, $3)`
	var results []*SearchResult
	collect := func(rows *sql.Rows) error {
		var r SearchResult
		if err := rows.Scan(&r.PackagePath, &r.Version, &r.ModulePath, &r.CommitTime,
			&r.NumImportedBy, &r.Score); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		results = append(results, &r)
		return nil
	}
	err := db.runQuery(ctx, query, collect, searchQuery, limit, offset)
	if err != nil {
		results = nil
	}
	return searchResponse{
		source:    "popular",
		results:   results,
		err:       err,
		uncounted: true,
	}
}

// popularSearcher returns a searcher that only searches packages with more
// than cutoff importers. Results can be invalid if it does not return the
// limit of results, all of which have greater score than the highest
// theoretical score of an unpopular package.
// TODO(b/141182438): remove this once the trial period with 'popularSearch' is
// over, and drop the partial indexes.
func (db *DB) popularSearcher(cutoff int) searcher {
	return func(ctx context.Context, searchQuery string, limit, offset int) searchResponse {
		query := fmt.Sprintf(`SELECT *
			FROM (
				SELECT
					package_path,
					version,
					module_path,
					commit_time,
					imported_by_count,
					(
						ts_rank(tsv_search_tokens, websearch_to_tsquery($1)) *
						ln(exp(1)+imported_by_count) *
						CASE WHEN redistributable THEN 1 ELSE 0.5 END
					) AS score
					FROM
						search_documents
					WHERE tsv_search_tokens @@ websearch_to_tsquery($1)
					AND imported_by_count > %[1]d
					ORDER BY
						score DESC,
						commit_time DESC,
						package_path
			) r
			WHERE r.score > ln(exp(1)+%[1]d)
			LIMIT $2
			OFFSET $3`, cutoff)
		var results []*SearchResult
		collect := func(rows *sql.Rows) error {
			var r SearchResult
			// Notably we're not recording r.NumResults here. There's no point, as
			// we're only scanning a fraction of the total records. In the UI this
			// should be presented as '1-10 of many'.
			//
			// For a potential future improvement, we could implement the hyperloglog
			// algorithm to estimate result counts.
			if err := rows.Scan(&r.PackagePath, &r.Version, &r.ModulePath, &r.CommitTime,
				&r.NumImportedBy, &r.Score); err != nil {
				return fmt.Errorf("rows.Scan(): %v", err)
			}
			results = append(results, &r)
			return nil
		}
		err := db.runQuery(ctx, query, collect, searchQuery, limit, offset)
		if err != nil {
			results = nil
		} else if len(results) != limit {
			// We didn't get a provably complete set of results.
			err = errIncompleteResults
		}
		return searchResponse{
			source:    fmt.Sprintf("popular-%d", cutoff),
			results:   results,
			err:       err,
			uncounted: true,
		}
	}
}

// addPackageDataToSearchResults adds package information to SearchResults that is not stored
// in the search_documents table.
func (db *DB) addPackageDataToSearchResults(ctx context.Context, results []*SearchResult) (err error) {
	defer derrors.Wrap(&err, "DB.enrichResults(results)")
	if len(results) == 0 {
		return nil
	}
	var (
		keys []string
		// resultMap tracks PackagePath->SearchResult, to allow joining with the
		// returned package data.
		resultMap = make(map[string]*SearchResult)
	)
	for _, r := range results {
		resultMap[r.PackagePath] = r
		key := fmt.Sprintf("(%s, %s, %s)", pq.QuoteLiteral(r.PackagePath),
			pq.QuoteLiteral(r.Version), pq.QuoteLiteral(r.ModulePath))
		keys = append(keys, key)
	}
	query := fmt.Sprintf(`
		SELECT
			path,
			name,
			synopsis,
			license_types
		FROM
			packages
		WHERE
			(path, version, module_path) IN (%s)`, strings.Join(keys, ","))
	collect := func(rows *sql.Rows) error {
		var (
			path, name, synopsis string
			licenseTypes         []string
		)
		if err := rows.Scan(&path, &name, &synopsis, pq.Array(&licenseTypes)); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		r, ok := resultMap[path]
		if !ok {
			return fmt.Errorf("BUG: unexpected package path: %q", path)
		}
		r.Name = name
		r.Synopsis = synopsis
		for _, l := range licenseTypes {
			if l != "" {
				r.Licenses = append(r.Licenses, l)
			}
		}
		return nil
	}
	return db.runQuery(ctx, query, collect)
}

// DeepSearch executes a full scan of the search table in two steps, by first
// querying and then enriching.
// TODO(b/141182438) delete this once a testing period is over.
func (db *DB) DeepSearch(ctx context.Context, q string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.DeepSearch(ctx, %q, %d, %d)", q, limit, offset)
	resp := db.deepSearch(ctx, q, limit, offset)

	if resp.err != nil {
		return nil, resp.err
	}
	if err := db.addPackageDataToSearchResults(ctx, resp.results); err != nil {
		return nil, err
	}
	return resp.results, nil
}

// PopularSearch executes a sequential scan of the search table in descending
// order of popularity.
func (db *DB) PopularSearch(ctx context.Context, q string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.PopularSearch(ctx, %q, %d, %d)", q, limit, offset)
	resp := db.popularSearch(ctx, q, limit, offset)

	if resp.err != nil {
		return nil, resp.err
	}
	if err := db.addPackageDataToSearchResults(ctx, resp.results); err != nil {
		return nil, err
	}
	return resp.results, nil

}

// Search fetches packages from the database that match the terms
// provided, and returns them in order of relevance.
// TODO(b/141182438) delete this once a testing period is over.
func (db *DB) Search(ctx context.Context, q string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.Search(ctx, %q, %d, %d)", q, limit, offset)

	// Score:
	// Packages are scored based on their relevance and imported_by_count. If
	// the package is not redistributable, lower its score by 50% since a lot of
	// details cannot be displayed.
	//
	// TODO(b/136283982): improve how this signal is used in search scoring.
	// The log factor contains exp(1) so that it is always >= 1. Taking the log
	// of imported_by_count instead of using it directly makes the effect less
	// dramatic: being 2x as popular only has an additive effect.
	//
	// Only include results whose score exceed a certain threshold. Based on
	// experimentation, we picked a score of greater than 0.1, but this may
	// change based on future experimentation.
	query := `
		SELECT
			r.package_path,
			r.version,
			r.module_path,
			p.NAME,
			p.synopsis,
			p.license_types,
			r.commit_time,
			r.imported_by_count,
			r.score,
			r.total
		FROM (
			SELECT
				package_path,
				version,
				module_path,
				imported_by_count,
				commit_time,
				COUNT(*) OVER() AS total,
				(
					ts_rank(tsv_search_tokens, websearch_to_tsquery($1)) *
					ln(exp(1)+imported_by_count) *
					CASE WHEN redistributable THEN 1
					ELSE 0.5 END
				) AS score
                    	FROM
				search_documents
                    	WHERE
				tsv_search_tokens @@ websearch_to_tsquery($1)
                    	ORDER BY
				score DESC,
				commit_time DESC,
				package_path
			LIMIT $2
			OFFSET $3
		) r
		INNER JOIN
			packages p
		ON
			p.path = r.package_path
		AND
			p.module_path = r.module_path
			AND p.version = r.version
		WHERE
			r.score > 0.1;`

	var results []*SearchResult
	collect := func(rows *sql.Rows) error {
		var (
			sr           SearchResult
			licenseTypes []string
		)
		if err := rows.Scan(&sr.PackagePath, &sr.Version, &sr.ModulePath, &sr.Name, &sr.Synopsis,
			pq.Array(&licenseTypes), &sr.CommitTime, &sr.NumImportedBy, &sr.Score, &sr.NumResults); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		for _, l := range licenseTypes {
			if l != "" {
				sr.Licenses = append(sr.Licenses, l)
			}
		}
		results = append(results, &sr)
		return nil
	}
	if err := db.runQuery(ctx, query, collect, q, limit, offset); err != nil {
		return nil, err
	}
	return results, nil
}

var upsertSearchStatement = fmt.Sprintf(`
	INSERT INTO search_documents (
		package_path,
		version,
		module_path,
		name,
		synopsis,
		license_types,
		redistributable,
		version_updated_at,
		commit_time,
		tsv_search_tokens,
		hll_register,
		hll_leading_zeros
	)
	SELECT
		p.path,
		p.version,
		p.module_path,
		p.name,
		p.synopsis,
		p.license_types,
		p.redistributable,
		CURRENT_TIMESTAMP,
		v.commit_time,
		(
			SETWEIGHT(TO_TSVECTOR($2), 'A') ||
			-- Try to limit to the maximum length of a tsvector.
			-- This is just a guess, since the max length is in bytes and there
			-- doesn't seem to be a way to determine the number of bytes in a tsvector.
			-- Since the max is 1048575, make sure part is half that size.
			SETWEIGHT(TO_TSVECTOR(left(p.synopsis, 1048575/2)), 'B') ||
			SETWEIGHT(TO_TSVECTOR(left(v.readme_contents, 1048575/2)), 'C')
		),
		hll_hash(p.path) & (%[1]d - 1),
		hll_zeros(hll_hash(p.path))
	FROM
		packages p
	INNER JOIN
		versions v
	ON
		p.module_path = v.module_path
		AND p.version = v.version
	WHERE
		p.path = $1
	ORDER BY
		-- Order the versions by release then prerelease.
		-- The default version should be the first release
		-- version available, if one exists.
		CASE WHEN v.prerelease = '~' THEN 0 ELSE 1 END,
		v.major DESC,
		v.minor DESC,
		v.patch DESC,
		v.prerelease DESC,
		v.module_path DESC
	LIMIT 1
	ON CONFLICT (package_path)
	DO UPDATE SET
		package_path=excluded.package_path,
		version=excluded.version,
		module_path=excluded.module_path,
		tsv_search_tokens=excluded.tsv_search_tokens,
		commit_time=excluded.commit_time,
		version_updated_at=(
			CASE WHEN excluded.version = search_documents.version
			THEN search_documents.version_updated_at
			ELSE CURRENT_TIMESTAMP
			END)
	;`, hllRegisterCount)

// UpsertSearchDocument inserts a row for each package in the version, if that
// package is the latest version.
//
// The given version should have already been validated via a call to
// validateVersion.
func (db *DB) UpsertSearchDocument(ctx context.Context, path string) (err error) {
	defer derrors.Wrap(&err, "UpsertSearchDocument(ctx, %q)", path)

	if isInternalPackage(path) {
		return xerrors.Errorf("cannot insert internal package %q into search documents: %w", path, derrors.InvalidArgument)
	}

	pathTokens := strings.Join(generatePathTokens(path), " ")
	_, err = db.exec(ctx, upsertSearchStatement, path, pathTokens)
	return err
}

// GetPackagesForSearchDocumentUpsert fetches all paths from packages that do
// not exist in search_documents.
func (db *DB) GetPackagesForSearchDocumentUpsert(ctx context.Context, limit int) (paths []string, err error) {
	defer derrors.Add(&err, "GetPackagesForSearchDocumentUpsert(ctx, %d)", limit)

	query := `
		SELECT DISTINCT(path)
		FROM packages p
		LEFT JOIN search_documents sd
		ON p.path = sd.package_path
		WHERE sd.package_path IS NULL
		LIMIT $1`

	collect := func(rows *sql.Rows) error {
		var path string
		if err := rows.Scan(&path); err != nil {
			return err
		}
		// Filter out packages in internal directories, since
		// they are skipped when upserting search_documents.
		if !isInternalPackage(path) {
			paths = append(paths, path)
		}
		return nil
	}
	if err := db.runQuery(ctx, query, collect, limit); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

type searchDocument struct {
	packagePath              string
	modulePath               string
	redistributable          bool
	version                  string
	importedByCount          int
	commitTime               time.Time
	versionUpdatedAt         time.Time
	importedByCountUpdatedAt time.Time
}

// getSearchDocument returns the search_document for the package with the given
// path. It is only used for testing purposes.
func (db *DB) getSearchDocument(ctx context.Context, path string) (*searchDocument, error) {
	query := `
		SELECT
			package_path,
			module_path,
			redistributable,
			version,
			imported_by_count,
			commit_time,
			version_updated_at,
			imported_by_count_updated_at
		FROM
			search_documents
		WHERE package_path=$1`
	row := db.queryRow(ctx, query, path)
	var (
		sd searchDocument
		t  pq.NullTime
	)
	if err := row.Scan(&sd.packagePath, &sd.modulePath,
		&sd.redistributable, &sd.version, &sd.importedByCount,
		&sd.commitTime, &sd.versionUpdatedAt, &t); err != nil {
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	if t.Valid {
		sd.importedByCountUpdatedAt = t.Time
	}
	return &sd, nil
}

// UpdateSearchDocumentsImportedByCount updates imported_by_count and
// imported_by_count_updated_at.
//
// It does so by completely recalculating the imported-by counts
// from the imports_unique table.
//
// UpdateSearchDocumentsImportedByCount returns the number of rows updated.
func (db *DB) UpdateSearchDocumentsImportedByCount(ctx context.Context) (nUpdated int64, err error) {
	defer derrors.Wrap(&err, "UpdateSearchDocumentsImportedByCount(ctx)")

	counts, err := db.computeImportedByCounts(ctx)
	if err != nil {
		return 0, err
	}
	err = db.Transact(func(tx *sql.Tx) error {
		if err := insertImportedByCounts(ctx, tx, counts); err != nil {
			return err
		}
		if err := compareImportedByCounts(ctx, tx); err != nil {
			return err
		}
		nUpdated, err = updateImportedByCounts(ctx, tx)
		return err
	})
	return nUpdated, err
}

func (db *DB) computeImportedByCounts(ctx context.Context) (counts map[string]int, err error) {
	defer derrors.Wrap(&err, "db.computeImportedByCounts(ctx)")

	counts = map[string]int{}
	// Get all (from_path, to_path) pairs, deduped.
	// Also get the from_path's module path.
	rows, err := db.query(ctx, `
		SELECT
			from_path, from_module_path, to_path
		FROM
			imports_unique
		GROUP BY
			from_path, from_module_path, to_path;
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var from, fromMod, to string
		if err := rows.Scan(&from, &fromMod, &to); err != nil {
			return nil, err
		}
		// Don't count an importer if it's in the same module as what it's importing.
		// Approximate that check by seeing if from_module_path is a prefix of to_path.
		// (In some cases, e.g. when to_path is in a nested module, that is not correct.)
		if (fromMod == stdlib.ModulePath && stdlib.Contains(to)) || strings.HasPrefix(to+"/", fromMod+"/") {
			continue
		}
		counts[to]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func insertImportedByCounts(ctx context.Context, tx *sql.Tx, counts map[string]int) (err error) {
	defer derrors.Wrap(&err, "insertImportedByCounts(ctx, tx, counts)")

	const createTableQuery = `
		CREATE TEMPORARY TABLE computed_imported_by_counts (
			package_path      TEXT NOT NULL,
			imported_by_count INTEGER DEFAULT 0 NOT NULL
		) ON COMMIT DROP;
    `
	if _, err := execTx(ctx, tx, createTableQuery); err != nil {
		return fmt.Errorf("CREATE TABLE: %v", err)
	}
	values := make([]interface{}, 0, 2*len(counts))
	for p, c := range counts {
		values = append(values, p, c)
	}
	columns := []string{"package_path", "imported_by_count"}
	return bulkInsert(ctx, tx, "computed_imported_by_counts", columns, values, "")
}

func compareImportedByCounts(ctx context.Context, tx *sql.Tx) (err error) {
	defer derrors.Wrap(&err, "compareImportedByCounts(ctx, tx)")

	rows, err := tx.QueryContext(ctx, `
		SELECT
			s.package_path,
			s.imported_by_count,
			c.imported_by_count
		FROM
			search_documents s
		INNER JOIN
			computed_imported_by_counts c
		ON
			s.package_path = c.package_path;`)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Compute some info about the changes to import-by counts.
	const changeThreshold = 0.05 // count how many counts change by at least this fraction
	var total, zero, change, diff int
	for rows.Next() {
		var path string
		var old, new int
		if err := rows.Scan(&path, &old, &new); err != nil {
			return err
		}
		total++
		if old != new {
			change++
		}
		if old == 0 {
			zero++
			continue
		}
		fracDiff := math.Abs(float64(new-old)) / float64(old)
		if fracDiff > changeThreshold {
			diff++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Infof("%6d total rows in search_documents match computed_imported_by_counts", total)
	log.Infof("%6d will change", change)
	log.Infof("%6d currently have a zero imported-by count", zero)
	log.Infof("%6d of the non-zero rows will change by more than %d%%", diff, int(changeThreshold*100))
	return nil
}

// updateImportedByCounts updates the imported_by_count column in search_documents
// for every package in computed_imported_by_counts.
//
// A row is updated even if the value doesn't change, so that the imported_by_count_updated_at
// column is set.
//
// Note that if a package is never imported, its imported_by_count column will
// be the default (0) and its imported_by_count_updated_at column will never be set.
func updateImportedByCounts(ctx context.Context, tx *sql.Tx) (int64, error) {
	const updateStmt = `
		UPDATE search_documents s
		SET
			imported_by_count = c.imported_by_count,
			imported_by_count_updated_at = CURRENT_TIMESTAMP
		FROM computed_imported_by_counts c
		WHERE s.package_path = c.package_path;`

	res, err := execTx(ctx, tx, updateStmt)
	if err != nil {
		return 0, fmt.Errorf("error updating imported_by_count and imported_by_count_updated_at for search documents: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("RowsAffected: %v", err)
	}
	return n, nil
}

var (
	commonHostnames = map[string]bool{
		"bitbucket.org":         true,
		"code.cloudfoundry.org": true,
		"gitea.com":             true,
		"gitee.com":             true,
		"github.com":            true,
		"gitlab.com":            true,
		"go.etcd.io":            true,
		"go.googlesource.com":   true,
		"golang.org":            true,
		"google.golang.org":     true,
		"gopkg.in":              true,
	}
	commonHostParts = map[string]bool{
		"code":   true,
		"git":    true,
		"gitlab": true,
		"go":     true,
		"google": true,
		"www":    true,
	}
)

// generatePathTokens returns the subPaths and path token parts that will be
// indexed for search, which includes (1) the packagePath (2) all sub-paths of
// the packagePath (3) all parts for a path element that is delimited by a dash
// and (4) all parts of a path element that is delimited by a dot, except for
// the last element.
func generatePathTokens(packagePath string) []string {
	packagePath = strings.Trim(packagePath, "/")

	subPathSet := make(map[string]bool)
	parts := strings.Split(packagePath, "/")
	for i, part := range parts {
		dashParts := strings.Split(part, "-")
		if len(dashParts) > 1 {
			for _, p := range dashParts {
				subPathSet[p] = true
			}
		}
		for j := i + 2; j <= len(parts); j++ {
			p := strings.Join(parts[i:j], "/")
			p = strings.Trim(p, "/")
			subPathSet[p] = true
		}

		if i == 0 && commonHostnames[part] {
			continue
		}
		// Only index host names if they are not part of commonHostnames.
		// Note that because "SELECT to_tsvector('github.com/foo/bar')"
		// will return "github.com" as one of its tokens, the common host
		// name will still be indexed until we change the pg search_config.
		// TODO(b/141318673).
		subPathSet[part] = true
		dotParts := strings.Split(part, ".")
		if len(dotParts) > 1 {
			for _, p := range dotParts[:len(dotParts)-1] {
				if !commonHostParts[p] {
					// If the host is not in commonHostnames, we want to
					// index each element up to the extension. For example,
					// if the host is sigs.k8s.io, we want to index sigs
					// and k8s. Skip common host parts.
					subPathSet[p] = true
				}
			}
		}
	}

	var subPaths []string
	for sp := range subPathSet {
		if len(sp) > 0 {
			subPaths = append(subPaths, sp)
		}
	}
	return subPaths
}

// isInternalPackage reports whether the path represents an internal directory.
func isInternalPackage(path string) bool {
	for _, p := range strings.Split(path, "/") {
		if p == "internal" {
			return true
		}
	}
	return false
}
