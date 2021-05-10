// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/middleware"
)

// getPackageSymbols returns all of the symbols for a given package path and module path.
func getPackageSymbols(ctx context.Context, ddb *database.DB, packagePath, modulePath string,
) (_ *internal.SymbolHistory, err error) {
	defer derrors.Wrap(&err, "getPackageSymbols(ctx, ddb, %q, %q)", packagePath, modulePath)
	defer middleware.ElapsedStat(ctx, "getPackageSymbols")()

	query := packageSymbolQueryJoin(
		squirrel.Select(
			"s1.name AS symbol_name",
			"s2.name AS parent_symbol_name",
			"ps.section",
			"ps.type",
			"ps.synopsis",
			"m.version",
			"d.goos",
			"d.goarch"), packagePath, modulePath).
		OrderBy("CASE WHEN ps.type='Type' THEN 0 ELSE 1 END").
		OrderBy("s1.name")
	q, args, err := query.PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return nil, err
	}
	sh, collect := collectSymbolHistory(func(sh *internal.SymbolHistory, sm internal.SymbolMeta, v string, build internal.BuildContext) error {
		if sm.Section == internal.SymbolSectionTypes && sm.Kind != internal.SymbolKindType {
			_, err := sh.GetSymbol(sm.ParentName, v, build)
			if err != nil {
				return fmt.Errorf("could not find parent for %q: %v", sm.Name, err)
			}
			return nil
		}
		return nil
	})
	if err := ddb.RunQuery(ctx, q, collect, args...); err != nil {
		return nil, err
	}
	return sh, nil
}

func packageSymbolQueryJoin(query squirrel.SelectBuilder, pkgPath, modulePath string) squirrel.SelectBuilder {
	return query.From("modules m").
		Join("units u on u.module_id = m.id").
		Join("documentation d ON d.unit_id = u.id").
		Join("documentation_symbols ds ON ds.documentation_id = d.id").
		Join("package_symbols ps ON ps.id = ds.package_symbol_id").
		Join("paths p1 ON u.path_id = p1.id").
		Join("symbol_names s1 ON ps.symbol_name_id = s1.id").
		Join("symbol_names s2 ON ps.parent_symbol_name_id = s2.id").
		Where(squirrel.Eq{"p1.path": pkgPath}).
		Where(squirrel.Eq{"m.module_path": modulePath}).
		Where("NOT m.incompatible").
		Where(squirrel.Eq{"m.version_type": "release"})
}

func collectSymbolHistory(check func(sh *internal.SymbolHistory, sm internal.SymbolMeta, v string, build internal.BuildContext) error) (*internal.SymbolHistory, func(rows *sql.Rows) error) {
	sh := internal.NewSymbolHistory()
	return sh, func(rows *sql.Rows) (err error) {
		defer derrors.Wrap(&err, "collectSymbolHistory")
		var (
			sm    internal.SymbolMeta
			build internal.BuildContext
			v     string
		)
		if err := rows.Scan(
			&sm.Name,
			&sm.ParentName,
			&sm.Section,
			&sm.Kind,
			&sm.Synopsis,
			&v,
			&build.GOOS,
			&build.GOARCH,
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		if err := check(sh, sm, v, build); err != nil {
			return fmt.Errorf("check(): %v", err)
		}
		sh.AddSymbol(sm, v, build)
		return nil
	}
}

func upsertSearchDocumentSymbols(ctx context.Context, ddb *database.DB,
	packagePath, modulePath, v string) (err error) {
	defer derrors.Wrap(&err, "upsertSearchDocumentSymbols(ctx, ddb, %q, %q, %q)", packagePath, modulePath, v)
	defer middleware.ElapsedStat(ctx, "upsertSearchDocumentSymbols")()

	// If a user is looking for the symbol "DB.Begin", from package
	// database/sql, we want them to be able to find this by searching for
	// "DB.Begin" and "sql.DB.Begin". Searching for "sql.DB", "DB", "Begin" or
	// "sql.DB" will not return "DB.Begin".
	query := packageSymbolQueryJoin(squirrel.Select(
		"p1.id AS package_path_id",
		"s1.id AS symbol_name_id",
		// Group the build contexts as an array, with the format
		// "<goos>/<goarch>". We only care about the build contexts when the
		// default goos/goarch for the package page does not contain the
		// matching symbol.
		//
		// TODO(https://golang/issue/44142): We could probably get away with
		// storing just the GOOS value, since we don't really need the GOARCH
		// to link to a symbol page. If we do that we should also change the
		// column type to []goos.
		//
		// Store in order of the build context list at internal.BuildContexts.
		`ARRAY_AGG(FORMAT('%s/%s', d.goos, d.goarch)
			ORDER BY
				CASE WHEN d.goos='linux' THEN 0
				WHEN d.goos='windows' THEN 1
				WHEN d.goos='darwin' THEN 2
				WHEN d.goos='js' THEN 3 END)`,
		// If a user is looking for the symbol "DB.Begin", from package
		// database/sql, we want them to be able to find this by searching for
		// "DB.Begin", "Begin", and "sql.DB.Begin". Searching for "sql.DB" or
		// "DB" will not return "DB.Begin".
		//
		// Index <package>.<identifier> (i.e. "sql.DB.Begin")
		`SETWEIGHT(
			TO_TSVECTOR('simple', concat(s1.name, ' ', concat(u.name, '.', s1.name))),
			'A') ||`+
			// Index <identifier>, including the parent name (i.e. DB.Begin).
			`SETWEIGHT(
				TO_TSVECTOR('simple', s1.name),
				'A') ||`+
			// Index <identifier> without parent name (i.e. "Begin").
			//
			// This is weighted less, so that if other symbols are just named
			// "Begin" they will rank higher in a search for "Begin".
			`SETWEIGHT(
				TO_TSVECTOR('simple', split_part(s1.name, '.', 2)),
				'B') AS tokens`,
	), packagePath, modulePath).
		Where(squirrel.Eq{"m.version": v}).
		GroupBy("p1.id, s1.id", "tokens").
		OrderBy("s1.name")

	q, args, err := query.PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return err
	}

	var values []interface{}
	collect := func(rows *sql.Rows) (err error) {
		var (
			packagePathID int
			symbolNameID  int
			tokens        string
			buildContexts []string
		)
		if err := rows.Scan(
			&packagePathID,
			&symbolNameID,
			pq.Array(&buildContexts),
			&tokens,
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		values = append(values, packagePathID, symbolNameID, pq.Array(buildContexts), tokens)
		return nil
	}
	if err := ddb.RunQuery(ctx, q, collect, args...); err != nil {
		return err
	}

	columns := []string{"package_path_id", "symbol_name_id", "build_contexts", "tsv_symbol_tokens"}
	return ddb.BulkInsert(ctx, "symbol_search_documents", columns, values,
		`ON CONFLICT (package_path_id, symbol_name_id)
				DO UPDATE
				SET
					build_contexts=excluded.build_contexts,
					tsv_symbol_tokens=excluded.tsv_symbol_tokens`)
}
