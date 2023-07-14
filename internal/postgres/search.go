// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/dcensus"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/postgres/search"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
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
	results []*SearchResult
	// err indicates a technical failure of the search query, or that results are
	// not provably complete.
	err error
}

// searchEvent is used to log structured information about search events for
// later analysis. A 'search event' occurs when a searcher or count estimate
// returns.
type searchEvent struct {
	// Type is either the searcher name or 'estimate' (the count estimate).
	Type string
	// Latency is the duration that the operation took.
	Latency time.Duration
	// Err is the error returned by the operation, if any.
	Err error
}

// A searcher is used to execute a single search request.
type searcher func(db *DB, ctx context.Context, q string, limit int, opts SearchOptions) searchResponse

// The pkgSearchers used by Search.
var pkgSearchers = map[string]searcher{
	"popular": (*DB).popularSearch,
	"deep":    (*DB).deepSearch,
}

var symbolSearchers = map[string]searcher{
	"symbol": (*DB).symbolSearch,
}

type SearchOptions = internal.SearchOptions
type SearchResult = internal.SearchResult

// SearchSupport implements the DataSource interface, supporting all search
// types.
func (db *DB) SearchSupport() internal.SearchSupport { return internal.FullSearch }

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
func (db *DB) Search(ctx context.Context, q string, opts SearchOptions) (_ []*SearchResult, err error) {
	defer derrors.WrapStack(&err, "DB.Search(ctx, %q, %+v)", q, opts)
	if !opts.SearchSymbols {
		const (
			limitMultiplier1 = 3
			limitMultiplier2 = 5
		)
		// Limit search to more rows than the requested number of results, so
		// that it can find other packages in the modules it selects.
		srs, err := db.search(ctx, q, opts, limitMultiplier1*opts.MaxResults)
		if err != nil {
			return nil, err
		}
		if len(srs) >= opts.MaxResults || numRows(srs) <= limitMultiplier1*opts.MaxResults {
			return srs, nil
		}
		// Grouped search didn't find enough results, but there are more
		// rows that could potentially match. Try one more time, with a
		// larger limit.
		return db.search(ctx, q, opts, limitMultiplier2*opts.MaxResults)
	}
	return db.search(ctx, q, opts, opts.MaxResults)
}

func (db *DB) search(ctx context.Context, q string, opts SearchOptions, limit int) (_ []*SearchResult, err error) {
	defer derrors.WrapStack(&err, "search(limit=%d)", limit)

	var searchers map[string]searcher
	if opts.SearchSymbols {
		searchers = symbolSearchers
	} else {
		searchers = pkgSearchers
	}
	resp, err := db.hedgedSearch(ctx, q, limit, opts, searchers, nil)
	if err != nil {
		return nil, err
	}
	// Filter out excluded paths.
	var results []*SearchResult
	for _, r := range resp.results {
		ex, err := db.IsExcluded(ctx, r.PackagePath)
		if err != nil {
			return nil, err
		}
		if !ex {
			results = append(results, r)
		}
	}
	if !opts.SearchSymbols {
		results = groupSearchResults(results)
	}
	if len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
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
//   - The Postgres ts_rank score, based the relevance of the document to the query.
//   - The log of the module's popularity, estimated by the number of importing packages.
//     The log factor contains exp(1) so that it is always >= 1. Taking the log
//     of imported_by_count instead of using it directly makes the effect less
//     dramatic: being 2x as popular only has an additive effect.
//   - A penalty factor for non-redistributable modules, since a lot of
//     details cannot be displayed.
//
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
func (db *DB) hedgedSearch(ctx context.Context, q string, limit int, opts SearchOptions, searchers map[string]searcher, guardTestResult func(string) func()) (_ *searchResponse, err error) {
	defer derrors.WrapStack(&err, "hedgedSearch(ctx, %q, %d, %+v)", q, limit, opts)

	searchStart := time.Now()
	responses := make(chan searchResponse, len(searchers))
	// cancel all unfinished searches when a result (or error) is returned. The
	// effectiveness of this depends on the database driver.
	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Fan out our search requests.
	for _, s := range searchers {
		s := s
		go func() {
			start := time.Now()
			resp := s(db, searchCtx, q, limit, opts)
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
	// cancel proactively here: we've got the search result we need.
	cancel()
	// latency is only recorded for valid search results, as fast failures could
	// skew the latency distribution.
	// Note that this latency measurement might differ meaningfully from the
	// resp.Latency, if time was spent waiting for the result count estimate.
	stats.RecordWithTags(ctx,
		[]tag.Mutator{tag.Upsert(keySearchSource, resp.source)},
		dcensus.MDur(searchLatency, time.Since(searchStart)))
	// To avoid fighting with the query planner, our searches only hit the
	// search_documents table and we enrich after getting the results. In the
	// future, we may want to fully denormalize and put all search data in the
	// search_documents table.
	if _, ok := searchers["symbol"]; !ok {
		if err := db.addPackageDataToSearchResults(ctx, resp.results); err != nil {
			return nil, err
		}
	}
	return &resp, nil
}

const hllRegisterCount = 128

// deepSearch searches all packages for the query. It is slower, but results
// are always valid.
func (db *DB) deepSearch(ctx context.Context, q string, limit int, opts SearchOptions) searchResponse {
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
	err := db.db.RunQuery(ctx, query, collect, q, limit, opts.Offset)
	if err != nil {
		results = nil
	}
	for i, r := range results {
		r.Offset = opts.Offset + i
	}
	if len(results) > 0 && results[0].NumResults > uint64(opts.MaxResultCount) {
		for _, r := range results {
			r.NumResults = uint64(opts.MaxResultCount)
		}
	}
	return searchResponse{
		source:  "deep",
		results: results,
		err:     err,
	}
}

func (db *DB) popularSearch(ctx context.Context, searchQuery string, limit int, opts SearchOptions) searchResponse {
	query := `
		SELECT
			package_path,
			version,
			module_path,
			commit_time,
			imported_by_count,
			score
		FROM popular_search($1, $2, $3, $4, $5)`
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
	err := db.db.RunQuery(ctx, query, collect, searchQuery, limit, opts.Offset, nonRedistributablePenalty, noGoModPenalty)
	if err != nil {
		results = nil
	}
	numResults := opts.MaxResultCount
	if opts.Offset+limit > opts.MaxResultCount || len(results) < limit {
		// It is practically impossible that len(results) < limit, because popular
		// search will never linearly scan everything before deep search completes,
		// but just to be slightly more theoretically correct, if our search
		// results are partial we know that we have exhausted all results.
		numResults = opts.Offset + len(results)
	}
	for i, r := range results {
		r.Offset = opts.Offset + i
		r.NumResults = uint64(numResults)
	}
	return searchResponse{
		source:  "popular",
		results: results,
		err:     err,
	}
}

// addPackageDataToSearchResults adds package information to SearchResults that is not stored
// in the search_documents table.
func (db *DB) addPackageDataToSearchResults(ctx context.Context, results []*SearchResult) (err error) {
	defer derrors.WrapStack(&err, "DB.addPackageDataToSearchResults(results)")
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
			p.path,
			u.name,
			d.synopsis,
			u.license_types,
			u.redistributable
		FROM
			units u
		INNER JOIN
			paths p
		ON u.path_id = p.id
		INNER JOIN
			modules m
		ON u.module_id = m.id
		LEFT JOIN
			documentation d
		ON u.id = d.unit_id
		WHERE
			(p.path, m.version, m.module_path) IN (%s)`, strings.Join(keys, ","))
	collect := func(rows *sql.Rows) error {
		var (
			path, name, synopsis string
			licenseTypes         []string
			redist               bool
		)
		if err := rows.Scan(&path, &name, database.NullIsEmpty(&synopsis), pq.Array(&licenseTypes), &redist); err != nil {
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
		r.Licenses = sortAndDedup(r.Licenses)
		return nil
	}
	return db.db.RunQuery(ctx, query, collect)
}

func sortAndDedup(s []string) []string {
	var r []string
	m := map[string]bool{}
	for _, x := range s {
		m[x] = true
	}
	for x := range m {
		r = append(r, x)
	}
	sort.Strings(r)
	return r
}

// groupSearchResults groups and re-orders the list of SearchResults by module
// and series path and returns a new list of SearchResults.
//
// The second and later packages from a module are grouped under the first package,
// and removed from the top-level list.
//
// Higher tagged major versions of a module replace lower ones.
func groupSearchResults(rs []*SearchResult) []*SearchResult {
	bestInGroup := map[string]*SearchResult{} // series path to result with max major version
	// Since rs is sorted by score, the first package we see for a series is the
	// highest-ranked one. However, we may prefer to show a lower-ranked one from a
	// module in the same series with a higher major version.
	for _, r := range rs {
		group, rMajor := groupAndMajorVersion(r)
		b := bestInGroup[group]
		if b == nil {
			// First result (package) with this key; remember it.
			bestInGroup[group] = r
			r.OtherMajor = map[string]int{}
		} else {
			_, bMajor := groupAndMajorVersion(b)
			switch {
			case !version.IsPseudo(r.Version) && (rMajor > bMajor || version.IsPseudo(b.Version)):
				// r is tagged, and is either in a higher major version, or the current best
				// is not tagged. Either way, prefer r to b.
				bestInGroup[group] = r
				r.OtherMajor = b.OtherMajor
				r.OtherMajor[b.ModulePath] = bMajor
				r.Score = b.Score // inherit the lower major version's higher score
			case rMajor == bMajor:
				// r is another package in b's group; remember it there.
				b.SameModule = append(b.SameModule, r)

			default:
				// A package from a different major version (either lower, or
				// higher untagged). Remember the major version.
				b.OtherMajor[r.ModulePath] = rMajor
			}
		}
	}
	// Collect new results and re-sort by score.
	var results []*SearchResult
	for _, r := range bestInGroup {
		if len(r.OtherMajor) == 0 {
			r.OtherMajor = nil
		}
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func groupAndMajorVersion(r *SearchResult) (string, int) {
	// Packages in the standard library are grouped by their top-level
	// directory, and we can consider them all part of the same major version.
	if r.ModulePath == stdlib.ModulePath {
		before, _, _ := strings.Cut(r.PackagePath, "/")
		return before, 1
	}
	series, major := internal.SeriesPathAndMajorVersion(r.ModulePath)
	if major == 1 && strings.HasPrefix(r.Version, "v0.") {
		major = 0
	}
	return series, major
}

// numRows counts the number of rows in a slice of SearchResults.
// Grouping will put some rows inside a SearchResult.
func numRows(rs []*SearchResult) int {
	n := 0
	for _, r := range rs {
		n += 1 + len(r.SameModule)
	}
	return n
}

var upsertSearchStatement = fmt.Sprintf(`
	INSERT INTO search_documents (
		package_path,
		package_path_id,
		version,
		module_path,
		module_path_id,
		unit_id,
		name,
		synopsis,
		license_types,
		redistributable,
		version_updated_at,
		commit_time,
		has_go_mod,
		-- TODO(https://golang.org/issue/44142): The path_tokens column is used
		-- to easily iterate on tsv_path_tokens, and can be removed once
		-- symbol search implementation is done.
		path_tokens,
		tsv_path_tokens,
		tsv_search_tokens,
		hll_register,
		hll_leading_zeros
	)
	SELECT
		p1.path,
		p1.id,
		m.version,
		m.module_path,
		p2.id,
		u.id AS unit_id,
		u.name,
		d.synopsis,
		u.license_types,
		u.redistributable,
		CURRENT_TIMESTAMP,
		m.commit_time,
		m.has_go_mod,
		$4,
		SETWEIGHT(TO_TSVECTOR('%s', replace($4, '_', '-')), 'A'),
		(
			SETWEIGHT(TO_TSVECTOR('path_tokens', $4), 'A') ||
			SETWEIGHT(TO_TSVECTOR($5), 'B') ||
			SETWEIGHT(TO_TSVECTOR($6), 'C') ||
			SETWEIGHT(TO_TSVECTOR($7), 'D')
		),
		hll_hash(p1.path) & (%d - 1),
		hll_zeros(hll_hash(p1.path))
	FROM units u
	INNER JOIN modules m ON u.module_id = m.id
	INNER JOIN paths p1 ON p1.id = u.path_id
	LEFT JOIN paths p2 ON p2.path = m.module_path
	LEFT JOIN documentation d ON u.id = d.unit_id
	WHERE
		p1.path = $1
		AND m.module_path = $2
		AND m.version = $3
	LIMIT 1 -- could be multiple build contexts
	ON CONFLICT (package_path_id)
	DO UPDATE SET
		package_path=excluded.package_path,
		version=excluded.version,
		module_path=excluded.module_path,
		unit_id=excluded.unit_id,
		name=excluded.name,
		synopsis=excluded.synopsis,
		license_types=excluded.license_types,
		redistributable=excluded.redistributable,
		commit_time=excluded.commit_time,
		has_go_mod=excluded.has_go_mod,
		path_tokens=excluded.path_tokens,
		tsv_path_tokens=excluded.tsv_path_tokens,
		tsv_search_tokens=excluded.tsv_search_tokens,
		-- the hll fields are functions of path, so they don't change
		version_updated_at=(
			CASE WHEN excluded.version = search_documents.version
			THEN search_documents.version_updated_at
			ELSE CURRENT_TIMESTAMP
			END)
	;`,
	search.SymbolTextSearchConfiguration,
	hllRegisterCount)

// upsertSearchDocuments adds search information for mod to the search_documents table.
// It assumes that all non-redistributable data has been removed from mod.
func upsertSearchDocuments(ctx context.Context, ddb *database.DB, mod *internal.Module) (err error) {
	defer derrors.WrapStack(&err, "upsertSearchDocuments(ctx, %q, %q)", mod.ModulePath, mod.Version)
	ctx, span := trace.StartSpan(ctx, "UpsertSearchDocuments")
	defer span.End()

	// Don't upsert a package if it is already present under a longer module
	// path. We need this because search_documents can have only one row per
	// import path, and we, like the go tool, prefer the package with the longer
	// module path. For example, if two packages have import path "a/b/c", one
	// in module "a" and the other in "a/b", we keep only the latter.
	lps, err := database.Collect1[string](ctx, ddb, `
		SELECT package_path
		FROM search_documents
		WHERE module_path LIKE $1 || '/%'
	`, mod.ModulePath)
	if err != nil {
		return err
	}
	longerPackages := map[string]bool{}
	for _, lp := range lps {
		longerPackages[lp] = true
	}

	for _, pkg := range mod.Packages() {
		if isInternalPackage(pkg.Path) {
			continue
		}
		if longerPackages[pkg.Path] {
			continue
		}
		args := UpsertSearchDocumentArgs{
			PackagePath: pkg.Path,
			ModulePath:  mod.ModulePath,
			Version:     mod.Version,
		}
		if len(pkg.Documentation) > 0 {
			// Use the synopsis of the first GOOS/GOARCH pair.
			args.Synopsis = pkg.Documentation[0].Synopsis
		}
		if pkg.Readme != nil {
			args.ReadmeFilePath = pkg.Readme.Filepath
			args.ReadmeContents = pkg.Readme.Contents
		}
		if err := UpsertSearchDocument(ctx, ddb, args); err != nil {
			return err
		}
	}
	return nil
}

type UpsertSearchDocumentArgs struct {
	PackagePath    string
	ModulePath     string
	Version        string
	Synopsis       string
	ReadmeFilePath string
	ReadmeContents string
}

// UpsertSearchDocument inserts a row in search_documents for the given package.
// The given module should have already been validated via a call to
// validateModule.
func UpsertSearchDocument(ctx context.Context, ddb *database.DB, args UpsertSearchDocumentArgs) (err error) {
	defer derrors.WrapStack(&err, "DB.UpsertSearchDocument(ctx, ddb, %q, %q)", args.PackagePath, args.ModulePath)

	// Only summarize the README if the package and module have the same path.
	// If this changes, fix DB.ReconcileSearch.
	if args.PackagePath != args.ModulePath {
		args.ReadmeFilePath = ""
		args.ReadmeContents = ""
	}
	pathTokens := strings.Join(GeneratePathTokens(args.PackagePath), " ")
	sectionB, sectionC, sectionD := SearchDocumentSections(args.Synopsis, args.ReadmeFilePath, args.ReadmeContents)
	_, err = ddb.Exec(ctx, upsertSearchStatement, args.PackagePath, args.ModulePath, args.Version, pathTokens, sectionB, sectionC, sectionD)
	return err
}

// GetPackagesForSearchDocumentUpsert fetches search information for packages in search_documents
// whose update time is before the given time.
func (db *DB) GetPackagesForSearchDocumentUpsert(ctx context.Context, before time.Time, limit int) (argsList []UpsertSearchDocumentArgs, err error) {
	defer derrors.WrapStack(&err, "GetPackagesForSearchDocumentUpsert(ctx, %s, %d)", before, limit)

	query := `
		SELECT
			sd.package_path,
			sd.module_path,
			sd.version,
			sd.synopsis,
			sd.redistributable,
			r.file_path,
			r.contents
		FROM modules m
		INNER JOIN units u
		ON m.id = u.module_id
		INNER JOIN paths p
		ON p.id = u.path_id
		LEFT JOIN readmes r
		ON u.id = r.unit_id
		INNER JOIN search_documents sd
		ON sd.package_path = p.path
		    AND sd.module_path = m.module_path
		    AND sd.version = m.version
		WHERE sd.updated_at < $1
		LIMIT $2`

	collect := func(rows *sql.Rows) error {
		var (
			a      UpsertSearchDocumentArgs
			redist bool
		)
		if err := rows.Scan(&a.PackagePath, &a.ModulePath, &a.Version, &a.Synopsis, &redist,
			database.NullIsEmpty(&a.ReadmeFilePath), database.NullIsEmpty(&a.ReadmeContents)); err != nil {
			return err
		}
		if !redist && !db.bypassLicenseCheck {
			a.Synopsis = ""
			a.ReadmeFilePath = ""
			a.ReadmeContents = ""
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
	defer derrors.WrapStack(&err, "UpdateSearchDocumentsImportedByCount(ctx)")

	curCounts, err := db.getSearchPackages(ctx)
	if err != nil {
		return 0, err
	}
	newCounts, err := db.computeImportedByCounts(ctx, curCounts)
	if err != nil {
		return 0, err
	}
	// Include only changed counts for packages that are in search_documents.
	changedCounts := map[string]int{}
	for p, nc := range newCounts {
		cc, present := curCounts[p]
		if present && cc != nc {
			changedCounts[p] = nc
		}
	}
	pct := 0
	if len(curCounts) > 0 {
		pct = len(changedCounts) * 100 / len(curCounts)
	}
	log.Debugf(ctx, "update-imported-by-counts: %d changed (%d%%)", len(changedCounts), pct)
	return db.UpdateSearchDocumentsImportedByCountWithCounts(ctx, changedCounts)
}

// How many imported-by counts to update at a time.
// A variable for testing.
var countBatchSize = 5_000

func (db *DB) UpdateSearchDocumentsImportedByCountWithCounts(ctx context.Context, counts map[string]int) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "UpdateSearchDocumentsImportedByCountWithCounts")
	for len(counts) > 0 {
		var nu int64
		err := db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
			if err := insertImportedByCounts(ctx, tx, counts, countBatchSize); err != nil {
				return err
			}
			nu, err = updateImportedByCounts(ctx, tx)
			return err
		})
		if err != nil {
			return nUpdated, err
		}
		nUpdated += nu
	}
	return nUpdated, nil
}

// getSearchPackages returns the set of package paths that are in the search_documents table,
// along with their current imported-by count.
func (db *DB) getSearchPackages(ctx context.Context) (counts map[string]int, err error) {
	defer derrors.WrapStack(&err, "DB.getSearchPackages(ctx)")

	counts = map[string]int{}
	err = db.db.RunQuery(ctx, `
		SELECT package_path, imported_by_count
		FROM search_documents
	`, func(rows *sql.Rows) error {
		var (
			p string
			c int
		)
		if err := rows.Scan(&p, &c); err != nil {
			return err
		}
		counts[p] = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return counts, nil
}

func (db *DB) computeImportedByCounts(ctx context.Context, curCounts map[string]int) (newCounts map[string]int, err error) {
	defer derrors.WrapStack(&err, "db.computeImportedByCounts(ctx)")

	newCounts = map[string]int{}
	// Get all (from_path, to_path) pairs, deduped.
	// Also get the from_path's module path.
	err = db.db.RunQuery(ctx, `
		SELECT DISTINCT from_path, from_module_path, to_path
		FROM imports_unique
	`, func(rows *sql.Rows) error {
		var from, fromMod, to string
		if err := rows.Scan(&from, &fromMod, &to); err != nil {
			return err
		}
		// Don't count an importer if it's not in search_documents.
		if _, ok := curCounts[from]; !ok {
			return nil
		}
		// Don't count an importer if it's in the same module as what it's importing.
		// Approximate that check by seeing if from_module_path is a prefix of to_path.
		// (In some cases, e.g. when to_path is in a nested module, that is not correct.)
		if (fromMod == stdlib.ModulePath && stdlib.Contains(to)) || strings.HasPrefix(to+"/", fromMod+"/") {
			return nil
		}
		newCounts[to]++
		return nil
	})
	if err != nil {
		return nil, err
	}
	return newCounts, nil
}

// insertImportedByCounts creates a temporary table and inserts at most limit
// rows into it, where each row is a key and value from the counts map. The
// inserted keys are deleted from counts.
func insertImportedByCounts(ctx context.Context, db *database.DB, counts map[string]int, limit int) (err error) {
	defer derrors.WrapStack(&err, "insertImportedByCounts(ctx, db, counts)")

	const createTableQuery = `
		CREATE TEMPORARY TABLE computed_imported_by_counts (
			package_path      TEXT NOT NULL,
			imported_by_count INTEGER NOT NULL
		) ON COMMIT DROP;
    `
	if _, err := db.Exec(ctx, createTableQuery); err != nil {
		return fmt.Errorf("CREATE TABLE: %v", err)
	}
	var values []any
	i := 0
	for p, c := range counts {
		if i >= limit {
			break
		}
		values = append(values, p, c)
		delete(counts, p)
		i++
	}
	columns := []string{"package_path", "imported_by_count"}
	return db.BulkInsert(ctx, "computed_imported_by_counts", columns, values, "")
}

// updateImportedByCounts updates the imported_by_count column in search_documents
// for every package in computed_imported_by_counts.
//
// Rows that don't change aren't updated.
//
// Note that if a package is never imported, its imported_by_count column will
// be the default (0) and its imported_by_count_updated_at column will never be set.
func updateImportedByCounts(ctx context.Context, db *database.DB) (int64, error) {
	// Lock the entire table to avoid deadlock. Without the lock, the update can
	// fail because module inserts are concurrently modifying rows of
	// search_documents.
	// See https://www.postgresql.org/docs/11/explicit-locking.html for what locks mean.
	// See https://www.postgresql.org/docs/11/sql-lock.html for the LOCK
	// statement, notably the paragraph beginning "If a transaction of this sort
	// is going to change the data...".
	const updateStmt = `
		LOCK TABLE search_documents IN SHARE ROW EXCLUSIVE MODE;
		UPDATE search_documents s
		SET
			imported_by_count = c.imported_by_count,
			imported_by_count_updated_at = CURRENT_TIMESTAMP
		FROM computed_imported_by_counts c
		WHERE s.package_path = c.package_path;`

	n, err := db.Exec(ctx, updateStmt)
	if err != nil {
		return 0, fmt.Errorf("error updating imported_by_count and imported_by_count_updated_at for search documents: %v", err)
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

// UpsertSearchDocumentWithImportedByCount is the same as UpsertSearchDocument,
// except it also updates the imported by count. This is only used for testing.
func (db *DB) UpsertSearchDocumentWithImportedByCount(ctx context.Context, args UpsertSearchDocumentArgs, importedByCount int) (err error) {
	defer derrors.WrapStack(&err, "DB.UpsertSearchDocumentWithImportedByCount(ctx, ddb, %q, %q)", args.PackagePath, args.ModulePath)

	if err := UpsertSearchDocument(ctx, db.db, args); err != nil {
		return err
	}
	_, err = db.db.Exec(ctx,
		`UPDATE search_documents SET imported_by_count=$1 WHERE package_path=$2;`,
		importedByCount, args.PackagePath)
	return err
}
