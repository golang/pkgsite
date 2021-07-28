// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbolsearch

import (
	"fmt"
	"strings"
)

// SymbolTextSearchConfiguration is a custom postgres text search configuration
// used for symbol search.
const SymbolTextSearchConfiguration = "symbols"

// Query returns a symbol search query to be used in internal/postgres.
// Each query that is returned accepts the following args:
// $1 = ids
// $2 = limit
// $3 = search query input (not used by SearchTypeSymbol)
func Query(st SearchType) string {
	var filter string
	switch st {
	case SearchTypeMultiWord:
		return fmt.Sprintf(baseQuery, multiwordCTE())
	case SearchTypePackageDotSymbol:
		// PackageDotSymbol case.
		filter = filterPackageDotSymbol
	case SearchTypeSymbol:
		filter = ""
	}
	return fmt.Sprintf(baseQuery, fmt.Sprintf(symbolCTE, filter))
}

const symbolCTE = `
	SELECT
		ssd.unit_id,
		ssd.package_symbol_id,
		ssd.symbol_name_id,
		ssd.goos,
		ssd.goarch,
		ssd.ln_imported_by_count AS score
	FROM symbol_search_documents ssd
	WHERE
		symbol_name_id = ANY($1) %s
	ORDER BY score DESC
	LIMIT $2
`

// TODO(golang/go#44142): Filtering on package path currently only works for
// standard library packages, since non-standard library packages will have a
// dot.
var filterPackageDotSymbol = fmt.Sprintf(`
	AND (
		ssd.uuid_package_name=%s OR
		ssd.uuid_package_path=%[1]s
	)`, "uuid_generate_v5(uuid_nil(), split_part($3, '.', 1))")

func multiwordCTE() string {
	return fmt.Sprintf(`
	SELECT
		ssd.unit_id,
		ssd.package_symbol_id,
		ssd.symbol_name_id,
		ssd.goos,
		ssd.goarch,
		(
			ts_rank(
				'{0.1, 0.2, 1.0, 1.0}',
				sd.tsv_path_tokens,
				to_tsquery('%s', %s)
			) * ssd.ln_imported_by_count
		) AS score
	FROM symbol_search_documents ssd
	INNER JOIN search_documents sd ON sd.package_path_id = ssd.package_path_id
	WHERE
		symbol_name_id = ANY($1)
		AND sd.tsv_path_tokens @@ to_tsquery('%[1]s', %[2]s)
	ORDER BY score DESC
	LIMIT $2
`,
		SymbolTextSearchConfiguration,
		splitORFunc(processArg("$3")))
}

const baseQuery = `
WITH ssd AS (%s)
SELECT
	s.name AS symbol_name,
	sd.package_path,
	sd.module_path,
	sd.version,
	sd.name,
	sd.synopsis,
	sd.license_types,
	sd.commit_time,
	sd.imported_by_count,
	ssd.goos,
	ssd.goarch,
	ps.type AS symbol_kind,
	ps.synopsis AS symbol_synopsis
FROM ssd
INNER JOIN symbol_names s ON s.id=ssd.symbol_name_id
INNER JOIN search_documents sd ON sd.unit_id = ssd.unit_id
INNER JOIN package_symbols ps ON ps.id=ssd.package_symbol_id
ORDER BY score DESC;`

// MatchingSymbolIDsQuery returns a query to fetch the symbol ids that match the
// search input, based on the SearchType.
func MatchingSymbolIDsQuery(st SearchType) string {
	var filter string
	switch st {
	case SearchTypeSymbol:
		// When $1 is the full symbol name, either <symbol> or
		// <type>.<methodOrField>, match on both the identifier name
		// and just the field or method name.
		// For example, "Begin" will return "DB.Begin".
		//
		// tsv_name_tokens does a bad job of indexing symbol names with
		// multiple "_", so also do an exact match search.
		filter = fmt.Sprintf(`tsv_name_tokens @@ %s OR lower(name) = lower($1)`,
			toTSQuery("$1"))
	case SearchTypePackageDotSymbol:
		// When $1 is either <package>.<symbol> OR
		// <package>.<type>.<methodOrField>, only match on the exact
		// symbol name.
		filter = fmt.Sprintf("lower(name) = lower(%s)", "substring($1 from E'[^.]*\\.(.+)$')")
	case SearchTypeMultiWord:
		// When $1 contains multiple words, separated by spaces, at least one
		// element for the query must match a symbol name.
		//
		// TODO(44142): This is currently somewhat slow, since many IDs can be
		// returned.
		filter = fmt.Sprintf(`tsv_name_tokens @@ %s`, toTSQuery(splitORFunc("$1")))
	}
	return fmt.Sprintf(`
		SELECT id
		FROM symbol_names
		WHERE %s`, filter)
}

func splitORFunc(arg string) string {
	return fmt.Sprintf("replace(%s, ' ', ' | ')", arg)
}

func toTSQuery(arg string) string {
	return fmt.Sprintf("to_tsquery('%s', %s)", SymbolTextSearchConfiguration, processArg(arg))
}

// processArg converts a symbol with underscores to slashes (for example,
// "A_B" -> "A-B"). This is because the postgres parser treats underscores as
// slashes, but we want a search for "A" to rank "A_B" lower than just "A". We
// also want to be able to search specificially for "A_B".
func processArg(arg string) string {
	s := "$1"
	if len(arg) == 2 && strings.HasPrefix(arg, "$") {
		// If the arg is a different $N, substitute that instead.
		s = arg
	}
	return strings.ReplaceAll(arg, s, fmt.Sprintf("replace(%s, '_', '-')", s))
}
