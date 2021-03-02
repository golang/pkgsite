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
	if !experiment.IsActive(ctx, internal.ExperimentInsertSymbols) {
		return nil
	}
	pathToPkgsymToID, err := upsertPackageSymbolsReturningIDs(ctx, db, modulePath, pathToID, pathToDocIDToDoc)
	if err != nil {
		return err
	}
	if err := upsertDocumentationSymbols(ctx, db, pathToPkgsymToID, pathToDocIDToDoc); err != nil {
		return err
	}

	if !experiment.IsActive(ctx, internal.ExperimentInsertSymbolHistory) {
		return nil
	}
	var (
		uniqueKeys       = map[string]internal.Symbol{}
		symHistoryValues []interface{}
	)
	for path, docIDToDoc := range pathToDocIDToDoc {
		buildToNameToSym, err := getSymbolHistory(ctx, db, path, modulePath)
		if err != nil {
			return err
		}
		for _, doc := range docIDToDoc {
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
					pkgsymID := pathToPkgsymToID[path][pkgsym]
					if pkgsymID == 0 {
						return fmt.Errorf("pkgsymID cannot be 0: %q", pkgsym)
					}

					// Validate that the unique constraint won't be violated.
					// It is easier to debug when returning an error here as
					// opposed to from the BulkUpsert statement.
					key := fmt.Sprintf("%d-%s-%s", pkgsymID, build.GOOS, build.GOARCH)
					if val, ok := uniqueKeys[key]; ok {
						return fmt.Errorf("DB package symbol %q already exists: %v; failed to insert symbol %q (%v) q with the same (package_symbol_id, goos, goarch)", key, val, s.Name, s)
					}
					uniqueKeys[key] = *s
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

func upsertDocumentationSymbols(ctx context.Context, db *database.DB,
	pathToPkgsymID map[string]map[packageSymbol]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (err error) {

	// Create a map of documentation_id TO package_symbol_id set.
	// This will be used to verify that all package_symbols for the unit have
	// been inserted.
	docIDToPkgsymIDs := map[int]map[int]bool{}
	for path, docIDToDoc := range pathToDocIDToDoc {
		for docID, doc := range docIDToDoc {
			err := updateSymbols(doc.API, func(s *internal.Symbol) error {
				pkgsymToID := pathToPkgsymID[path]
				pkgsymID := pkgsymToID[packageSymbol{synopsis: s.Synopsis, section: s.Section}]
				_, ok := docIDToPkgsymIDs[docID]
				if !ok {
					docIDToPkgsymIDs[docID] = map[int]bool{}
				}
				docIDToPkgsymIDs[docID][pkgsymID] = true
				return nil
			})
			if err != nil {
				return err
			}
		}
	}

	// Fetch all existing rows in documentation_symbols for this unit using the
	// documentation IDs.
	// Keep track of which rows already exist in documentation_symbols using
	// gotDocIDToPkgsymIDs.
	var documentationIDs []interface{}
	for docID := range docIDToPkgsymIDs {
		documentationIDs = append(documentationIDs, docID)
	}
	gotDocIDToPkgsymIDs := map[int]map[int]bool{}
	collect := func(rows *sql.Rows) error {
		var id, docID, pkgsymID int
		if err := rows.Scan(&id, &docID, &pkgsymID); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		if !docIDToPkgsymIDs[docID][pkgsymID] {
			return fmt.Errorf("unexpected pkgsymID %d for docID %d", pkgsymID, docID)
		}
		if _, ok := gotDocIDToPkgsymIDs[docID]; !ok {
			gotDocIDToPkgsymIDs[docID] = map[int]bool{}
		}
		gotDocIDToPkgsymIDs[docID][pkgsymID] = true
		return nil
	}
	if err := db.RunQuery(ctx, `
        SELECT
            ds.id,
            ds.documentation_id,
            ds.package_symbol_id
        FROM documentation_symbols ds
        WHERE documentation_id = ANY($1);`, collect, pq.Array(documentationIDs)); err != nil {
		return err
	}

	// Get the difference between the documentation_symbols for this package,
	// and the ones that already exist in the documentation_symbols table. Only
	// insert rows that do not already exist.
	var values []interface{}
	for docID, pkgsymIDSet := range docIDToPkgsymIDs {
		gotSet := gotDocIDToPkgsymIDs[docID]
		for pkgsymID := range pkgsymIDSet {
			if !gotSet[pkgsymID] {
				values = append(values, docID, pkgsymID)
			}
		}
	}
	// Insert the rows.
	// Note that the order of pkgsymcols must match that of the SELECT query in
	// the collect function.
	docsymcols := []string{"documentation_id", "package_symbol_id"}
	if err := db.BulkInsert(ctx, "documentation_symbols", docsymcols,
		values, database.OnConflictDoNothing); err != nil {
		return err
	}
	return nil
}

func upsertPackageSymbolsReturningIDs(ctx context.Context, db *database.DB,
	modulePath string, pathToID map[string]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (_ map[string]map[packageSymbol]int, err error) {
	defer derrors.WrapStack(&err, "upsertPackageSymbolsReturningIDs(ctx, db, %q, pathToID, pathToDocIDToDoc)", modulePath)
	nameToID, err := upsertSymbolNamesReturningIDs(ctx, db, pathToDocIDToDoc)
	if err != nil {
		return nil, err
	}

	idToPath := map[int]string{}
	for path, id := range pathToID {
		idToPath[id] = path
	}

	modulePathID := pathToID[modulePath]
	if modulePathID == 0 {
		return nil, fmt.Errorf("modulePathID cannot be 0: %q", modulePath)
	}
	pathTopkgsymToID := map[string]map[packageSymbol]int{}
	collect := func(rows *sql.Rows) error {
		var (
			id, pathID int
			section    internal.SymbolSection
			synopsis   string
		)
		if err := rows.Scan(&id, &pathID, &section, &synopsis); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		path := idToPath[pathID]
		if _, ok := pathTopkgsymToID[path]; !ok {
			pathTopkgsymToID[path] = map[packageSymbol]int{}
		}
		pathTopkgsymToID[path][packageSymbol{synopsis: synopsis, section: section}] = id
		return nil
	}
	if err := db.RunQuery(ctx, `
        SELECT
            ps.id,
            ps.package_path_id,
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
				if _, ok := pathTopkgsymToID[path][ps]; !ok {
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
	pkgsymcols := []string{"id", "package_path_id", "section", "synopsis"}
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
	return pathTopkgsymToID, nil
}

func upsertSymbolNamesReturningIDs(ctx context.Context, db *database.DB,
	pathToDocIDToDocs map[string]map[int]*internal.Documentation) (_ map[string]int, err error) {
	defer derrors.WrapStack(&err, "upsertSymbolNamesReturningIDs")
	var names []string
	for _, docIDToDocs := range pathToDocIDToDocs {
		for _, doc := range docIDToDocs {
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

// getUnitSymbols returns all of the symbols for the given unitID.
func getUnitSymbols(ctx context.Context, db *database.DB, unitID int) (_ map[internal.BuildContext][]*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "getUnitSymbols(ctx, db, %d)", unitID)

	// Fetch all symbols for the unit. Order by symbol_type "Type" first, so
	// that when we collect the children the structs for these symbols will
	// already be created.
	query := `
        SELECT
            s1.name AS symbol_name,
            s2.name AS parent_symbol_name,
            ps.section,
            ps.type,
            ps.synopsis,
            d.goos,
            d.goarch
        FROM documentation_symbols ds
        INNER JOIN documentation d ON d.id = ds.documentation_id
        INNER JOIN package_symbols ps ON ds.package_symbol_id = ps.id
        INNER JOIN symbol_names s1 ON ps.symbol_name_id = s1.id
        INNER JOIN symbol_names s2 ON ps.parent_symbol_name_id = s2.id
        WHERE d.unit_id = $1
        ORDER BY CASE WHEN ps.type='Type' THEN 0 ELSE 1 END;`
	// buildToSymbols contains all of the symbols for this unit, grouped by
	// build context.
	buildToSymbols := map[internal.BuildContext][]*internal.Symbol{}
	// buildToNameToType contains all of the types for this unit, grouped by
	// name and build context. This is used to keep track of the parent types,
	// so that we can map the children to those symbols.
	buildToNameToType := map[internal.BuildContext]map[string]*internal.Symbol{}
	collect := func(rows *sql.Rows) error {
		var s internal.Symbol
		if err := rows.Scan(
			&s.Name, &s.ParentName,
			&s.Section, &s.Kind, &s.Synopsis,
			&s.GOOS, &s.GOARCH); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		build := internal.BuildContext{GOOS: s.GOOS, GOARCH: s.GOARCH}
		switch s.Section {
		// For symbols that belong to a type, map that symbol as a children of
		// the parent type.
		case internal.SymbolSectionTypes:
			if s.Kind == internal.SymbolKindType {
				_, ok := buildToNameToType[build]
				if !ok {
					buildToNameToType[build] = map[string]*internal.Symbol{}
				}
				buildToNameToType[build][s.Name] = &s
				buildToSymbols[build] = append(buildToSymbols[build], &s)
			} else {
				nameToType, ok := buildToNameToType[build]
				if !ok {
					return fmt.Errorf("build context %v for parent type %q could not be found for symbol %q", build, s.ParentName, s.Name)
				}
				parent, ok := nameToType[s.ParentName]
				if !ok {
					return fmt.Errorf("parent type %q could not be found for symbol %q", s.ParentName, s.Name)
				}
				parent.Children = append(parent.Children, &s)
			}
		default:
			buildToSymbols[build] = append(buildToSymbols[build], &s)
		}
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, unitID); err != nil {
		return nil, err
	}
	return buildToSymbols, nil
}

func getSymbolHistory(ctx context.Context, db *database.DB, packagePath, modulePath string) (_ map[internal.BuildContext]map[string]*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "getSymbolHistory(ctx, db, %q, %q)", packagePath, modulePath)
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
		versionToNameToSymbol, err := db.GetPackageSymbols(ctx, path, stdlib.ModulePath)
		if err != nil {
			return nil, err
		}
		errs := symbol.CompareStdLib(path, apiVersions[path], versionToNameToSymbol)
		if len(errs) > 0 {
			pkgToErrors[path] = errs
		}
	}
	return pkgToErrors, nil
}
