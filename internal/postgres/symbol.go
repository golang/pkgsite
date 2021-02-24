// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/symbol"
)

func insertSymbols(ctx context.Context, db *database.DB, modulePath, version string,
	pathToID map[string]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (err error) {
	defer derrors.WrapStack(&err, "insertSymbols(ctx, db, %q, %q, pathToID, pathToDocs)", modulePath, version)
	if !experiment.IsActive(ctx, internal.ExperimentInsertSymbolHistory) {
		return nil
	}
	pkgsymToID, err := upsertPackageSymbolsReturningIDs(ctx, db, modulePath, pathToID, pathToDocIDToDoc)
	if err != nil {
		return err
	}

	var (
		uniqueKeys       = map[string]string{}
		symHistoryValues []interface{}
	)
	for path, docs := range pathToDocIDToDoc {
		buildToNameToSym, err := getSymbolHistory(ctx, db, path, modulePath)
		if err != nil {
			return err
		}
		for _, doc := range docs {
			builds := []internal.BuildContext{{GOOS: doc.GOOS, GOARCH: doc.GOARCH}}
			if doc.GOOS == internal.All {
				builds = internal.BuildContexts
			}
			for _, build := range builds {
				nameToSymbol := buildToNameToSym[internal.BuildContext{GOOS: build.GOOS, GOARCH: build.GOARCH}]
				if err := updateSymbols(doc.API, func(s *internal.Symbol) (err error) {
					defer derrors.WrapStack(&err, "updateSymbols(%q)", s.Name)
					if !shouldUpdateSymbolHistory(s.Name, version, nameToSymbol) {
						return nil
					}

					pkgsym := packageSymbol{synopsis: s.Synopsis, section: s.Section}
					pkgsymID := pkgsymToID[pkgsym]
					if pkgsymID == 0 {
						return fmt.Errorf("pkgsymID cannot be 0: %q", pkgsym)
					}

					// Validate that the unique constraint won't be violated.
					// It is easier to debug when returning an error here as
					// opposed to from the BulkUpsert statement.
					key := fmt.Sprintf("%d-%s-%s", pkgsymID, build.GOOS, build.GOARCH)
					if val, ok := uniqueKeys[key]; ok {
						return fmt.Errorf("symbol %q exists at %q -- failed to insert symbol %q (%q) q with the same (package_symbol_id, goos, goarch)", key, val, s.Name, s.Synopsis)
					}
					uniqueKeys[key] = fmt.Sprintf("%q (%q)", s.Name, s.Synopsis)
					symHistoryValues = append(symHistoryValues, pkgsymID, build.GOOS, build.GOARCH, version)
					return nil
				}); err != nil {
					return err
				}
			}
		}
	}
	uniqueSymCols := []string{"package_symbol_id", "goos", "goarch"}
	symCols := append(uniqueSymCols, "since_version")
	return db.BulkUpsert(ctx, "symbol_history", symCols, symHistoryValues, uniqueSymCols)
}

// shouldUpdateSymbolHistory reports whether the row for the given symbolName
// should be updated. oldHist contains all of the current symbols in the
// database for the same package and GOOS/GOARCH.
//
// shouldUpdateSymbolHistory reports true if the symbolName does not currently
// exist, or if the newVersion is older than or equal to the current database version.
func shouldUpdateSymbolHistory(symbolName, newVersion string, oldHist map[string]*internal.Symbol) bool {
	dh, ok := oldHist[symbolName]
	if !ok {
		return true
	}
	return semver.Compare(newVersion, dh.SinceVersion) < 1
}

type packageSymbol struct {
	synopsis string
	section  internal.SymbolSection
}

func upsertPackageSymbolsReturningIDs(ctx context.Context, db *database.DB,
	modulePath string, pathToID map[string]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (_ map[packageSymbol]int, err error) {
	defer derrors.WrapStack(&err, "upsertPackageSymbolsReturningIDs(ctx, db, %q, pathToID, pathToDocs)", modulePath)
	nameToID, err := upsertSymbolNamesReturningIDs(ctx, db, pathToDocIDToDoc)
	if err != nil {
		return nil, err
	}

	modulePathID := pathToID[modulePath]
	if modulePathID == 0 {
		return nil, fmt.Errorf("modulePathID cannot be 0: %q", modulePath)
	}
	pkgsymToID := map[packageSymbol]int{}
	collect := func(rows *sql.Rows) error {
		var (
			id       int
			section  internal.SymbolSection
			synopsis string
		)
		if err := rows.Scan(&id, &section, &synopsis); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		pkgsymToID[packageSymbol{synopsis: synopsis, section: section}] = id
		return nil
	}
	if err := db.RunQuery(ctx, `
        SELECT
            ps.id,
            ps.section,
            ps.synopsis
        FROM package_symbols ps
        INNER JOIN symbol_names sn ON ps.symbol_name_id = sn.id
        WHERE module_path_id = $1;`, collect, modulePathID); err != nil {
		return nil, err
	}

	var packageSymbols []interface{}
	for path, docs := range pathToDocIDToDoc {
		pathID := pathToID[path]
		if pathID == 0 {
			return nil, fmt.Errorf("pathID cannot be 0: %q", path)
		}
		for _, doc := range docs {
			if err := updateSymbols(doc.API, func(s *internal.Symbol) error {
				ps := packageSymbol{synopsis: s.Synopsis, section: s.Section}
				symID := nameToID[s.Name]
				if symID == 0 {
					return fmt.Errorf("pathID cannot be 0: %q", s.Name)
				}
				if s.ParentName == "" {
					s.ParentName = s.Name
				}
				parentID := nameToID[s.ParentName]
				if parentID == 0 {
					return fmt.Errorf("pathID cannot be 0: %q", s.ParentName)
				}
				if _, ok := pkgsymToID[ps]; !ok {
					packageSymbols = append(packageSymbols, pathID,
						modulePathID, symID, parentID, s.Section, s.Kind,
						s.Synopsis)
				}
				return nil
			}); err != nil {
				return nil, err
			}
		}
	}
	// The order of pkgsymcols must match that of the SELECT query in the
	//collect function.
	pkgsymcols := []string{"id", "section", "synopsis"}
	if err := db.BulkInsertReturning(ctx, "package_symbols",
		[]string{
			"package_path_id",
			"module_path_id",
			"symbol_name_id",
			"parent_symbol_name_id",
			"section",
			"type",
			"synopsis",
		}, packageSymbols, database.OnConflictDoNothing, pkgsymcols, collect); err != nil {
		return nil, err
	}
	return pkgsymToID, nil
}

func upsertSymbolNamesReturningIDs(ctx context.Context, db *database.DB,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (_ map[string]int, err error) {
	defer derrors.WrapStack(&err, "upsertSymbolNamesReturningIDs")
	var names []string
	for _, docs := range pathToDocIDToDoc {
		for _, doc := range docs {
			if err := updateSymbols(doc.API, func(s *internal.Symbol) error {
				names = append(names, s.Name)
				return nil
			}); err != nil {
				return nil, err
			}
		}
	}
	query := `
        SELECT id, name
        FROM symbol_names
        WHERE name = ANY($1);`
	nameToID := map[string]int{}
	collect := func(rows *sql.Rows) error {
		var (
			id   int
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		nameToID[name] = id
		if id == 0 {
			return fmt.Errorf("id can't be 0: %q", name)
		}
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, pq.Array(names)); err != nil {
		return nil, err
	}

	var values []interface{}
	for _, name := range names {
		if _, ok := nameToID[name]; !ok {
			values = append(values, name)
		}
	}
	if err := db.BulkInsertReturning(ctx, "symbol_names", []string{"name"},
		values, database.OnConflictDoNothing, []string{"id", "name"}, collect); err != nil {
		return nil, err
	}
	return nameToID, nil
}

func getSymbolHistory(ctx context.Context, db *database.DB, packagePath, modulePath string) (_ map[internal.BuildContext]map[string]*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "getSymbolHistoryForPath(ctx, db, %q, %q)", packagePath, modulePath)
	query := `
        SELECT
            s1.name AS symbol_name,
            s2.name AS parent_symbol_name,
            ps.section,
            ps.type,
            ps.synopsis,
            sh.since_version,
            sh.goos,
            sh.goarch
        FROM symbol_history sh
        INNER JOIN package_symbols ps ON sh.package_symbol_id = ps.id
        INNER JOIN symbol_names s1 ON ps.symbol_name_id = s1.id
        INNER JOIN symbol_names s2 ON ps.parent_symbol_name_id = s2.id
        INNER JOIN paths p1 ON ps.package_path_id = p1.id
        INNER JOIN paths p2 ON ps.module_path_id = p2.id
        WHERE p1.path = $1 AND p2.path = $2;`

	// Map from GOOS/GOARCH to (map from symbol name to symbol).
	buildToNameToSym := map[internal.BuildContext]map[string]*internal.Symbol{}
	collect := func(rows *sql.Rows) error {
		var (
			sh internal.Symbol
		)
		if err := rows.Scan(
			&sh.Name,
			&sh.ParentName,
			&sh.Section,
			&sh.Kind,
			&sh.Synopsis,
			&sh.SinceVersion,
			&sh.GOOS,
			&sh.GOARCH,
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		nameToSym, ok := buildToNameToSym[internal.BuildContext{GOOS: sh.GOOS, GOARCH: sh.GOARCH}]
		if !ok {
			nameToSym = map[string]*internal.Symbol{}
			buildToNameToSym[internal.BuildContext{GOOS: sh.GOOS, GOARCH: sh.GOARCH}] = nameToSym
		}
		nameToSym[sh.Name] = &sh
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, packagePath, modulePath); err != nil {
		return nil, err
	}
	return buildToNameToSym, nil
}

func updateSymbols(symbols []*internal.Symbol, updateFunc func(s *internal.Symbol) error) error {
	for _, s := range symbols {
		if err := updateFunc(s); err != nil {
			return err
		}
		for _, s := range s.Children {
			if err := updateFunc(s); err != nil {
				return err
			}
		}
	}
	return nil
}

// CompareStdLib is a helper function for comparing the output of
// getSymbolHistory and symbol.ParsePackageAPIInfo. This is only meant for use
// locally for testing purposes.
func (db *DB) CompareStdLib(ctx context.Context) (map[string][]string, error) {
	apiVersions, err := symbol.ParsePackageAPIInfo()
	if err != nil {
		return nil, err
	}
	pkgToErrors := map[string][]string{}
	for path := range apiVersions {
		hist, err := getSymbolHistory(ctx, db.db, path, stdlib.ModulePath)
		if err != nil {
			return nil, err
		}
		// symbol.ParsePackageAPIInfo does not support OS/ARCH-dependent symbols.
		data := hist[internal.BuildContext{GOOS: "linux", GOARCH: "amd64"}]
		errs := symbol.CompareStdLib(path, apiVersions[path], data)
		if len(errs) > 0 {
			pkgToErrors[path] = errs
		}
	}
	return pkgToErrors, nil
}
