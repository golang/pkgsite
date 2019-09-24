// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
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
		Name:        "custom.googleapis.com/go-discovery/search/latency",
		Measure:     SearchLatency,
		Aggregation: ochttp.DefaultLatencyDistribution,
		Description: "Search latency, by result source query type.",
		TagKeys:     []tag.Key{SearchSource},
	}
	// SearchResponseCount counts search responses by search query type.
	SearchResponseCount = &view.View{
		Name:        "custom.googleapis.com/go-discovery/search/count",
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
}

// A searcher is used to execute a single search request.
type searcher func(ctx context.Context, q string, limit, offset int) searchResponse

// FastSearch executes three search requests concurrently:
//   - very popular packages (imported_by_count > 50); the top ~1%
//   - popular packages (imported_by_count > 8); the top ~5%
//   - all packages ("deep" search)
// Popular search takes significantly less time to scan very common search
// terms (e.g. "errors", "cloud", or "kubernetes"), due to partial indexing.
// Because 0 <= ts_rank() <= 1, we know that the highest score of any unpopular
// package is ln(e+N), where N is the number of importers above which a package
// is 'popular'. Therefore if the lowest scoring result of popular search is
// greater than ln(e+N), we know that we haven't missed any results and can
// return the search result immediately, cancelling other searches.
//
// On the other hand, if we *have* missed a result it is likely that the search
// term is infrequent, and deep scan will be fast due to our inverted gin index
// on search tokens.
//
// The gap in this optimization is search terms that are very frequent, but
// rarely relevant: "int" or "package", for example. In these cases we'll pay
// the penalty of a deep search that scans nearly every package.
func (db *DB) FastSearch(ctx context.Context, q string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.FastSearch(ctx, %q, %d, %d)", q, limit, offset)
	return db.guardedFastSearch(ctx, q, limit, offset, nil)
}

// guardedFastSearch is factored out for testing purposes: it accepts an
// optional func to allow tests to control the order in which search results
// are returned.
func (db *DB) guardedFastSearch(ctx context.Context, q string, limit, offset int, guardTestResult func(string) func()) ([]*SearchResult, error) {
	// We fan out multiple search requests, searching very popular, popular, and
	// all search documents. The thresholds for popularity were chosen as a
	// trade-off between hit rate and latency: the more selective the subset of
	// documents, the lower the latency but also the lower the hit rate.
	//
	// The heuristic for choosing these thresholds is pinned to a target latency
	// of ~500ms.  The 'very popular' index was chosen so that extremely dense
	// and relevant search terms (e.g. 'kubernetes' or 'github') are returned
	// within this threshold. The 'popular' index was chosen as a backstop for
	// terms that miss the 'very popular' result set.
	searchers := []searcher{db.popularSearcher(50), db.popularSearcher(8), db.deepSearch}
	responses := make(chan searchResponse, len(searchers))
	// cancel all unfinished searches when a result (or error) is returned. The
	// effectiveness of this depends on the database driver.
	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

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
	// cancel proactively here: we've got the search result we need.
	cancel()
	if resp.err != nil {
		return nil, resp.err
	}
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

// deepSearch searches all packages for the query. It is slower, but results
// are always valid.
func (db *DB) deepSearch(ctx context.Context, q string, limit, offset int) searchResponse {
	query := `
		SELECT *
		FROM (
			SELECT
				package_path,
				version,
				module_path,
				commit_time,
				imported_by_count,
				COUNT(*) OVER() AS total,
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
			&r.NumImportedBy, &r.NumResults, &r.Score); err != nil {
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

// popularSearcher returns a searcher that only searches packages with more
// than cutoff importers. Results can be invalid if it does not return the
// limit of results, all of which have greater score than the highest
// theoretical score of an unpopular package.
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
			source:  fmt.Sprintf("popular-%d", cutoff),
			results: results,
			err:     err,
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

// Search fetches packages from the database that match the terms
// provided, and returns them in order of relevance.
func (db *DB) Search(ctx context.Context, searchQuery string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "DB.Search(ctx, %q, %d, %d)", searchQuery, limit, offset)

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
	if err := db.runQuery(ctx, query, collect, searchQuery, limit, offset); err != nil {
		return nil, err
	}
	return results, nil
}

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
	_, err = db.exec(ctx, `
		INSERT INTO search_documents (
			package_path,
			version,
			module_path,
			version_updated_at,
			redistributable,
			commit_time,
			tsv_search_tokens
		)
		SELECT
			p.path,
			p.version,
			p.module_path,
			CURRENT_TIMESTAMP,
			p.redistributable,
			v.commit_time,
			(
				SETWEIGHT(TO_TSVECTOR($2), 'A') ||
				SETWEIGHT(TO_TSVECTOR(p.synopsis), 'B') ||
				SETWEIGHT(TO_TSVECTOR(v.readme_contents), 'C')
			)
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
			v.prerelease DESC
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
		;`, path, pathTokens)
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
// imported_by_count_updated_at for packages where:
//
// (1) The package is imported by a package in search_documents, whose
// imported_by_count_updated_at < version_updated_at. For example, if package B
// imports package A, and in search_documents B's imported_by_count_updated_at
// < version_updated_at, imported_by_count and imported_by_count_updated_at for
// A will be updated.
// (2) Packages where imported_by_count_updated_at < version_updated_at. That
// way, we won't keep updating B's importers (i.e. A), if B is never imported
// by anything.
//
// Note: we assume that clock drift is not an issue.
func (db *DB) UpdateSearchDocumentsImportedByCount(ctx context.Context, limit int) error {
	query := `
		WITH modified_packages AS (
			SELECT
				p.path AS package_path,
				v.updated_at
			FROM packages p
			INNER JOIN versions v
			ON p.module_path=v.module_path
			AND p.version=v.version
			WHERE v.updated_at > (
				SELECT COALESCE(MAX(imported_by_count_updated_at), TO_TIMESTAMP(0))
				FROM search_documents
			)
			LIMIT $1
		)
		UPDATE search_documents
		SET
			imported_by_count = n.imported_by_count,
			-- Note: we assume that max(updated_at) is only
			-- computed once for all rows updated.
			imported_by_count_updated_at = (SELECT MAX(updated_at) FROM modified_packages)
		FROM (
			SELECT
				p.package_path,
				COUNT(DISTINCT(i.from_path)) AS imported_by_count
			FROM (
				SELECT package_path
				FROM modified_packages
				UNION (
					SELECT i.to_path
					FROM imports_unique i
					INNER JOIN modified_packages m
					ON i.from_path = m.package_path
				)
			) p
			LEFT JOIN imports_unique i
			ON p.package_path = i.to_path
			GROUP BY p.package_path
		) n
		WHERE search_documents.package_path = n.package_path;`
	if _, err := db.exec(ctx, query, limit); err != nil {
		return fmt.Errorf("error updating imported_by_count and imported_by_count_updated_at for search documents: %v", err)
	}
	return nil
}

// LegacySearch fetches packages from the database that match the terms
// provided, and returns them in order of relevance as a []*SearchResult.
func (db *DB) LegacySearch(ctx context.Context, searchQuery string, limit, offset int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "LegacySearch(ctx, %q, %d, %d)", searchQuery, limit, offset)

	if limit == 0 {
		return nil, xerrors.Errorf("cannot search: limit cannot be 0: %w", derrors.InvalidArgument)
	}

	query := `
		WITH results AS (
			SELECT
				package_path,
				version,
				module_path,
				name,
				synopsis,
				license_types,
				commit_time,
				num_imported_by,
				CASE WHEN COALESCE(cardinality(license_types), 0) = 0
				  -- If the package does not have any license
				  -- files, lower its score by 50% since it will not be
				  -- redistributable.
				  -- TODO(b/136283982): improve how this signal
				  -- is used in search scoring
				  THEN (ts_rank(tsv_search_tokens, websearch_to_tsquery($1))*
				  	ln(exp(1)+num_imported_by)*0.5)
				  ELSE (ts_rank(tsv_search_tokens, websearch_to_tsquery($1))*
				  	ln(exp(1)+num_imported_by))
				  END AS score
			FROM
				mvw_search_documents
			WHERE
				tsv_search_tokens @@ websearch_to_tsquery($1)
		)

		SELECT
			r.package_path,
			r.version,
			r.module_path,
			r.name,
			r.synopsis,
			r.license_types,
			r.commit_time,
			r.num_imported_by,
			r.score,
			COUNT(*) OVER() AS total
		FROM
			results r
		WHERE
			r.score > 0.1
		ORDER BY
			r.score DESC,
			commit_time DESC,
			package_path
		LIMIT $2
		OFFSET $3;`
	rows, err := db.query(ctx, query, searchQuery, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		path, version, modulePath, name, synopsis string
		licenseTypes                              []string
		commitTime                                time.Time
		numImportedBy, total                      uint64
		score                                     float64
		results                                   []*SearchResult
	)
	for rows.Next() {
		if err := rows.Scan(&path, &version, &modulePath, &name, &synopsis,
			pq.Array(&licenseTypes), &commitTime, &numImportedBy, &score, &total); err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}
		var lics []string
		for _, l := range licenseTypes {
			if l != "" {
				lics = append(lics, l)
			}
		}
		results = append(results, &SearchResult{
			Name:          name,
			PackagePath:   path,
			ModulePath:    modulePath,
			Version:       version,
			Synopsis:      synopsis,
			Licenses:      lics,
			CommitTime:    commitTime,
			Score:         score,
			NumImportedBy: numImportedBy,
			NumResults:    total,
		})
	}
	return results, nil
}

// RefreshSearchDocuments replaces the old contents ofthe mvw_search_documents
// and executes the backing query to provide new data. It does so without
// locking out concurrent selects on the materialized view.
func (db *DB) RefreshSearchDocuments(ctx context.Context) error {
	query := "REFRESH MATERIALIZED VIEW CONCURRENTLY mvw_search_documents;"
	if _, err := db.exec(ctx, query); err != nil {
		return xerrors.Errorf("DB.RefreshSearchDocuments(ctx): %w", err)
	}
	return nil
}

// legacyInsertDocuments inserts a row for each package in the version.
//
// The given version should have already been validated via a call to
// validateVersion.
//
// This function will be deprecated once the search_documents table has been
// backfilled.
func (db *DB) legacyInsertDocuments(ctx context.Context, version *internal.Version) error {
	return db.Transact(func(tx *sql.Tx) error {
		return prepareAndExec(tx, `INSERT INTO documents (
				package_path,
				package_suffix,
				module_path,
				series_path,
				version,
				tsv_search_tokens
			) VALUES(
				 $1,
				 $2,
				 $3,
				 $4,
				 $5,
				SETWEIGHT(TO_TSVECTOR($6), 'A') ||
				SETWEIGHT(TO_TSVECTOR($7), 'A') ||
				SETWEIGHT(TO_TSVECTOR($8), 'B') ||
				SETWEIGHT(TO_TSVECTOR($9), 'C')
			) ON CONFLICT DO NOTHING;`, func(stmt *sql.Stmt) error {
			for _, p := range version.Packages {
				if _, err := stmt.ExecContext(ctx, p.Path, p.V1Path, version.ModulePath, version.SeriesPath(), version.Version, p.Name, strings.Join(generatePathTokens(p.Path), " "), p.Synopsis, version.ReadmeContents); err != nil {
					return fmt.Errorf("error inserting document for package path %q: %v", p.Path, err)
				}
			}
			return nil
		})
	})
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
