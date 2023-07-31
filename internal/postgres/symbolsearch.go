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
	"golang.org/x/pkgsite/internal/middleware/stats"
	"golang.org/x/pkgsite/internal/postgres/search"
	"golang.org/x/sync/errgroup"
)

func upsertSymbolSearchDocuments(ctx context.Context, tx *database.DB,
	modulePath, v string) (err error) {
	defer derrors.Wrap(&err, "upsertSymbolSearchDocuments(ctx, ddb, %q, %q)", modulePath, v)
	defer stats.Elapsed(ctx, "upsertSymbolSearchDocuments")()

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
func (db *DB) symbolSearch(ctx context.Context, q string, limit int, opts SearchOptions) searchResponse {
	defer stats.Elapsed(ctx, "symbolSearch")()

	var (
		results []*SearchResult
		err     error
	)
	sr := searchResponse{source: "symbol"}
	it := search.ParseInputType(q)
	switch it {
	case search.InputTypeOneDot:
		results, err = runSymbolSearchOneDot(ctx, db.db, q, limit)
	case search.InputTypeMultiWord:
		results, err = runSymbolSearchMultiWord(ctx, db.db, q, limit, opts.SymbolFilter)
	case search.InputTypeNoDot:
		results, err = runSymbolSearch(ctx, db.db, search.SearchTypeSymbol, q, limit)
	case search.InputTypeTwoDots:
		results, err = runSymbolSearchPackageDotSymbol(ctx, db.db, q, limit)
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
func runSymbolSearchMultiWord(ctx context.Context, ddb *database.DB, q string, limit int,
	symbolFilter string) (_ []*SearchResult, err error) {
	defer derrors.Wrap(&err, "runSymbolSearchMultiWord(ctx, ddb, query, %q, %d, %q)",
		q, limit, symbolFilter)
	defer stats.Elapsed(ctx, "runSymbolSearchMultiWord")()

	symbolToPathTokens := multiwordSearchCombinations(q, symbolFilter)
	if len(symbolToPathTokens) == 0 {
		// There are no words in the query that could be a symbol name.
		return nil, derrors.NotFound
	}
	if strings.Contains(q, "|") {
		// TODO(golang/go#44142): The search.SearchTypeMultiWordOr case
		// is currently not supported.
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
			st := search.SearchTypeMultiWordExact
			r, err := runSymbolSearch(searchCtx, ddb, st, symbol, limit, pathTokens)
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
func multiwordSearchCombinations(q, symbolFilter string) map[string]string {
	words := strings.Fields(q)
	symbolToPathTokens := map[string]string{}
	for i, w := range words {
		// Is this word a possible symbol name? If not, continue.
		if strings.Contains(w, "/") || strings.Contains(w, "-") || commonHostnames[w] {
			continue
		}
		// A symbolFilter was used, and this word does not match it, so
		// it can't be the symbol name.
		if symbolFilter != "" && w != symbolFilter {
			continue
		}
		// If it is, try search for this word assuming it is the symbol name
		// and everything else is a path element.
		pathTokens := append(append([]string{}, words[0:i]...), words[i+1:]...)
		sort.Strings(pathTokens)
		symbolToPathTokens[w] = strings.Join(pathTokens, " & ")
	}
	if len(symbolToPathTokens) == 0 {
		return nil
	}
	if len(symbolToPathTokens) > 3 {
		// There are too many searches that can be performed, so
		// return no results.
		// TODO(golang/go#44142): Leave add support for an OR query.
		return nil
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
	defer stats.Elapsed(ctx, "runSymbolSearchOneDot")()

	group, searchCtx := errgroup.WithContext(ctx)
	resultsArray := make([][]*SearchResult, 2)
	for i, st := range []search.SearchType{
		search.SearchTypeSymbol,
		search.SearchTypePackageDotSymbol,
	} {
		i := i
		st := st
		group.Go(func() error {
			var (
				results []*SearchResult
				err     error
			)
			if st == search.SearchTypePackageDotSymbol {
				results, err = runSymbolSearchPackageDotSymbol(searchCtx, ddb, q, limit)
			} else {
				results, err = runSymbolSearch(searchCtx, ddb, st, q, limit)
			}
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

func runSymbolSearchPackageDotSymbol(ctx context.Context, ddb *database.DB, q string, limit int) (_ []*SearchResult, err error) {
	pkg, symbol, err := splitPackageAndSymbolNames(q)
	if err != nil {
		return nil, err
	}
	return runSymbolSearch(ctx, ddb, search.SearchTypePackageDotSymbol, symbol, limit, pkg)
}

func splitPackageAndSymbolNames(q string) (pkgName string, symbolName string, err error) {
	parts := strings.Split(q, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return "", "", derrors.NotFound
	}
	for _, p := range parts {
		// Handle cases where we have odd dot placement, such as .Foo or
		// Foo..
		if p == "" {
			return "", "", derrors.NotFound
		}
	}
	return parts[0], strings.Join(parts[1:], "."), nil
}

func runSymbolSearch(ctx context.Context, ddb *database.DB,
	st search.SearchType, q string, limit int, args ...any) (results []*SearchResult, err error) {
	defer derrors.Wrap(&err, "runSymbolSearch(ctx, ddb, %q, %q, %d, %v)", st, q, limit, args)
	defer stats.Elapsed(ctx, fmt.Sprintf("%s-runSymbolSearch", st))()

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
	query := search.SymbolQuery(st)
	args = append([]any{q, limit}, args...)
	if err := ddb.RunQuery(ctx, query, collect, args...); err != nil {
		return nil, err
	}
	return results, nil
}
