// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
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
	"go.opencensus.io/trace"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
)

var (
	// searchLatency holds observed latency in individual search queries.
	searchLatency = stats.Float64(
		"go-discovery/search/latency",
		"Latency of a search query.",
		stats.UnitMilliseconds,
	)
	// keySearchSource is a census tag for search query types.
	keySearchSource = tag.MustNewKey("search.source")
	// SearchLatencyDistribution aggregates search request latency by search
	// query type.
	SearchLatencyDistribution = &view.View{
		Name:        "go-discovery/search/latency",
		Measure:     searchLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Search latency, by result source query type.",
		TagKeys:     []tag.Key{keySearchSource},
	}
	// SearchResponseCount counts search responses by search query type.
	SearchResponseCount = &view.View{
		Name:        "go-discovery/search/count",
		Measure:     searchLatency,
		Aggregation: view.Count(),
		Description: "Search count, by result source query type.",
		TagKeys:     []tag.Key{keySearchSource},
	}
)

// searchResponse is used for internal bookkeeping when fanning-out search
// request to multiple different search queries.
type searchResponse struct {
	// source is a unique identifier for the search query type (e.g. 'deep',
	// 'popular'), to be used in logging and reporting.
	source string
	// results are partially filled out from only the search_documents table.
	results []*internal.SearchResult
	// err indicates a technical failure of the search query, or that results are
	// not provably complete.
	err error
	// uncounted reports whether this response is missing total result counts. If
	// uncounted is true, search will wait for either the hyperloglog count
	// estimate, or for an alternate search method to return with
	// uncounted=false.
	uncounted bool
}

// searchEvent is used to log structured information about search events for
// later analysis. A 'search event' occurs when a searcher or count estimate
// returns.
type searchEvent struct {
	// Type is either the searcher name or 'estimate' (the count estimate).
	Type string
	// Latency is the duration that that the operation took.
	Latency time.Duration
	// Err is the error returned by the operation, if any.
	Err error
}

// A searcher is used to execute a single search request.
type searcher func(db *DB, ctx context.Context, q string, limit, offset int) searchResponse

// The searchers used by Search.
var searchers = map[string]searcher{
	"popular": (*DB).popularSearch,
	"deep":    (*DB).deepSearch,
}

// Search executes two search requests concurrently:
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
func (db *DB) Search(ctx context.Context, q string, limit, offset int) (_ []*internal.SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.Search(ctx, %q, %d, %d)", q, limit, offset)
	resp, err := db.hedgedSearch(ctx, q, limit, offset, searchers, nil)
	if err != nil {
		return nil, err
	}
	// Filter out excluded paths.
	var results []*internal.SearchResult
	for _, r := range resp.results {
		ex, err := db.IsExcluded(ctx, r.PackagePath)
		if err != nil {
			return nil, err
		}
		if !ex {
			results = append(results, r)
		}
	}
	return results, nil
}

// Penalties to search scores, applied as multipliers to the score.
const (
	// Module license is non-redistributable.
	nonRedistributablePenalty = 0.5
	// Module does not have a go.mod file.
	// Start this off gently (close to 1), but consider lowering
	// it as time goes by and more of the ecosystem converts to modules.
	noGoModPenalty = 0.8
)

// scoreExpr is the expression that computes the search score.
// It is the product of:
// - The Postgres ts_rank score, based the relevance of the document to the query.
// - The log of the module's popularity, estimated by the number of importing packages.
//   The log factor contains exp(1) so that it is always >= 1. Taking the log
//   of imported_by_count instead of using it directly makes the effect less
//   dramatic: being 2x as popular only has an additive effect.
// - A penalty factor for non-redistributable modules, since a lot of
//   details cannot be displayed.
// The first argument to ts_rank is an array of weights for the four tsvector sections,
// in the order D, C, B, A.
// The weights below match the defaults except for B.
var scoreExpr = fmt.Sprintf(`
		ts_rank('{0.1, 0.2, 1.0, 1.0}', tsv_search_tokens, websearch_to_tsquery($1)) *
		ln(exp(1)+imported_by_count) *
		CASE WHEN redistributable THEN 1 ELSE %f END *
		CASE WHEN COALESCE(has_go_mod, true) THEN 1 ELSE %f END
	`, nonRedistributablePenalty, noGoModPenalty)

// hedgedSearch executes multiple search methods and returns the first
// available result.
// The optional guardTestResult func may be used to allow tests to control the
// order in which search results are returned.
func (db *DB) hedgedSearch(ctx context.Context, q string, limit, offset int, searchers map[string]searcher, guardTestResult func(string) func()) (*searchResponse, error) {
	searchStart := time.Now()
	responses := make(chan searchResponse, len(searchers))
	// cancel all unfinished searches when a result (or error) is returned. The
	// effectiveness of this depends on the database driver.
	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Asynchronously query for the estimated result count.
	estimateChan := make(chan estimateResponse, 1)
	go func() {
		start := time.Now()
		estimateResp := db.estimateResultsCount(searchCtx, q)
		log.Debug(ctx, searchEvent{
			Type:    "estimate",
			Latency: time.Since(start),
			Err:     estimateResp.err,
		})
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
			resp := s(db, searchCtx, q, limit, offset)
			log.Debug(ctx, searchEvent{
				Type:    resp.source,
				Latency: time.Since(start),
				Err:     resp.err,
			})
			if guardTestResult != nil {
				defer guardTestResult(resp.source)()
			}
			responses <- resp
		}()
	}
	// Note for future readers: in previous iterations of this code we kept
	// reading responses if the first one had an error, with the goal to minimize
	// error ratio. That didn't behave well if Postgres was overloaded.
	resp := <-responses
	if resp.err != nil {
		return nil, fmt.Errorf("%q search failed: %v", resp.source, resp.err)
	}
	if resp.uncounted {
		// Since the response is uncounted, we should wait for either the count
		// estimate to return, or for the first counted response.
	loop:
		for {
			select {
			case nextResp := <-responses:
				switch {
				case nextResp.err != nil:
					// There are alternatives here: we could continue waiting for the
					// estimate. But on the principle that errors are most likely to be
					// caused by Postgres overload, we exit early to cancel the estimate.
					return nil, fmt.Errorf("while waiting for count, got error from searcher %q: %v", nextResp.source, nextResp.err)
				case !nextResp.uncounted:
					log.Infof(ctx, "using counted search results from searcher %s", nextResp.source)
					// use this response since it is counted.
					resp = nextResp
					break loop
				}
			case estr := <-estimateChan:
				if estr.err != nil {
					return nil, fmt.Errorf("error getting estimated count: %v", estr.err)
				}
				log.Debug(ctx, "using count estimate")
				for _, r := range resp.results {
					// TODO: change the return signature of search to separate
					// result-level data from this query-level metadata.
					r.NumResults = estr.estimate
					r.Approximate = true
				}
				break loop
			case <-ctx.Done():
				return nil, fmt.Errorf("context deadline exceeded while waiting for estimated result count")
			}
		}
	}
	// cancel proactively here: we've got the search result we need.
	cancel()
	// latency is only recorded for valid search results, as fast failures could
	// skew the latency distribution.
	// Note that this latency measurement might differ meaningfully from the
	// resp.Latency, if time was spent waiting for the result count estimate.
	latency := float64(time.Since(searchStart)) / float64(time.Millisecond)
	stats.RecordWithTags(ctx,
		[]tag.Mutator{tag.Upsert(keySearchSource, resp.source)},
		searchLatency.M(latency))
	// To avoid fighting with the query planner, our searches only hit the
	// search_documents table and we enrich after getting the results. In the
	// future, we may want to fully denormalize and put all search data in the
	// search_documents table.
	if err := db.addPackageDataToSearchResults(ctx, resp.results); err != nil {
		return nil, err
	}
	return &resp, nil
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
					%[2]s *
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
	) d`, hllRegisterCount, scoreExpr)

type estimateResponse struct {
	estimate uint64
	err      error
}

// EstimateResultsCount uses the hyperloglog algorithm to estimate the number
// of results for the given search term.
func (db *DB) estimateResultsCount(ctx context.Context, q string) estimateResponse {
	row := db.db.QueryRow(ctx, hllQuery, q)
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
	query := fmt.Sprintf(`
		SELECT *, COUNT(*) OVER() AS total
		FROM (
			SELECT
				package_path,
				version,
				module_path,
				commit_time,
				imported_by_count,
				(%s) AS score
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
		OFFSET $3`, scoreExpr)
	var results []*internal.SearchResult
	collect := func(rows *sql.Rows) error {
		var r internal.SearchResult
		if err := rows.Scan(&r.PackagePath, &r.Version, &r.ModulePath, &r.CommitTime,
			&r.NumImportedBy, &r.Score, &r.NumResults); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		results = append(results, &r)
		return nil
	}
	err := db.db.RunQuery(ctx, query, collect, q, limit, offset)
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
		FROM popular_search($1, $2, $3, $4, $5)`
	var results []*internal.SearchResult
	collect := func(rows *sql.Rows) error {
		var r internal.SearchResult
		if err := rows.Scan(&r.PackagePath, &r.Version, &r.ModulePath, &r.CommitTime,
			&r.NumImportedBy, &r.Score); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		results = append(results, &r)
		return nil
	}
	err := db.db.RunQuery(ctx, query, collect, searchQuery, limit, offset, nonRedistributablePenalty, noGoModPenalty)
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

// addPackageDataToSearchResults adds package information to SearchResults that is not stored
// in the search_documents table.
func (db *DB) addPackageDataToSearchResults(ctx context.Context, results []*internal.SearchResult) (err error) {
	defer derrors.Wrap(&err, "DB.enrichResults(results)")
	if len(results) == 0 {
		return nil
	}
	var (
		keys []string
		// resultMap tracks PackagePath->SearchResult, to allow joining with the
		// returned package data.
		resultMap = make(map[string]*internal.SearchResult)
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
			license_types,
			redistributable
		FROM
			packages
		WHERE
			(path, version, module_path) IN (%s)`, strings.Join(keys, ","))
	collect := func(rows *sql.Rows) error {
		var (
			path, name, synopsis string
			licenseTypes         []string
			redist               bool
		)
		if err := rows.Scan(&path, &name, &synopsis, pq.Array(&licenseTypes), &redist); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		r, ok := resultMap[path]
		if !ok {
			return fmt.Errorf("BUG: unexpected package path: %q", path)
		}
		r.Name = name
		if redist || db.bypassLicenseCheck {
			r.Synopsis = synopsis
		}
		for _, l := range licenseTypes {
			if l != "" {
				r.Licenses = append(r.Licenses, l)
			}
		}
		return nil
	}
	return db.db.RunQuery(ctx, query, collect)
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
		has_go_mod,
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
		m.commit_time,
		m.has_go_mod,
		(
			SETWEIGHT(TO_TSVECTOR('path_tokens', $2), 'A') ||
			SETWEIGHT(TO_TSVECTOR($3), 'B') ||
			SETWEIGHT(TO_TSVECTOR($4), 'C') ||
			SETWEIGHT(TO_TSVECTOR($5), 'D')
		),
		hll_hash(p.path) & (%[1]d - 1),
		hll_zeros(hll_hash(p.path))
	FROM
		packages p
	INNER JOIN
		modules m

	ON
		p.module_path = m.module_path
		AND p.version = m.version
	WHERE
		p.path = $1
	%s
	LIMIT 1
	ON CONFLICT (package_path)
	DO UPDATE SET
		package_path=excluded.package_path,
		version=excluded.version,
		module_path=excluded.module_path,
		name=excluded.name,
		synopsis=excluded.synopsis,
		license_types=excluded.license_types,
		redistributable=excluded.redistributable,
		commit_time=excluded.commit_time,
		has_go_mod=excluded.has_go_mod,
		tsv_search_tokens=excluded.tsv_search_tokens,
		-- the hll fields are functions of path, so they don't change
		version_updated_at=(
			CASE WHEN excluded.version = search_documents.version
			THEN search_documents.version_updated_at
			ELSE CURRENT_TIMESTAMP
			END)
	;`, hllRegisterCount, orderByLatest)

// UpsertSearchDocuments adds search information for mod ot the search_documents table.
func UpsertSearchDocuments(ctx context.Context, db *database.DB, mod *internal.Module) (err error) {
	defer derrors.Wrap(&err, "UpsertSearchDocuments(ctx, %q)", mod.ModulePath)
	ctx, span := trace.StartSpan(ctx, "UpsertSearchDocuments")
	defer span.End()
	for _, pkg := range mod.LegacyPackages {
		if isInternalPackage(pkg.Path) {
			continue
		}
		// TODO(golang/go#40971): don't store synopsis unless the package is redistributable or
		// we are bypassing license checks.
		err := UpsertSearchDocument(ctx, db, upsertSearchDocumentArgs{
			PackagePath:    pkg.Path,
			ModulePath:     mod.ModulePath,
			Synopsis:       pkg.Synopsis,
			ReadmeFilePath: mod.LegacyReadmeFilePath,
			ReadmeContents: mod.LegacyReadmeContents,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type upsertSearchDocumentArgs struct {
	PackagePath    string
	ModulePath     string
	Synopsis       string
	ReadmeFilePath string
	ReadmeContents string
}

// UpsertSearchDocument inserts a row for each package in the module, if that
// package is the latest version and is not internal.
//
// The given module should have already been validated via a call to
// validateModule.
func UpsertSearchDocument(ctx context.Context, db *database.DB, args upsertSearchDocumentArgs) (err error) {
	defer derrors.Wrap(&err, "UpsertSearchDocument(ctx, db, %q, %q)", args.PackagePath, args.ModulePath)

	// Only summarize the README if the package and module have the same path.
	if args.PackagePath != args.ModulePath {
		args.ReadmeFilePath = ""
		args.ReadmeContents = ""
	}
	pathTokens := strings.Join(GeneratePathTokens(args.PackagePath), " ")
	sectionB, sectionC, sectionD := SearchDocumentSections(args.Synopsis, args.ReadmeFilePath, args.ReadmeContents)
	_, err = db.Exec(ctx, upsertSearchStatement, args.PackagePath, pathTokens, sectionB, sectionC, sectionD)
	return err
}

// GetPackagesForSearchDocumentUpsert fetches search information for packages in search_documents
// whose update time is before the given time.
func (db *DB) GetPackagesForSearchDocumentUpsert(ctx context.Context, before time.Time, limit int) (argsList []upsertSearchDocumentArgs, err error) {
	defer derrors.Wrap(&err, "GetPackagesForSearchDocumentUpsert(ctx, %s, %d)", before, limit)

	query := `
		SELECT sd.package_path, sd.module_path, sd.synopsis, m.readme_file_path, m.readme_contents
		FROM search_documents sd
		INNER JOIN modules m
		USING (module_path, version)
		WHERE sd.updated_at < $1
		LIMIT $2`

	collect := func(rows *sql.Rows) error {
		var a upsertSearchDocumentArgs
		if err := rows.Scan(&a.PackagePath, &a.ModulePath, &a.Synopsis, &a.ReadmeFilePath, &a.ReadmeContents); err != nil {
			return err
		}
		argsList = append(argsList, a)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, before, limit); err != nil {
		return nil, err
	}
	return argsList, nil
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

	searchPackages, err := db.getSearchPackages(ctx)
	if err != nil {
		return 0, err
	}
	counts, err := db.computeImportedByCounts(ctx, searchPackages)
	if err != nil {
		return 0, err
	}
	err = db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
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

// getSearchPackages returns the set of package paths that are in the search_documents table.
func (db *DB) getSearchPackages(ctx context.Context) (set map[string]bool, err error) {
	defer derrors.Wrap(&err, "DB.getSearchPackages(ctx)")

	set = map[string]bool{}
	err = db.db.RunQuery(ctx, `SELECT package_path FROM search_documents`, func(rows *sql.Rows) error {
		var p string
		if err := rows.Scan(&p); err != nil {
			return err
		}
		set[p] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return set, nil
}

func (db *DB) computeImportedByCounts(ctx context.Context, searchDocsPackages map[string]bool) (counts map[string]int, err error) {
	defer derrors.Wrap(&err, "db.computeImportedByCounts(ctx)")

	counts = map[string]int{}
	// Get all (from_path, to_path) pairs, deduped.
	// Also get the from_path's module path.
	rows, err := db.db.Query(ctx, `
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
		// Don't count an importer if it's not in search_documents.
		if !searchDocsPackages[from] {
			continue
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

func insertImportedByCounts(ctx context.Context, db *database.DB, counts map[string]int) (err error) {
	defer derrors.Wrap(&err, "insertImportedByCounts(ctx, db, counts)")

	const createTableQuery = `
		CREATE TEMPORARY TABLE computed_imported_by_counts (
			package_path      TEXT NOT NULL,
			imported_by_count INTEGER DEFAULT 0 NOT NULL
		) ON COMMIT DROP;
    `
	if _, err := db.Exec(ctx, createTableQuery); err != nil {
		return fmt.Errorf("CREATE TABLE: %v", err)
	}
	values := make([]interface{}, 0, 2*len(counts))
	for p, c := range counts {
		values = append(values, p, c)
	}
	columns := []string{"package_path", "imported_by_count"}
	return db.BulkInsert(ctx, "computed_imported_by_counts", columns, values, "")
}

func compareImportedByCounts(ctx context.Context, db *database.DB) (err error) {
	defer derrors.Wrap(&err, "compareImportedByCounts(ctx, tx)")

	query := `
		SELECT
			s.package_path,
			s.imported_by_count,
			c.imported_by_count
		FROM
			search_documents s
		INNER JOIN
			computed_imported_by_counts c
		ON
			s.package_path = c.package_path
	`
	// Compute some info about the changes to import-by counts.
	const changeThreshold = 0.05 // count how many counts change by at least this fraction
	var total, zero, change, diff int
	err = db.RunQuery(ctx, query, func(rows *sql.Rows) error {
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
			return nil
		}
		fracDiff := math.Abs(float64(new-old)) / float64(old)
		if fracDiff > changeThreshold {
			diff++
		}
		return nil
	})
	if err != nil {
		return err
	}
	log.Infof(ctx, "%6d total rows in search_documents match computed_imported_by_counts", total)
	log.Infof(ctx, "%6d will change", change)
	log.Infof(ctx, "%6d currently have a zero imported-by count", zero)
	log.Infof(ctx, "%6d of the non-zero rows will change by more than %d%%", diff, int(changeThreshold*100))
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
func updateImportedByCounts(ctx context.Context, db *database.DB) (int64, error) {
	const updateStmt = `
		UPDATE search_documents s
		SET
			imported_by_count = c.imported_by_count,
			imported_by_count_updated_at = CURRENT_TIMESTAMP
		FROM computed_imported_by_counts c
		WHERE s.package_path = c.package_path;`

	res, err := db.Exec(ctx, updateStmt)
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

// GeneratePathTokens returns the subPaths and path token parts that will be
// indexed for search, which includes (1) the packagePath (2) all sub-paths of
// the packagePath (3) all parts for a path element that is delimited by a dash
// and (4) all parts of a path element that is delimited by a dot, except for
// the last element.
func GeneratePathTokens(packagePath string) []string {
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
	sort.Strings(subPaths)
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

// DeleteOlderVersionFromSearchDocuments deletes from search_documents every package with
// the given module path whose version is older than the given version.
// It is used when fetching a module with an alternative path. See internal/worker/fetch.go:fetchAndUpdateState.
func (db *DB) DeleteOlderVersionFromSearchDocuments(ctx context.Context, modulePath, version string) (err error) {
	defer derrors.Wrap(&err, "DeleteOlderVersionFromSearchDocuments(ctx, %q, %q)", modulePath, version)

	return db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
		// Collect all package paths in search_documents with the given module path
		// and an older version. (package_path is the primary key of search_documents.)
		var ppaths []string
		query := `
			SELECT package_path, version
			FROM search_documents
			WHERE module_path = $1
		`
		err := tx.RunQuery(ctx, query, func(rows *sql.Rows) error {
			var ppath, v string
			if err := rows.Scan(&ppath, &v); err != nil {
				return err
			}
			if semver.Compare(v, version) < 0 {
				ppaths = append(ppaths, ppath)
			}
			return nil
		}, modulePath)
		if err != nil {
			return err
		}
		if len(ppaths) == 0 {
			return nil
		}

		// Delete all of those paths.
		q := fmt.Sprintf(`DELETE FROM search_documents WHERE package_path IN ('%s')`, strings.Join(ppaths, `', '`))
		res, err := tx.Exec(ctx, q)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("RowsAffected: %v", err)
		}
		log.Infof(ctx, "deleted %d rows from search_documents", n)
		return nil
	})
}
