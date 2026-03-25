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
	"golang.org/x/pkgsite/internal/middleware/stats"
)

// GetSymbols returns all of the symbols for a given package path and module path.
func (db *DB) GetSymbols(ctx context.Context, pkgPath, modulePath, version string, bc internal.BuildContext) (_ []*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "DB.GetSymbols(ctx, %q, %q, %q, %v)", pkgPath, modulePath, version, bc)
	defer stats.Elapsed(ctx, "DB.GetSymbols")()

	uc, err := db.getUnitContext(ctx, pkgPath, modulePath, version, bc)
	if err != nil {
		return nil, err
	}
	if uc.docID == 0 {
		return nil, derrors.NotFound
	}

	query := packageSymbolQuery(
		squirrel.Select(
			"s1.name AS symbol_name",
			"s2.name AS parent_symbol_name",
			"ps.section",
			"ps.type",
			"ps.synopsis")).
		Where(squirrel.Eq{"ds.documentation_id": uc.docID}).
		OrderBy("CASE WHEN ps.type='Type' THEN 0 ELSE 1 END").
		OrderBy("s1.name")

	q, args, err := query.PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return nil, err
	}

	var symbols []*internal.Symbol
	symbolMap := make(map[string]*internal.Symbol)
	collect := func(rows *sql.Rows) error {
		var (
			name, parentName, synopsis string
			section                    internal.SymbolSection
			kind                       internal.SymbolKind
		)
		if err := rows.Scan(&name, &parentName, &section, &kind, &synopsis); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		sm := internal.SymbolMeta{
			Name:       name,
			ParentName: parentName,
			Section:    section,
			Kind:       kind,
			Synopsis:   synopsis,
		}
		if sm.ParentName != "" && sm.ParentName != sm.Name {
			if parent, ok := symbolMap[sm.ParentName]; ok {
				parent.Children = append(parent.Children, &sm)
				return nil
			}
		}
		// Treat as top-level if no parent or parent not found in this build context.
		s := &internal.Symbol{
			SymbolMeta: sm,
			GOOS:       uc.bestBC.GOOS,
			GOARCH:     uc.bestBC.GOARCH,
		}
		symbols = append(symbols, s)
		symbolMap[sm.Name] = s
		return nil
	}

	if err := db.db.RunQuery(ctx, q, collect, args...); err != nil {
		return nil, err
	}

	if len(symbols) == 0 {
		return nil, derrors.NotFound
	}
	return symbols, nil
}

// getPackageSymbols returns all of the symbols for a given package path and module path.
func getPackageSymbols(ctx context.Context, ddb *database.DB, packagePath, modulePath string,
) (_ *internal.SymbolHistory, err error) {
	defer derrors.Wrap(&err, "getPackageSymbols(ctx, ddb, %q, %q)", packagePath, modulePath)
	defer stats.Elapsed(ctx, "getPackageSymbols")()

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
		Where("NOT m.incompatible").
		Where(squirrel.Eq{"m.version_type": "release"}).
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

func packageSymbolQuery(query squirrel.SelectBuilder) squirrel.SelectBuilder {
	return query.From("documentation_symbols ds").
		Join("package_symbols ps ON ps.id = ds.package_symbol_id").
		Join("symbol_names s1 ON ps.symbol_name_id = s1.id").
		Join("symbol_names s2 ON ps.parent_symbol_name_id = s2.id")
}

func packageSymbolQueryJoin(query squirrel.SelectBuilder, pkgPath, modulePath string) squirrel.SelectBuilder {
	return packageSymbolQuery(query).
		Join("documentation d ON d.id = ds.documentation_id").
		Join("units u on u.id = d.unit_id").
		Join("modules m ON m.id = u.module_id").
		Join("paths p1 ON u.path_id = p1.id").
		Where(squirrel.Eq{"p1.path": pkgPath}).
		Where(squirrel.Eq{"m.module_path": modulePath})
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
