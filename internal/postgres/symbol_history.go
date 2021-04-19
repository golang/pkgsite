// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/middleware"
)

// GetPackageSymbols returns all of the symbols for a given package path and module path.
func (db *DB) GetPackageSymbols(ctx context.Context, packagePath, modulePath string,
) (_ map[string]map[string]*internal.UnitSymbol, err error) {
	defer derrors.Wrap(&err, "GetPackageSymbols(ctx, db, %q, %q)", packagePath, modulePath)
	defer middleware.ElapsedStat(ctx, "GetPackageSymbols")

	doctable := "new_documentation"
	if experiment.IsActive(ctx, internal.ExperimentDoNotInsertNewDocumentation) {
		doctable = "documentation"
	}
	query := fmt.Sprintf(`
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
		INNER JOIN %s d ON d.unit_id = u.id
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
			symbol_name;`, doctable)

	// versionToNameToUnitSymbol contains all of the types for this unit,
	// grouped by name and build context. This is used to keep track of the
	// parent types, so that we can map the children to those symbols.
	versionToNameToUnitSymbol := map[string]map[string]*internal.UnitSymbol{}
	collect := func(rows *sql.Rows) error {
		var (
			newUS internal.UnitSymbol
			build internal.BuildContext
		)
		if err := rows.Scan(
			&newUS.Name,
			&newUS.ParentName,
			&newUS.Section,
			&newUS.Kind,
			&newUS.Synopsis,
			&newUS.Version,
			&build.GOOS,
			&build.GOARCH,
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		if newUS.Section == internal.SymbolSectionTypes && newUS.Kind != internal.SymbolKindType {
			if err := validateChildSymbol(&newUS, build, versionToNameToUnitSymbol); err != nil {
				return err
			}
		}
		nts, ok := versionToNameToUnitSymbol[newUS.Version]
		if !ok {
			nts = map[string]*internal.UnitSymbol{}
			versionToNameToUnitSymbol[newUS.Version] = nts
		}
		us, ok := nts[newUS.Name]
		if !ok {
			us = &newUS
			nts[newUS.Name] = us
		}
		us.AddBuildContext(build)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, packagePath, modulePath); err != nil {
		return nil, err
	}
	return versionToNameToUnitSymbol, nil
}

func validateChildSymbol(us *internal.UnitSymbol, build internal.BuildContext,
	versionToNameToUnitSymbol map[string]map[string]*internal.UnitSymbol) error {
	nameToUnitSymbol, ok := versionToNameToUnitSymbol[us.Version]
	if !ok {
		return fmt.Errorf("version %q could not be found: %q", us.Version, us.Name)
	}
	parent, ok := nameToUnitSymbol[us.ParentName]
	if !ok {
		return fmt.Errorf("parent %q could not be found at version %q: %q",
			us.ParentName, us.Version, us.Name)
	}
	if !parent.SupportsBuild(build) {
		return fmt.Errorf("parent %q does not have build %v at version %q: %q",
			us.ParentName, build, us.Version, us.Name)
	}
	return nil
}
