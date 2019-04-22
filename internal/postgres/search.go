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
)

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
				name_tokens,
				path_tokens,
				synopsis_tokens,
				readme_tokens
			) VALUES(
				 $1,
				 $2,
				 $3,
				 $4,
				 $5,
				SETWEIGHT(TO_TSVECTOR($6), 'A'),
				SETWEIGHT(TO_TSVECTOR($7), 'A'),
				SETWEIGHT(TO_TSVECTOR($8), 'B'),
				SETWEIGHT(TO_TSVECTOR($9), 'C')
			) ON CONFLICT DO NOTHING;`, func(stmt *sql.Stmt) error {
			for _, p := range version.Packages {
				pathTokens := strings.Join([]string{p.Path, version.ModulePath, version.SeriesPath}, " ")
				if _, err := stmt.ExecContext(ctx, p.Path, p.Suffix, version.ModulePath, version.SeriesPath, version.Version, p.Name, pathTokens, p.Synopsis, version.ReadMe); err != nil {
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

	// Total is the total number of packages that were returned for this search.
	Total uint64
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
	query := `WITH results AS (
			SELECT
				package_path,
				version,
				module_path,
				name,
				synopsis,
				license_types,
				license_paths,
				commit_time,
				num_imported_by,
				(
					ts_rank (
						name_tokens ||
						path_tokens ||
						synopsis_tokens ||
						readme_tokens, to_tsquery($1)
					) *  log(exp(1)+num_imported_by)
				) AS rank
			FROM
				vw_search_results
		)

		SELECT
			r.package_path,
			r.version,
			r.module_path,
			r.name,
			r.synopsis,
			r.license_types,
			r.license_paths,
			r.commit_time,
			r.num_imported_by,
			r.rank,
			COUNT(*) OVER() AS total
		FROM
			results r
		WHERE
			r.rank > POWER(10,-10)
		ORDER BY
			r.rank DESC
		LIMIT $2
		OFFSET $3;`
	rows, err := db.QueryContext(ctx, query, strings.Join(terms, " | "), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %s, %q, %d, %d): %v", query, terms, limit, offset, err)
	}
	defer rows.Close()

	var (
		path, version, modulePath, name, synopsis string
		licenseTypes, licensePaths                []string
		commitTime                                time.Time
		numImportedBy, total                      uint64
		rank                                      float64
		results                                   []*SearchResult
	)
	for rows.Next() {
		if err := rows.Scan(&path, &version, &modulePath, &name, &synopsis,
			pq.Array(&licenseTypes), pq.Array(&licensePaths), &commitTime, &numImportedBy, &rank, &total); err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}

		lics, err := zipLicenseInfo(licenseTypes, licensePaths)
		if err != nil {
			return nil, fmt.Errorf("zipLicenseInfo(%v, %v): %v", licenseTypes, licensePaths, err)
		}
		results = append(results, &SearchResult{
			Rank:          rank,
			NumImportedBy: numImportedBy,
			Total:         total,
			Package: &internal.VersionedPackage{
				Package: internal.Package{
					Name:     name,
					Path:     path,
					Synopsis: synopsis,
					Licenses: lics,
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
