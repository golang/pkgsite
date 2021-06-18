// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Masterminds/squirrel"
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
