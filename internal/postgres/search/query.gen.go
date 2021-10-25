// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated with go generate -run gen_query.go. DO NOT EDIT.

package search

// querySearchSymbol is used when the search query is only one word, with no dots.
// In this case, the word must match a symbol name and ranking is completely
// determined by the path_tokens.
const querySearchSymbol = `
WITH ssd AS (
	SELECT
		ssd.unit_id,
		ssd.package_symbol_id,
		ssd.symbol_name_id,
		ssd.goos,
		ssd.goarch,
		ssd.imported_by_count AS score
	FROM symbol_search_documents ssd
	WHERE 
		lower(symbol_name) = lower($1)
	ORDER BY
		score DESC,
		package_path
	LIMIT $2
)
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

// querySearchPackageDotSymbol is used when the search query is one element
// containing a dot, where the first part is assumed to be the package name and
// the second the symbol name. For example, "sql.DB" or "sql.DB.Begin".
const querySearchPackageDotSymbol = `
WITH ssd AS (
	SELECT
		ssd.unit_id,
		ssd.package_symbol_id,
		ssd.symbol_name_id,
		ssd.goos,
		ssd.goarch,
		ssd.imported_by_count AS score
	FROM symbol_search_documents ssd
	WHERE 
		lower(symbol_name) = lower($1)
		AND (
			ssd.uuid_package_name=uuid_generate_v5(uuid_nil(), split_part($3, '.', 1)) OR
			ssd.uuid_package_path=uuid_generate_v5(uuid_nil(), split_part($3, '.', 1))
		)
	ORDER BY
		score DESC,
		package_path
	LIMIT $2
)
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

// querySearchMultiWordExact is used when the search query is multiple elements.
const querySearchMultiWordExact = `
WITH ssd AS (
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
				to_tsquery('symbols', quote_literal(replace($3, '_', '-')))
			) * sd.ln_imported_by_count
		) AS score
	FROM symbol_search_documents ssd
	INNER JOIN search_documents sd ON sd.package_path_id = ssd.package_path_id
	WHERE
		lower(symbol_name) = lower($1)
		AND sd.tsv_path_tokens @@ to_tsquery('symbols', quote_literal(replace($3, '_', '-')))
	ORDER BY score DESC
	LIMIT $2
)
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
