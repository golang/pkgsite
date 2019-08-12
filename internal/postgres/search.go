// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/xerrors"
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
	// Rank is used to sort items in an array of SearchResult.
	Rank float64

	// NumImportedBy is the number of packages that import Package.
	NumImportedBy uint64

	// NumResults is the total number of packages that were returned for this search.
	NumResults uint64
}

// Search fetches packages from the database that match the terms
// provided, and returns them in order of relevance.
func (db *DB) Search(ctx context.Context, searchQuery string, limit, offset int) ([]*SearchResult, error) {
	query := `
		WITH results AS (
			SELECT
				package_path,
				version,
				module_path,
				imported_by_count,
				commit_time,
				-- Rank packages based on their relevance and
				-- imported_by_count.
				-- If the package is not redistributable,
				-- lower its rank by 50% since a lot of details
				-- cannot be displayed.
				-- TODO(b/136283982): improve how this signal
				-- is used in search ranking.
				-- The log factor contains exp(1) (which is e) so that
				-- it is always >= 1. Taking the log of imported_by_count
				-- instead of using it directly makes the effect less dramatic:
				-- being 2x as popular only has an additive effect.
				ts_rank(tsv_search_tokens, websearch_to_tsquery($1)) *
					log(exp(1)+imported_by_count) *
					CASE WHEN redistributable THEN 1 ELSE 0.5 END
					AS rank
			FROM
				search_documents
			WHERE
				tsv_search_tokens @@ websearch_to_tsquery($1)
		)

		SELECT
			r.package_path,
			r.version,
			r.module_path,
			p.name,
			p.synopsis,
			p.license_types,
			r.commit_time,
			r.imported_by_count,
			r.rank,
			COUNT(*) OVER() AS total
		FROM
			results r
		INNER JOIN
			packages p
		ON
			p.path = r.package_path
			AND p.module_path = r.module_path
			AND p.version = r.version
		WHERE
			-- Only include results whose rank exceed a certain threshold.
			-- Based on experimentation, we picked a rank of greater than 0.1,
			-- but this may change based on future experimentation.
			r.rank > 0.1
		ORDER BY
			r.rank DESC,
			r.commit_time DESC,
			p.path
		LIMIT $2
		OFFSET $3;`
	rows, err := db.query(ctx, query, searchQuery, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("db.query(ctx, %s, %q, %d, %d): %v", query, searchQuery, limit, offset, err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var (
			sr           SearchResult
			licenseTypes []string
		)
		if err := rows.Scan(&sr.PackagePath, &sr.Version, &sr.ModulePath, &sr.Name, &sr.Synopsis,
			pq.Array(&licenseTypes), &sr.CommitTime, &sr.NumImportedBy, &sr.Rank, &sr.NumResults); err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}
		for _, l := range licenseTypes {
			if l != "" {
				sr.Licenses = append(sr.Licenses, l)
			}
		}
		results = append(results, &sr)
	}
	return results, nil
}

// UpsertSearchDocument inserts a row for each package in the version, if that
// package is the latest version.
//
// The given version should have already been validated via a call to
// validateVersion.
func (db *DB) UpsertSearchDocument(ctx context.Context, path string) error {
	if strings.Contains(path, "internal") {
		return xerrors.Errorf("cannot insert internal package %q into search documents: %w", path, derrors.InvalidArgument)
	}

	pathTokens := strings.Join(generatePathTokens(path), " ")
	_, err := db.exec(ctx, `
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
	if err != nil {
		return fmt.Errorf("db.exec(ctx, [query], %q, %q): %v", path, pathTokens, err)
	}
	return nil
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

// LegacySearch fetches packages from the database that match the terms
// provided, and returns them in order of relevance as a []*SearchResult.
func (db *DB) LegacySearch(ctx context.Context, searchQuery string, limit, offset int) ([]*SearchResult, error) {
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
				  -- files, lower its rank by 50% since it will not be
				  -- redistributable.
				  -- TODO(b/136283982): improve how this signal
				  -- is used in search ranking
				  THEN (ts_rank(tsv_search_tokens, websearch_to_tsquery($1))*
				  	log(exp(1)+num_imported_by)*0.5)
				  ELSE (ts_rank(tsv_search_tokens, websearch_to_tsquery($1))*
				  	log(exp(1)+num_imported_by))
				  END AS rank
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
			r.rank,
			COUNT(*) OVER() AS total
		FROM
			results r
		WHERE
			r.rank > 0.1
		ORDER BY
			r.rank DESC,
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
		rank                                      float64
		results                                   []*SearchResult
	)
	for rows.Next() {
		if err := rows.Scan(&path, &version, &modulePath, &name, &synopsis,
			pq.Array(&licenseTypes), &commitTime, &numImportedBy, &rank, &total); err != nil {
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
			Rank:          rank,
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
		return err
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
					return fmt.Errorf("error inserting document for package %+v: %v", p, err)
				}
			}
			return nil
		})
	})
}

// generatePathTokens returns the subPaths and path token parts that will be
// indexed for search, which includes (1) the packagePath (2) all sub-paths of
// the packagePath (3) all parts for a path element that is delimited by a dash
// and (4) all parts of a path element that is delimited by a dot, except for
// the last element.
func generatePathTokens(packagePath string) []string {
	packagePath = strings.Trim(packagePath, "/")

	subPathSet := make(map[string]bool)
	parts := strings.Split(packagePath, "/")
	for i := 0; i < len(parts); i++ {
		subPathSet[parts[i]] = true

		dotParts := strings.Split(parts[i], ".")
		if len(dotParts) > 1 {
			for _, p := range dotParts[:len(dotParts)-1] {
				subPathSet[p] = true
			}
		}

		dashParts := strings.Split(parts[i], "-")
		if len(dashParts) > 1 {
			for _, p := range dashParts {
				subPathSet[p] = true
			}
		}

		for j := i + 1; j <= len(parts); j++ {
			p := strings.Join(parts[i:j], "/")
			p = strings.Trim(p, "/")
			subPathSet[p] = true
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
