// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

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
	query := `
		SELECT
			s1.name AS symbol_name,
			s2.name AS parent_symbol_name,
			ps.section,
			ps.type,
			ps.synopsis,
			m.version,
			d.goos,
			d.goarch
		FROM modules m
		INNER JOIN units u ON u.module_id = m.id
		INNER JOIN documentation d ON d.unit_id = u.id
		INNER JOIN documentation_symbols ds ON ds.documentation_id = d.id
		INNER JOIN package_symbols ps ON ps.id = ds.package_symbol_id
		INNER JOIN paths p1 ON u.path_id = p1.id
		INNER JOIN symbol_names s1 ON ps.symbol_name_id = s1.id
		INNER JOIN symbol_names s2 ON ps.parent_symbol_name_id = s2.id
		WHERE
			p1.path = $1
			AND m.module_path = $2
			AND NOT m.incompatible
			AND m.version_type = 'release'
		ORDER BY
			CASE WHEN ps.type='Type' THEN 0 ELSE 1 END,
			symbol_name;`

	sh, collect := collectSymbolHistory(func(sh *internal.SymbolHistory, sm internal.SymbolMeta, v string, build internal.BuildContext) error {
		if sm.Section == internal.SymbolSectionTypes && sm.Kind != internal.SymbolKindType {
			_, err := sh.GetSymbol(sm.ParentName, v, build)
			return err
		}
		return nil
	})
	if err := ddb.RunQuery(ctx, query, collect, packagePath, modulePath); err != nil {
		return nil, err
	}
	return sh, nil
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

// legacyGetPackageSymbols returns all of the symbols for a given package path and module path.
func legacyGetPackageSymbols(ctx context.Context, ddb *database.DB, packagePath, modulePath string,
) (_ map[string]map[string]*internal.UnitSymbol, err error) {
	defer derrors.Wrap(&err, "legacyGetPackageSymbols(ctx, ddb, %q, %q)", packagePath, modulePath)
	defer middleware.ElapsedStat(ctx, "getPackageSymbols")()
	query := `
		SELECT
			s1.name AS symbol_name,
			s2.name AS parent_symbol_name,
			ps.section,
			ps.type,
			ps.synopsis,
			m.version,
			d.goos,
			d.goarch
		FROM modules m
		INNER JOIN units u ON u.module_id = m.id
		INNER JOIN documentation d ON d.unit_id = u.id
		INNER JOIN documentation_symbols ds ON ds.documentation_id = d.id
		INNER JOIN package_symbols ps ON ps.id = ds.package_symbol_id
		INNER JOIN paths p1 ON u.path_id = p1.id
		INNER JOIN symbol_names s1 ON ps.symbol_name_id = s1.id
		INNER JOIN symbol_names s2 ON ps.parent_symbol_name_id = s2.id
		WHERE
			p1.path = $1
			AND m.module_path = $2
			AND NOT m.incompatible
			AND m.version_type = 'release'
		ORDER BY
			CASE WHEN ps.type='Type' THEN 0 ELSE 1 END,
			symbol_name;`

	// versionToNameToUnitSymbol contains all of the types for this unit,
	// grouped by name and build context. This is used to keep track of the
	// parent types, so that we can map the children to those symbols.
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
		if newUS.Section == internal.SymbolSectionTypes && newUS.Kind != internal.SymbolKindType {
			if err := validateChildSymbol(&newUS, v, build, versionToNameToUnitSymbol); err != nil {
				return err
			}
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
	if err := ddb.RunQuery(ctx, query, collect, packagePath, modulePath); err != nil {
		return nil, err
	}
	return versionToNameToUnitSymbol, nil
}

func validateChildSymbol(us *internal.UnitSymbol, v string, build internal.BuildContext,
	versionToNameToUnitSymbol map[string]map[string]*internal.UnitSymbol) error {
	nameToUnitSymbol, ok := versionToNameToUnitSymbol[v]
	if !ok {
		return fmt.Errorf("version %q could not be found: %q", v, us.Name)
	}
	parent, ok := nameToUnitSymbol[us.ParentName]
	if !ok {
		return fmt.Errorf("parent %q could not be found at version %q: %q",
			us.ParentName, v, us.Name)
	}
	if !parent.SupportsBuild(build) {
		return fmt.Errorf("parent %q does not have build %v at version %q: %q",
			us.ParentName, build, v, us.Name)
	}
	return nil
}
