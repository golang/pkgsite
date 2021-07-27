// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/postgres/symbolsearch"
	"golang.org/x/sync/errgroup"
)

func upsertSymbolSearchDocuments(ctx context.Context, tx *database.DB,
	modulePath, v string) (err error) {
	defer derrors.Wrap(&err, "upsertSymbolSearchDocuments(ctx, ddb, %q, %q)", modulePath, v)

	if !experiment.IsActive(ctx, internal.ExperimentInsertSymbolSearchDocuments) {
		return nil
	}

	// If a user is looking for the symbol "DB.Begin", from package
	// database/sql, we want them to be able to find this by searching for
	// "DB.Begin" and "sql.DB.Begin". Searching for "sql.DB", "DB", "Begin" or
	// "sql.DB" will not return "DB.Begin".
	// If a user is looking for the symbol "DB.Begin", from package
	// database/sql, we want them to be able to find this by searching for
	// "DB.Begin", "Begin", and "sql.DB.Begin". Searching for "sql.DB" or
	// "DB" will not return "DB.Begin".
	q := `
		INSERT INTO symbol_search_documents (
			package_path_id,
			symbol_name_id,
			unit_id,
			package_symbol_id,
			goos,
			goarch,
			package_name,
			package_path,
			imported_by_count
		)
		SELECT DISTINCT ON (sd.package_path_id, ps.symbol_name_id)
			sd.package_path_id,
			ps.symbol_name_id,
			sd.unit_id,
			ps.id AS package_symbol_id,
			d.goos,
			d.goarch,
			sd.name,
			sd.package_path,
			sd.imported_by_count
		FROM search_documents sd
		INNER JOIN units u ON sd.unit_id = u.id
		INNER JOIN documentation d ON d.unit_id = sd.unit_id
		INNER JOIN documentation_symbols ds ON d.id = ds.documentation_id
		INNER JOIN package_symbols ps ON ps.id = ds.package_symbol_id
		WHERE
			sd.module_path = $1 AND sd.version = $2
			AND u.name != 'main' -- do not insert data for commands
			AND sd.redistributable
		ORDER BY
			sd.package_path_id,
			ps.symbol_name_id,
			-- Order should match internal.BuildContexts.
			CASE WHEN d.goos = 'all' THEN 0
			WHEN d.goos = 'linux' THEN 1
			WHEN d.goos = 'windows' THEN 2
			WHEN d.goos = 'darwin' THEN 3
			WHEN d.goos = 'js' THEN 4
			END
		ON CONFLICT (package_path_id, symbol_name_id)
		DO UPDATE SET
			unit_id = excluded.unit_id,
			package_symbol_id = excluded.package_symbol_id,
			goos = excluded.goos,
			goarch = excluded.goarch,
			package_name = excluded.package_name,
			package_path = excluded.package_path,
			imported_by_count = excluded.imported_by_count;`
	_, err = tx.Exec(ctx, q, modulePath, v)
	return err
}

// symbolSearch searches all symbols in the symbol_search_documents table for
// the query.
//
// TODO(https://golang.org/issue/44142): factor out common code between
// symbolSearch and deepSearch.
func (db *DB) symbolSearch(ctx context.Context, q string, limit, offset, maxResultCount int) searchResponse {
	var (
		query   string
		err     error
		results []*SearchResult
	)
	sr := searchResponse{source: "symbol"}
	if strings.Count(q, " ") == 0 {
		switch strings.Count(q, ".") {
		case 0:
			// There is only 1 word in the search, and there are no dots, so
			// this must be the symbol name.
			query = symbolsearch.QuerySymbol
		case 1:
			// There is only 1 element, split by 1 dot, so the search must
			// either be for <package>.<symbol> or <type>.<methodOrFieldName>.
			results, err = runSymbolSearchOneDot(ctx, db.db, q, limit)
		case 2:
			// There is only 1 element, split by 2 dots, so the search must
			// be for <package>.<type>.<methodOrFieldName>.
			query = symbolsearch.QueryPackageDotSymbol
		default:
			// There is no situation where we will get results for oe element
			// containing more than 2 dots.
			//
			// In this case, return no results.
			return sr
		}
	} else {
		// The search query contains multiple words, separated by spaces.
		query = symbolsearch.QueryMultiWord
	}
	if query != "" {
		results, err = runSymbolSearch(ctx, db.db, query, q, limit)
	}
	if err != nil {
		sr.err = err
		return sr
	}
	for _, r := range results {
		r.NumResults = uint64(len(results))
	}
	sr.results = results
	return sr
}

// runSymbolSearchOneDot is used when q contains only 1 dot, so the search must
// either be for <package>.<symbol> or <type>.<methodOrFieldName>.
//
// This search is split into two parallel queries, since the query is very slow
// when using an OR in the WHERE clause.
func runSymbolSearchOneDot(ctx context.Context, ddb *database.DB, q string, limit int) (_ []*SearchResult, err error) {
	group, searchCtx := errgroup.WithContext(ctx)
	resultsArray := make([][]*SearchResult, 2)
	for i, query := range []string{
		symbolsearch.QuerySymbol,
		symbolsearch.QueryPackageDotSymbol,
	} {
		i := i
		query := query
		group.Go(func() error {
			results, err := runSymbolSearch(searchCtx, ddb, query, q, limit)
			if err != nil {
				return err
			}
			resultsArray[i] = results
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	results := append(resultsArray[0], resultsArray[1]...)
	sort.Slice(results, func(i, j int) bool { return results[i].NumImportedBy > results[j].NumImportedBy })
	if len(results) > limit {
		results = results[0:limit]
	}
	return results, nil
}

func runSymbolSearch(ctx context.Context, ddb *database.DB, query, q string, limit int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "runSymbolSearch(ctx, ddb, query, %q, %d)", q, limit)

	var results []*SearchResult
	collect := func(rows *sql.Rows) error {
		var r SearchResult
		if err := rows.Scan(
			&r.SymbolName,
			&r.PackagePath,
			&r.ModulePath,
			&r.Version,
			&r.Name,
			&r.Synopsis,
			pq.Array(&r.Licenses),
			&r.CommitTime,
			&r.NumImportedBy,
			&r.SymbolGOOS,
			&r.SymbolGOARCH,
			&r.SymbolKind,
			&r.SymbolSynopsis); err != nil {
			return fmt.Errorf("symbolSearch: rows.Scan(): %v", err)
		}
		results = append(results, &r)
		return nil
	}
	if err := ddb.RunQuery(ctx, query, collect, q, limit); err != nil {
		return nil, err
	}
	return results, nil
}
