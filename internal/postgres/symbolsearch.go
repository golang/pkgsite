// Copyright 2021 The Go Authors. All rights reserved.
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

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/postgres/symbolsearch"
	"golang.org/x/sync/errgroup"
)

func upsertSymbolSearchDocuments(ctx context.Context, tx *database.DB,
	modulePath, v string) (err error) {
	defer derrors.Wrap(&err, "upsertSymbolSearchDocuments(ctx, ddb, %q, %q)", modulePath, v)
	defer middleware.ElapsedStat(ctx, "upsertSymbolSearchDocuments")()

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
			imported_by_count,
			symbol_name
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
			sd.imported_by_count,
			s.name
		FROM search_documents sd
		INNER JOIN units u ON sd.unit_id = u.id
		INNER JOIN documentation d ON d.unit_id = sd.unit_id
		INNER JOIN documentation_symbols ds ON d.id = ds.documentation_id
		INNER JOIN package_symbols ps ON ps.id = ds.package_symbol_id
		INNER JOIN symbol_names s ON s.id = ps.symbol_name_id
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
			imported_by_count = excluded.imported_by_count,
			symbol_name = excluded.symbol_name;`
	_, err = tx.Exec(ctx, q, modulePath, v)
	return err
}

// symbolSearch searches all symbols in the symbol_search_documents table for
// the query.
//
// TODO(https://golang.org/issue/44142): factor out common code between
// symbolSearch and deepSearch.
func (db *DB) symbolSearch(ctx context.Context, q string, limit, offset, maxResultCount int) searchResponse {
	defer middleware.ElapsedStat(ctx, "symbolSearch")()

	var (
		results []*SearchResult
		err     error
	)
	sr := searchResponse{source: "symbol"}
	it := symbolsearch.ParseInputType(q)
	switch it {
	case symbolsearch.InputTypeOneDot:
		results, err = runSymbolSearchOneDot(ctx, db.db, q, limit)
	case symbolsearch.InputTypeMultiWord:
		results, err = runSymbolSearchMultiWord(ctx, db.db, q, limit)
	case symbolsearch.InputTypeNoDot:
		results, err = runSymbolSearch(ctx, db.db, symbolsearch.SearchTypeSymbol, q, limit)
	case symbolsearch.InputTypeTwoDots:
		results, err = runSymbolSearch(ctx, db.db, symbolsearch.SearchTypePackageDotSymbol, q, limit, q)
	default:
		// There is no supported situation where we will get results for one
		// element containing more than 2 dots.
		return sr
	}

	if len(results) == 0 {
		if err != nil && !errors.Is(err, derrors.NotFound) {
			sr.err = err
		}
		return sr
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].NumImportedBy != results[j].NumImportedBy {
			return results[i].NumImportedBy > results[j].NumImportedBy
		}

		// If two packages have the same imported by count, return them in
		// alphabetical order by package path.
		if results[i].PackagePath != results[j].PackagePath {
			return results[i].PackagePath < results[j].PackagePath
		}

		// If one package has multiple matching symbols, return them by
		// alphabetical order of symbol name.
		return results[i].SymbolName < results[j].SymbolName
	})
	if len(results) > limit {
		results = results[0:limit]
	}
	for _, r := range results {
		r.NumResults = uint64(len(results))
	}
	sr.results = results
	return sr
}

// runSymbolSearchMultiWord executes a symbol search for SearchTypeMultiWord.
func runSymbolSearchMultiWord(ctx context.Context, ddb *database.DB, q string, limit int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "runSymbolSearchMultiWord(ctx, ddb, query, %q, %d)", q, limit)
	defer middleware.ElapsedStat(ctx, "runSymbolSearchMultiWord")()

	symbolToPathTokens := multiwordSearchCombinations(q)
	if len(symbolToPathTokens) == 0 {
		// There are no words in the query that could be a symbol name.
		return nil, derrors.NotFound
	}
	group, searchCtx := errgroup.WithContext(ctx)
	resultsArray := make([][]*SearchResult, len(symbolToPathTokens))
	count := 0
	for symbol, pathTokens := range symbolToPathTokens {
		symbol := symbol
		pathTokens := pathTokens
		i := count
		count += 1
		group.Go(func() error {
			st := symbolsearch.SearchTypeMultiWordExact
			if strings.Contains(q, "|") {
				st = symbolsearch.SearchTypeMultiWordOr
			}
			ids, err := fetchMatchingSymbolIDs(searchCtx, ddb, st, symbol)
			if err != nil {
				if !errors.Is(err, derrors.NotFound) {
					return err
				}
				return nil
			}
			r, err := fetchSymbolSearchResults(ctx, ddb, st, ids, limit, pathTokens)
			if err != nil {
				return err
			}
			resultsArray[i] = r
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return mergedResults(resultsArray, limit), nil
}

func mergedResults(resultsArray [][]*SearchResult, limit int) []*SearchResult {
	var results []*SearchResult
	deduped := map[string]bool{}
	for _, array := range resultsArray {
		for _, r := range array {
			key := fmt.Sprintf("%s@%s", r.PackagePath, r.SymbolName)
			if !deduped[key] {
				results = append(results, r)
				deduped[key] = true
			}
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].NumImportedBy > results[j].NumImportedBy })
	if len(results) > limit {
		results = results[0:limit]
	}
	return results
}

// multiwordSearchCombinations returns a map of symbol name to path_tokens to
// be used for possible search combinations.
//
// For each word, check if there is an invalid symbol character or if it
// matches a common hostname. If so, the search on tsv_path_tokens must match
// that search.
//
// It is assumed that the symbol name is always 1 word. For example, if the
// user wants sql.DB.Begin, "sql DB.Begin", "sql Begin", or "sql DB" will all
// be return the relevant result, but "sql DB Begin" will not.
func multiwordSearchCombinations(q string) map[string]string {
	words := strings.Fields(q)
	symbolToPathTokens := map[string]string{}
	for i, w := range words {
		// Is this word a possible symbol name? If not, continue.
		if strings.Contains(w, "/") || strings.Contains(w, "-") || commonHostnames[w] {
			continue
		}
		// If it is, try search for this word assuming it is the symbol name
		// and everything else is a path element.
		symbolToPathTokens[w] = strings.Join(append(append([]string{}, words[0:i]...), words[i+1:]...), " & ")
	}
	if len(symbolToPathTokens) > 2 {
		// There are more than 2 possible searches that can be performed, so
		// just perform an OR query.
		orQuery := strings.Join(strings.Fields(q), " | ")
		return map[string]string{orQuery: orQuery}
	}
	return symbolToPathTokens
}

// runSymbolSearchOneDot is used when q contains only 1 dot, so the search must
// either be for <package>.<symbol> or <type>.<methodOrFieldName>.
//
// This search is split into two parallel queries, since the query is very slow
// when using an OR in the WHERE clause.
func runSymbolSearchOneDot(ctx context.Context, ddb *database.DB, q string, limit int) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "runSymbolSearchOneDot(ctx, ddb, %q, %d)", q, limit)
	defer middleware.ElapsedStat(ctx, "runSymbolSearchOneDot")()

	group, searchCtx := errgroup.WithContext(ctx)
	resultsArray := make([][]*SearchResult, 2)
	for i, st := range []symbolsearch.SearchType{
		symbolsearch.SearchTypeSymbol,
		symbolsearch.SearchTypePackageDotSymbol,
	} {
		i := i
		st := st
		group.Go(func() error {
			var args []interface{}
			if st == symbolsearch.SearchTypePackageDotSymbol {
				args = append(args, q)
			}
			results, err := runSymbolSearch(searchCtx, ddb, st, q, limit, args...)
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
	return mergedResults(resultsArray, limit), nil
}

func runSymbolSearch(ctx context.Context, ddb *database.DB,
	st symbolsearch.SearchType, q string, limit int, args ...interface{}) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "runSymbolSearch(ctx, ddb, query, %q, %d)", q, limit)
	defer middleware.ElapsedStat(ctx, "runSymbolSearch")()

	ids, err := fetchMatchingSymbolIDs(ctx, ddb, st, q)
	if err != nil {
		if errors.Is(err, derrors.NotFound) {
			return nil, nil
		}
		return nil, err
	}
	return fetchSymbolSearchResults(ctx, ddb, st, ids, limit, args...)
}

// fetchMatchingSymbolIDs fetches the symbol ids to be used for a given
// symbolsearch.SearchType. It runs the query returned by
// symbolsearch.MatchingSymbolIDsQuery. The ids returned will be used by in
// runSymbolSearch.
func fetchMatchingSymbolIDs(ctx context.Context, ddb *database.DB, st symbolsearch.SearchType, q string) (_ []int, err error) {
	defer derrors.Wrap(&err, "fetchMatchingSymbolIDs(ctx, ddb, %d, %q)", st, q)
	defer middleware.ElapsedStat(ctx, "fetchMatchingSymbolIDs")()

	var ids []int
	collect := func(rows *sql.Rows) error {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	}
	query := symbolsearch.MatchingSymbolIDsQuery(st)
	if err := ddb.RunQuery(ctx, query, collect, q); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, derrors.NotFound
	}
	return ids, nil
}

// fetchSymbolSearchResults executes a symbol search for the given
// symbolsearch.SearchType and args.
func fetchSymbolSearchResults(ctx context.Context, ddb *database.DB,
	st symbolsearch.SearchType, ids []int, limit int, args ...interface{}) (results []*SearchResult, err error) {
	defer derrors.Wrap(&err, "fetchSymbolSearchResults(ctx, ddb, st: %d, ids: %v, limit:  %d, args: %v)", st, ids, limit, args)
	defer middleware.ElapsedStat(ctx, "fetchSymbolSearchResults")()

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
	query := symbolsearch.Query(st)
	args = append([]interface{}{pq.Array(ids), limit}, args...)
	if err := ddb.RunQuery(ctx, query, collect, args...); err != nil {
		return nil, err
	}
	return results, nil
}
