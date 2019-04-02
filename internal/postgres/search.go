// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"

	"golang.org/x/discovery/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// InsertDocuments inserts a row for each package in the version.
func (db *DB) InsertDocuments(version *internal.Version) error {
	if err := validateVersion(version); err != nil {
		return status.Errorf(codes.InvalidArgument, fmt.Sprintf("validateVersion(%+v): %v", version, err))
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
				if _, err := stmt.Exec(p.Path, p.Suffix, version.Module.Path, version.Module.Series.Path, version.Version, p.Name, pathTokens, p.Synopsis, version.ReadMe); err != nil {
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
func (db *DB) Search(terms []string) ([]*SearchResult, error) {
	if len(terms) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("cannot search: no terms"))
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
	rows, err := db.Query(query, strings.Join(terms, " | "))
	if err != nil {
		return nil, fmt.Errorf("db.Query(%q, %q) returned error: %v", query, terms, err)
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
			return nil, fmt.Errorf("row.Scan(%q, %f, %d): %v", path, rank, importers, err)
		}
		pathToResults[path] = &SearchResult{
			Relevance:    rank,
			NumImporters: importers,
		}
		paths = append(paths, path)
	}

	pkgs, err := db.GetLatestPackageForPaths(paths)
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		pathToResults[p.Path].Package = p
	}

	var results []*SearchResult
	for _, p := range pathToResults {
		results = append(results, p)
	}

	sort.Slice(results, func(i, j int) bool {
		return calculateRank(results[i].Relevance, results[i].NumImporters) > calculateRank(results[j].Relevance, results[j].NumImporters)
	})
	return results, nil
}
