// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symbolsearch

import (
	"fmt"
)

func newQuery(st SearchType) string {
	var filter string
	switch st {
	case SearchTypeMultiWord:
		return fmt.Sprintf(baseQuery, multiwordCTE)
	case SearchTypeSymbol:
		filter = ""
	case SearchTypePackageDotSymbol:
		// PackageDotSymbol case.
		filter = newfilterPackageDotSymbol
	}
	q := fmt.Sprintf(baseQuery, fmt.Sprintf(symbolCTE, filter))
	return q
}

// TODO(golang/go#44142): Filtering on package path currently only works for
// standard library packages, since non-standard library packages will have a
// dot.
const newfilterPackageDotSymbol = `
	AND (
		ssd.uuid_package_name=uuid_generate_v5(uuid_nil(), split_part($3, '.', 1)) OR
		ssd.uuid_package_path=uuid_generate_v5(uuid_nil(), split_part($3, '.', 1))
	)`

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

var multiwordCTE = fmt.Sprintf(`
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

// matchingIDsQuery returns a query to fetch the symbol ids that match the
// search input, based on the SearchType.
func matchingIDsQuery(st SearchType) string {
	var filter string
	switch st {
	case SearchTypeSymbol:
		// When searching for just a symbol, match on both the identifier name
		// and just the field or method name. For example, "Begin" will return
		// "DB.Begin".
		// tsv_name_tokens does a bad job of indexing symbol names with
		// multiple "_", so also do an exact match search.
		filter = fmt.Sprintf(`tsv_name_tokens @@ %s OR lower(name) = lower($1)`,
			toTSQuery("$1"))
	case SearchTypePackageDotSymbol:
		// When searching for a <package>.<symbol>, only match on the exact
		// symbol name. It is assumed that $1 = <package>.<symbol>.
		filter = fmt.Sprintf("lower(name) = lower(%s)", "substring($1 from E'[^.]*\\.(.+)$')")
	case SearchTypeMultiWord:
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
