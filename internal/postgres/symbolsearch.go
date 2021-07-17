// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/postgres/symbolsearch"
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
			goarch
		)
		SELECT DISTINCT ON (sd.package_path_id, ps.symbol_name_id)
			sd.package_path_id,
			ps.symbol_name_id,
			sd.unit_id,
			ps.id AS package_symbol_id,
			d.goos,
			d.goarch
		FROM search_documents sd
		INNER JOIN units u
			ON sd.unit_id = u.id
		INNER JOIN documentation d
			ON d.unit_id = sd.unit_id
		INNER JOIN documentation_symbols ds
			ON d.id = ds.documentation_id
		INNER JOIN package_symbols ps
			ON ps.id = ds.package_symbol_id
		WHERE
			sd.module_path = $1 AND sd.version = $2
			AND u.name != 'main' -- do not insert data for commands
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
			goarch = excluded.goarch;`
	_, err = tx.Exec(ctx, q, modulePath, v)
	return err
}

// symbolSearch searches all symbols in the symbol_search_documents table for
// the query.
//
// TODO(https://golang.org/issue/44142): factor out common code between
// symbolSearch and deepSearch.
func (db *DB) symbolSearch(ctx context.Context, q string, limit, offset, maxResultCount int) searchResponse {
	var results []*SearchResult
	collect := func(rows *sql.Rows) error {
		var r SearchResult
		if err := rows.Scan(
			&r.PackagePath,
			&r.ModulePath,
			&r.Version,
			&r.Name,
			&r.Synopsis,
			pq.Array(&r.Licenses),
			&r.CommitTime,
			&r.NumImportedBy,
			&r.SymbolName,
			&r.SymbolGOOS,
			&r.SymbolGOARCH,
			&r.SymbolKind,
			&r.SymbolSynopsis,
			&r.NumResults); err != nil {
			return fmt.Errorf("symbolSearch: rows.Scan(): %v", err)
		}
		results = append(results, &r)
		return nil
	}

	var (
		query string
		err   error
	)
	if len(strings.Fields(q)) == 1 {
		switch len(strings.Split(q, ".")) {
		case 1:
			// There is only 1 word in the search, and there are no dots, so
			// this must be the symbol name.
			query = symbolsearch.QuerySymbol
		case 2:
			// There is only 1 element, split by 1 dot, so the search must
			// either be for <package>.<symbol> or <type>.<methodOrFieldName>.
			query = symbolsearch.QueryOneDot
		case 3:
			// There is only 1 element, split by 2 dots, so the search must
			// be for <package>.<type>.<methodOrFieldName>.
			query = symbolsearch.QueryPackageDotSymbol
		}
	} else {
		// TODO: add additional queries based on q.
		err = fmt.Errorf("unsupported query structure: %q", q)
	}
	if err == nil {
		err = db.db.RunQuery(ctx, query, collect, q, limit, offset)
		if err != nil {
			results = nil
		}
		if len(results) > 0 && results[0].NumResults > uint64(maxResultCount) {
			for _, r := range results {
				r.NumResults = uint64(maxResultCount)
			}
		}
	}
	return searchResponse{
		source:  "symbol",
		results: results,
		err:     err,
	}
}
