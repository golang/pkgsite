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
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/symbol"
)

// GetSymbolHistory returns a map of the first version when a symbol name is
// added to the API, to the symbol name, to the UnitSymbol struct. The
// UnitSymbol.Children field will always be empty, as children names are also
// tracked.
func (db *DB) GetSymbolHistory(ctx context.Context, packagePath, modulePath string,
) (_ map[string]map[string]*internal.UnitSymbol, err error) {
	defer derrors.Wrap(&err, "GetSymbolHistory(ctx, %q, %q)", packagePath, modulePath)
	defer middleware.ElapsedStat(ctx, "GetSymbolHistory")()

	if experiment.IsActive(ctx, internal.ExperimentReadSymbolHistory) {
		return GetSymbolHistoryFromTable(ctx, db.db, packagePath, modulePath, nil)
	}
	return GetSymbolHistoryWithPackageSymbols(ctx, db.db, packagePath, modulePath)
}

// GetSymbolHistoryFromTable fetches symbol history data from the symbol_history table.
//
// GetSymbolHistoryFromTable is exported for use in tests.
func GetSymbolHistoryFromTable(ctx context.Context, ddb *database.DB,
	packagePath, modulePath string, bc *internal.BuildContext) (_ map[string]map[string]*internal.UnitSymbol, err error) {
	defer derrors.WrapStack(&err, "GetSymbolHistoryFromTable(ctx, ddb, %q, %q)", packagePath, modulePath)

	q := squirrel.Select(
		"s1.name AS symbol_name",
		"s2.name AS parent_symbol_name",
		"ps.section",
		"ps.type",
		"ps.synopsis",
		"sh.since_version",
		"sh.goos",
		"sh.goarch",
	).From("symbol_history sh").
		Join("package_symbols ps ON ps.id = sh.package_symbol_id").
		Join("symbol_names s1 ON ps.symbol_name_id = s1.id").
		Join("symbol_names s2 ON ps.parent_symbol_name_id = s2.id").
		Join("paths p1 ON sh.package_path_id = p1.id").
		Join("paths p2 ON sh.module_path_id = p2.id").
		Where(squirrel.Eq{"p1.path": packagePath}).
		Where(squirrel.Eq{"p2.path": modulePath})
	if bc != nil {
		q = q.Where(squirrel.Eq{"sh.goos": bc.GOOS}).
			Where(squirrel.Eq{"sh.goarch": bc.GOARCH})
	}
	query, args, err := q.PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return nil, err
	}

	// versionToNameToUnitSymbol is a map of the version a symbol was
	// introduced, to the name and unit symbol.
	versionToNameToUnitSymbol := map[string]map[string]*internal.UnitSymbol{}
	collect := func(rows *sql.Rows) error {
		var (
			newUS internal.UnitSymbol
			build internal.BuildContext
			v     string
		)
		if err := rows.Scan(
			&newUS.Name,
			&newUS.ParentName,
			&newUS.Section,
			&newUS.Kind,
			&newUS.Synopsis,
			&v,
			&build.GOOS,
			&build.GOARCH,
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		nts, ok := versionToNameToUnitSymbol[v]
		if !ok {
			nts = map[string]*internal.UnitSymbol{}
			versionToNameToUnitSymbol[v] = nts
		}
		us, ok := nts[newUS.Name]
		if !ok {
			us = &newUS
			nts[newUS.Name] = us
		}
		us.AddBuildContext(build)
		return nil
	}
	if err := ddb.RunQuery(ctx, query, collect, args...); err != nil {
		return nil, err
	}
	return versionToNameToUnitSymbol, nil
}

// GetSymbolHistoryWithPackageSymbols fetches symbol history data by using data
// from package_symbols and documentation_symbols, and computed using
// symbol.IntroducedHistory.
//
// GetSymbolHistoryWithPackageSymbols is exported for use in tests.
func GetSymbolHistoryWithPackageSymbols(ctx context.Context, ddb *database.DB,
	packagePath, modulePath string) (_ map[string]map[string]*internal.UnitSymbol, err error) {
	versionToNameToUnitSymbols, err := getPackageSymbols(ctx, ddb, packagePath, modulePath)
	if err != nil {
		return nil, err
	}
	return symbol.IntroducedHistory(versionToNameToUnitSymbols), nil
}

// GetSymbolHistoryForBuildContext returns a map of the first version when a symbol name is
// added to the API for the specified build context, to the symbol name, to the
// UnitSymbol struct. The UnitSymbol.Children field will always be empty, as
// children names are also tracked.
func (db *DB) GetSymbolHistoryForBuildContext(ctx context.Context, packagePath, modulePath string,
	build internal.BuildContext) (nameToVersion map[string]string, err error) {
	defer derrors.Wrap(&err, "GetSymbolHistoryForBuildContext(ctx, %q, %q)", packagePath, modulePath)
	defer middleware.ElapsedStat(ctx, "GetSymbolHistoryForBuildContext")()

	var versionToNameToUnitSymbol map[string]map[string]*internal.UnitSymbol
	if experiment.IsActive(ctx, internal.ExperimentReadSymbolHistory) {
		if build.GOOS == internal.All {
			// It doesn't matter which one we use, so just pick a random one.
			build = internal.BuildContextLinux
		}
		versionToNameToUnitSymbol, err = GetSymbolHistoryFromTable(ctx, db.db, packagePath, modulePath, &build)
	} else {
		versionToNameToUnitSymbol, err = GetSymbolHistoryWithPackageSymbols(ctx, db.db, packagePath, modulePath)
	}
	if err != nil {
		return nil, err
	}

	nameToVersion = map[string]string{}
	for v, nts := range versionToNameToUnitSymbol {
		for n := range nts {
			nameToVersion[n] = v
		}
	}
	return nameToVersion, nil
}
