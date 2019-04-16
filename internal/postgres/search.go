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
				pathTokens := strings.Join([]string{p.Path, version.Module.Path, version.Module.Series.Path}, " ")
				if _, err := stmt.ExecContext(ctx, p.Path, p.Suffix, version.Module.Path, version.Module.Series.Path, version.Version, p.Name, pathTokens, p.Synopsis, version.ReadMe); err != nil {
					return fmt.Errorf("error inserting document for package %+v: %v", p, err)
				}
			}
			return nil
		})
	})
}

// SearchResult represents a single search result from SearchDocuments.
type SearchResult struct {
	// Relevance is the ts_rank score for this package based on the terms used
	// for search.
	Relevance float64

	// NumImporters is the number of packages that import Package.
	NumImporters int64

	// Package is the package data corresponding to this SearchResult.
	Package *internal.Package
}

func calculateRank(relevance float64, imports int64) float64 {
	return relevance * math.Log(math.E+float64(imports))
}

// Search fetches packages from the database that match the terms
// provided, and returns them in order of relevance as a []*SearchResult.
func (db *DB) Search(ctx context.Context, terms []string) ([]*SearchResult, error) {
	if len(terms) == 0 {
		return nil, derrors.InvalidArgument(fmt.Sprintf("cannot search: no terms"))
	}
	query := `SELECT
			d.package_path,
			d.relevance,
			CASE WHEN
				i.importers IS NULL THEN 0
				ELSE i.importers
				END AS importers
		FROM (
			SELECT DISTINCT ON (package_path) package_path,
			ts_rank (
				name_tokens ||
				path_tokens ||
				synopsis_tokens ||
				readme_tokens, to_tsquery($1)
			) AS relevance
			FROM
				documents
			ORDER BY
				package_path, relevance DESC
		) d
		LEFT JOIN (
			SELECT imports.from_path, COUNT(*) AS importers
			FROM imports
			GROUP BY 1
		) i
		ON d.package_path = i.from_path;`
	rows, err := db.QueryContext(ctx, query, strings.Join(terms, " | "))
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q): %v", query, terms, err)
	}
	defer rows.Close()

	var (
		path      string
		rank      float64
		importers int64
		paths     []string
	)
	pathToResults := map[string]*SearchResult{}
	for rows.Next() {
		if err := rows.Scan(&path, &rank, &importers); err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}
		pathToResults[path] = &SearchResult{
			Relevance:    rank,
			NumImporters: importers,
		}
		paths = append(paths, path)
	}

	pkgs, err := db.GetLatestPackageForPaths(ctx, paths)
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		pathToResults[p.Path].Package = p
	}

	var results []*SearchResult
	for _, p := range pathToResults {
		// Filter out results that are not relevant to the terms in this search.
		if calculateRank(p.Relevance, p.NumImporters) > 0 {
			results = append(results, p)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return calculateRank(results[i].Relevance, results[i].NumImporters) > calculateRank(results[j].Relevance, results[j].NumImporters)
	})
	return results, nil
}

// GetLatestPackageForPaths returns a list of packages that have the latest version that
// corresponds to each path specified in the list of paths. The resulting list is
// sorted by package path lexicographically. So if multiple packages have the same
// path then the package whose module path comes first lexicographically will be
// returned.
func (db *DB) GetLatestPackageForPaths(ctx context.Context, paths []string) ([]*internal.Package, error) {
	var (
		packages                                  []*internal.Package
		commitTime, createdAt, updatedAt          time.Time
		path, modulePath, name, synopsis, version string
		licenseTypes, licensePaths                []string
	)

	query := `
		SELECT DISTINCT ON (p.path)
			p.path,
			p.module_path,
			v.version,
			v.commit_time,
			p.license_types,
			p.license_paths,
			p.name,
			p.synopsis
		FROM
			vw_licensed_packages p
		INNER JOIN
			versions v
		ON
			v.module_path = p.module_path
			AND v.version = p.version
		WHERE
			p.path = ANY($1)
		ORDER BY
			p.path,
			p.module_path,
			v.major DESC,
			v.minor DESC,
			v.patch DESC,
			v.prerelease DESC;`

	rows, err := db.QueryContext(ctx, query, pq.Array(paths))
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %v): %v", query, pq.Array(paths), err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&path, &modulePath, &version, &commitTime,
			pq.Array(&licenseTypes), pq.Array(&licensePaths), &name, &synopsis); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		lics, err := zipLicenseInfo(licenseTypes, licensePaths)
		if err != nil {
			return nil, fmt.Errorf("zipLicenseInfo(%v, %v): %v", licenseTypes, licensePaths, err)
		}
		packages = append(packages, &internal.Package{
			Name:     name,
			Path:     path,
			Synopsis: synopsis,
			Licenses: lics,
			Version: &internal.Version{
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				Module: &internal.Module{
					Path: modulePath,
				},
				Version:    version,
				CommitTime: commitTime,
			},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err() returned error %v", err)
	}

	return packages, nil
}
