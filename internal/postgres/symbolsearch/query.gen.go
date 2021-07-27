// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated with go generate -run gen_query.go. DO NOT EDIT.

package symbolsearch

// QuerySymbol is used when the search query is only one word, with no dots.
// In this case, the word must match a symbol name and ranking is completely
// determined by the path_tokens.
const QuerySymbol = `
WITH results AS (
	SELECT
			s.name AS symbol_name,
			sd.package_path,
			sd.module_path,
			sd.version,
			sd.name AS package_name,
			sd.synopsis,
			sd.license_types,
			sd.commit_time,
			sd.imported_by_count,
			ssd.package_symbol_id,
			ssd.goos,
			ssd.goarch,
			ln(exp(1)+sd.imported_by_count) AS score
	FROM symbol_search_documents ssd
	INNER JOIN search_documents sd ON sd.unit_id = ssd.unit_id
	INNER JOIN symbol_names s ON s.id = ssd.symbol_name_id
	WHERE s.tsv_name_tokens @@ to_tsquery('symbols', replace($1, '_', '-'))
)
SELECT
	r.symbol_name,
	r.package_path,
	r.module_path,
	r.version,
	r.package_name,
	r.synopsis,
	r.license_types,
	r.commit_time,
	r.imported_by_count,
	r.goos,
	r.goarch,
	ps.type AS symbol_type,
	ps.synopsis AS symbol_synopsis
FROM results r
INNER JOIN package_symbols ps ON r.package_symbol_id = ps.id
WHERE r.score > 0.1
ORDER BY
	score DESC,
	commit_time DESC,
	symbol_name,
	package_path
LIMIT $2;`

// QueryPackageDotSymbol is used when the search query is one element
// containing a dot, where the first part is assumed to be the package name and
// the second the symbol name. For example, "sql.DB" or "sql.DB.Begin".
const QueryPackageDotSymbol = `
WITH results AS (
	SELECT
			s.name AS symbol_name,
			sd.package_path,
			sd.module_path,
			sd.version,
			sd.name AS package_name,
			sd.synopsis,
			sd.license_types,
			sd.commit_time,
			sd.imported_by_count,
			ssd.package_symbol_id,
			ssd.goos,
			ssd.goarch,
			ln(exp(1)+sd.imported_by_count) AS score
	FROM symbol_search_documents ssd
	INNER JOIN search_documents sd ON sd.unit_id = ssd.unit_id
	INNER JOIN symbol_names s ON s.id = ssd.symbol_name_id
	WHERE (sd.name=split_part($1, '.', 1) OR sd.package_path=split_part($1, '.', 1)) AND (
			s.tsv_name_tokens @@ to_tsquery('symbols', substring(replace($1, '_', '-') from E'[^.]*\.(.+)$'))
		)
)
SELECT
	r.symbol_name,
	r.package_path,
	r.module_path,
	r.version,
	r.package_name,
	r.synopsis,
	r.license_types,
	r.commit_time,
	r.imported_by_count,
	r.goos,
	r.goarch,
	ps.type AS symbol_type,
	ps.synopsis AS symbol_synopsis
FROM results r
INNER JOIN package_symbols ps ON r.package_symbol_id = ps.id
WHERE r.score > 0.1
ORDER BY
	score DESC,
	commit_time DESC,
	symbol_name,
	package_path
LIMIT $2;`

// QueryOneDot is used when the search query is one element
// containing a dot. This means it can either be <package>.<symbol> or
// <type>.<methodOrField>.
const QueryOneDot = `
WITH results AS (
	SELECT
			s.name AS symbol_name,
			sd.package_path,
			sd.module_path,
			sd.version,
			sd.name AS package_name,
			sd.synopsis,
			sd.license_types,
			sd.commit_time,
			sd.imported_by_count,
			ssd.package_symbol_id,
			ssd.goos,
			ssd.goarch,
			ln(exp(1)+sd.imported_by_count) AS score
	FROM symbol_search_documents ssd
	INNER JOIN search_documents sd ON sd.unit_id = ssd.unit_id
	INNER JOIN symbol_names s ON s.id = ssd.symbol_name_id
	WHERE (sd.name=split_part($1, '.', 1) OR sd.package_path=split_part($1, '.', 1)) AND (
			s.tsv_name_tokens @@ to_tsquery('symbols', substring(replace($1, '_', '-') from E'[^.]*\.(.+)$'))
		) OR s.tsv_name_tokens @@ to_tsquery('symbols', replace($1, '_', '-'))
)
SELECT
	r.symbol_name,
	r.package_path,
	r.module_path,
	r.version,
	r.package_name,
	r.synopsis,
	r.license_types,
	r.commit_time,
	r.imported_by_count,
	r.goos,
	r.goarch,
	ps.type AS symbol_type,
	ps.synopsis AS symbol_synopsis
FROM results r
INNER JOIN package_symbols ps ON r.package_symbol_id = ps.id
WHERE r.score > 0.1
ORDER BY
	score DESC,
	commit_time DESC,
	symbol_name,
	package_path
LIMIT $2;`

// QueryMultiWord is used when the search query is multiple elements.
const QueryMultiWord = `
WITH results AS (
	SELECT
			s.name AS symbol_name,
			sd.package_path,
			sd.module_path,
			sd.version,
			sd.name AS package_name,
			sd.synopsis,
			sd.license_types,
			sd.commit_time,
			sd.imported_by_count,
			ssd.package_symbol_id,
			ssd.goos,
			ssd.goarch,
			(
				ts_rank(
					'{0.1, 0.2, 1.0, 1.0}',
					sd.tsv_path_tokens,
					to_tsquery('symbols', replace(replace($1, '_', '-'), ' ', ' | '))
				)
				* ln(exp(1)+sd.imported_by_count)
			) AS score
	FROM symbol_search_documents ssd
	INNER JOIN search_documents sd ON sd.unit_id = ssd.unit_id
	INNER JOIN symbol_names s ON s.id = ssd.symbol_name_id
	WHERE (
			s.tsv_name_tokens @@ to_tsquery('symbols', replace(replace($1, '_', '-'), ' ', ' | '))
		) AND (
			sd.tsv_path_tokens @@ to_tsquery('symbols', replace(replace($1, '_', '-'), ' ', ' | '))
		)
)
SELECT
	r.symbol_name,
	r.package_path,
	r.module_path,
	r.version,
	r.package_name,
	r.synopsis,
	r.license_types,
	r.commit_time,
	r.imported_by_count,
	r.goos,
	r.goarch,
	ps.type AS symbol_type,
	ps.synopsis AS symbol_synopsis
FROM results r
INNER JOIN package_symbols ps ON r.package_symbol_id = ps.id
WHERE r.score > 0.1
ORDER BY
	score DESC,
	commit_time DESC,
	symbol_name,
	package_path
LIMIT $2;`
