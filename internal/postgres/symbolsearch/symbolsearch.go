// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run gen_query.go

// Package symbolsearch provides helper functions for constructing queries for
// symbol search, which are using in internal/postgres.
//
// The exported queries are generated using gen_query.go. query.gen.go should
// never be edited directly. It should always be recreated by running
// `go generate -run gen_query.go`.
package symbolsearch

import (
	"fmt"
	"strings"
)

const SymbolTextSearchConfiguration = "symbols"

var (
	rawQuerySymbol           = fmt.Sprintf(symbolSearchBaseQuery, scoreMultipliers, filterSymbol)
	rawQueryPackageDotSymbol = fmt.Sprintf(symbolSearchBaseQuery, scoreMultipliers, filterPackageDotSymbol)
	rawQueryOneDot           = fmt.Sprintf(symbolSearchBaseQuery, scoreMultipliers, filterOneDot)
	rawQueryMultiWord        = fmt.Sprintf(symbolSearchBaseQuery, formatScore(scoreMultiWord), filterMultiWord)
)

var (
	// filterSymbol is used when $1 is the full symbol name, either
	// <symbol> or <type>.<methodOrField>.
	filterSymbol = fmt.Sprintf(`s.tsv_name_tokens @@ %s`, toTSQuery("$1"))

	// filterSymbol is used when $1 contains the full symbol name, either
	// <symbol> or <type>.<methodOrField>, and has multiple words.
	filterSymbolOR = fmt.Sprintf(`s.tsv_name_tokens @@ %s`, toTSQuery(splitOR))

	// filterPackageDotSymbol is used when $1 is either <package>.<symbol> OR
	// <package>.<type>.<methodOrField>.
	filterPackageDotSymbol = fmt.Sprintf("%s AND %s",
		// Split the package name from $1, which can be assumed to be the
		// element preceding the first dot.
		formatFilter("sd.name = split_part($1, '.', 1)"),
		// Split the symbol name from $1, which can be assumed to be everything
		// following the first dot.
		fmt.Sprintf(formatFilter("s.tsv_name_tokens @@ %s"),
			toTSQuery("substring($1 from E'[^.]*\\.(.+)$')")))

	// filterOneDot is used when $1 is one word containing a single dot, which
	// means it is either <package>.<symbol> or <type>.<methodOrField>.
	filterOneDot = fmt.Sprintf("%s OR %s", filterPackageDotSymbol, filterSymbol)

	// filterPackage is used to filter matching elements from
	// sd.tsv_path_tokens.
	filterPackage = fmt.Sprintf(`sd.tsv_path_tokens @@ %s`, toTSQuery(splitOR))

	// filterMultiWord when $1 contains multiple words, separated by spaces.
	// One element for the query must match a symbol name, and one (could be
	// the same element) must match the package name.
	filterMultiWord = fmt.Sprintf("%s AND %s", formatFilter(filterSymbolOR),
		formatFilter(filterPackage))
)

var (
	// scoreMultiWord is the score when $1 contains multiple words.
	scoreMultiWord = fmt.Sprintf("%s%s", rankPathTokens, formatMultiplier(scoreMultipliers))

	// scoreMultipliers is the score of multiplying the multiplers.
	//
	// It is also used as the score for QuerySymbol and QueryPackageDotIdentifier.
	// In both cases, the matching symbols will be filtered in the WHERE
	// clause, and the only remaining information to rank the results by are
	// the multiplers.
	scoreMultipliers = fmt.Sprintf("%s%s%s",
		popularityMultiplier,
		formatMultiplier(redistributableMultipler),
		formatMultiplier(goModMultipler))

	rankPathTokens = fmt.Sprintf(
		"ts_rank(%s,%s,%s"+indent(")", 3),
		indent("'{0.1, 0.2, 1.0, 1.0}'", 4),
		indent("sd.tsv_path_tokens", 4),
		indent(toTSQuery(splitOR), 4))

	// Popularity multipler to increase ranking of popular packages.
	popularityMultiplier = `ln(exp(1)+imported_by_count)`

	// Multipler based on whether the module license is non-redistributable.
	redistributableMultipler = fmt.Sprintf(
		`CASE WHEN sd.redistributable THEN 1 ELSE %f END`,
		nonRedistributablePenalty)

	// Multipler based on wehther the module has a go.mod file.
	goModMultipler = fmt.Sprintf(
		`CASE WHEN COALESCE(has_go_mod, true) THEN 1 ELSE %f END`,
		noGoModPenalty)
)

func formatScore(s string) string {
	return fmt.Sprintf("(\n\t\t\t\t%s\n\t\t\t)", s)
}

func formatFilter(s string) string {
	return fmt.Sprintf("(\n\t\t\t%s\n\t\t)", s)
}

func formatMultiplier(s string) string {
	return indent(fmt.Sprintf("* %s", s), 3)
}

func indent(s string, n int) string {
	for i := 0; i <= n; i++ {
		s = "\t" + s
	}
	return "\n" + s
}

const splitOR = "replace($1, ' ', ' | ')"

// Penalties to search scores, applied as multipliers to the score.
const (
	// Module license is non-redistributable.
	nonRedistributablePenalty = 0.5
	// Module does not have a go.mod file.
	// Start this off gently (close to 1), but consider lowering
	// it as time goes by and more of the ecosystem converts to modules.
	noGoModPenalty = 0.8
)

func toTSQuery(arg string) string {
	return fmt.Sprintf("to_tsquery('%s', %s)", SymbolTextSearchConfiguration, processArg(arg))
}

// processSymbol converts a symbol with underscores to slashes (for example,
// "A_B" -> "A-B"). This is because the postgres parser treats underscores as
// slashes, but we want a search for "A" to rank "A_B" lower than just "A". We
// also want to be able to search specificially for "A_B".
func processArg(arg string) string {
	return strings.ReplaceAll(arg, "$1", "replace($1, '_', '-')")
}

const symbolSearchBaseQuery = `
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
			%s AS score
	FROM symbol_search_documents ssd
	INNER JOIN search_documents sd ON sd.unit_id = ssd.unit_id
	INNER JOIN symbol_names s ON s.id = ssd.symbol_name_id
	WHERE %s
)
SELECT
	r.package_path,
	r.module_path,
	r.version,
	r.package_name,
	r.synopsis,
	r.license_types,
	r.commit_time,
	r.imported_by_count,
	r.symbol_name,
	r.goos,
	r.goarch,
	ps.type AS symbol_type,
	ps.synopsis AS symbol_synopsis,
	COUNT(*) OVER() AS total
FROM results r
INNER JOIN package_symbols ps ON r.package_symbol_id = ps.id
WHERE r.score > 0.1
ORDER BY
	score DESC,
	commit_time DESC,
	symbol_name,
	package_path
LIMIT $2
OFFSET $3;`
