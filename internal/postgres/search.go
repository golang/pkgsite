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
	"golang.org/x/discovery/internal/license"
)

// RefreshSearchDocuments replaces the old contents ofthe mvw_search_documents
// and executes the backing query to provide new data. It does so without
// locking out concurrent selects on the materialized view.
func (db *DB) RefreshSearchDocuments(ctx context.Context) error {
	query := "REFRESH MATERIALIZED VIEW CONCURRENTLY mvw_search_documents;"
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("db.ExecContext(ctx, %q): %v", query, err)
	}
	return nil
}

// InsertDocuments inserts a row for each package in the version.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it was caused by an invalid version.
func (db *DB) InsertDocuments(ctx context.Context, version *internal.Version) error {
	if err := validateVersion(version); err != nil {
		return derrors.InvalidArgument(fmt.Sprintf("validateVersion(%+v): %v", version, err))
	}

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
				pathTokens := strings.Join([]string{p.Path, version.ModulePath, version.SeriesPath}, " ")
				if _, err := stmt.ExecContext(ctx, p.Path, p.Suffix, version.ModulePath, version.SeriesPath, version.Version, p.Name, pathTokens, p.Synopsis, version.ReadmeContents); err != nil {
					return fmt.Errorf("error inserting document for package %+v: %v", p, err)
				}
			}
			return nil
		})
	})
}

// SearchResult represents a single search result from SearchDocuments.
type SearchResult struct {
	// Rank is used to sort items in an array of SearchResult.
	Rank float64

	// NumImportedBy is the number of packages that import Package.
	NumImportedBy uint64

	// Package is the package data corresponding to this SearchResult.
	Package *internal.VersionedPackage

	// NumResults is the total number of packages that were returned for this search.
	NumResults uint64
}

// Search fetches packages from the database that match the terms
// provided, and returns them in order of relevance as a []*SearchResult.
func (db *DB) Search(ctx context.Context, terms []string, limit, offset int) ([]*SearchResult, error) {
	if limit == 0 {
		return nil, derrors.InvalidArgument(fmt.Sprintf("cannot search: limit cannot be 0"))
	}
	if len(terms) == 0 {
		return nil, derrors.InvalidArgument(fmt.Sprintf("cannot search: no terms"))
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
				(ts_rank(tsv_search_tokens, to_tsquery($1))*log(exp(1)+num_imported_by)) AS rank
			FROM
				mvw_search_documents
			WHERE
				tsv_search_tokens @@ to_tsquery($1)
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
			r.rank > POWER(10,-10)
		ORDER BY
			r.rank DESC, package_path
		LIMIT $2
		OFFSET $3;
	`
	rows, err := db.QueryContext(ctx, query, strings.Join(terms, " | "), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %s, %q, %d, %d): %v", query, terms, limit, offset, err)
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
		var licenses []*license.Metadata
		for _, t := range licenseTypes {
			licenses = append(licenses, &license.Metadata{Type: t})
		}
		results = append(results, &SearchResult{
			Rank:          rank,
			NumImportedBy: numImportedBy,
			NumResults:    total,
			Package: &internal.VersionedPackage{
				Package: internal.Package{
					Name:     name,
					Path:     path,
					Synopsis: synopsis,
					Licenses: licenses,
				},
				VersionInfo: internal.VersionInfo{
					ModulePath: modulePath,
					Version:    version,
					CommitTime: commitTime,
				},
			},
		})
	}
	return results, nil
}
