// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
)

func insertSymbols(ctx context.Context, db *database.DB, modulePath, version string,
	pathToID map[string]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (err error) {
	defer derrors.WrapStack(&err, "insertSymbols(ctx, db, %q, %q, pathToID, pathToDocs)", modulePath, version)
	nameToID, err := upsertSymbolNamesReturningIDs(ctx, db, pathToDocIDToDoc)
	if err != nil {
		return err
	}
	pathToPkgsymToID, err := upsertPackageSymbolsReturningIDs(ctx, db, modulePath, pathToID, nameToID, pathToDocIDToDoc)
	if err != nil {
		return err
	}
	if err := upsertDocumentationSymbols(ctx, db, pathToPkgsymToID, pathToDocIDToDoc); err != nil {
		return err
	}
	return upsertSymbolHistory(ctx, db, modulePath, version, nameToID,
		pathToID, pathToPkgsymToID, pathToDocIDToDoc)
}

type packageSymbol struct {
	name     string
	synopsis string

	// parentName is a unique key in packageSymbol because the section can change
	// with the name and synopsis remaining the same. For example:
	// https://pkg.go.dev/go/types@go1.8#Universe is in the Variables section,
	// with parentName Universe.
	// https://pkg.go.dev/go/types@go1.16#Universe is in the Types section,
	// with parentName Scope.
	//
	// https://pkg.go.dev/github.com/89z/page@v1.2.1#Help is in the Types
	// section under type InputMode.
	// https://pkg.go.dev/github.com/89z/page@v1.1.3#Help is in the Types
	// section under ScreenMode.
	parentName string
}

func upsertDocumentationSymbols(ctx context.Context, db *database.DB,
	pathToPkgsymID map[string]map[packageSymbol]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (err error) {
	defer derrors.WrapStack(&err, "upsertDocumentationSymbols(ctx, db, pathToPkgsymID, pathToDocIDToDoc)")

	// Create a map of documentation_id TO package_symbol_id set.
	// This will be used to verify that all package_symbols for the unit have
	// been inserted.
	docIDToPkgsymIDs := map[int]map[int]bool{}
	for path, docIDToDoc := range pathToDocIDToDoc {
		for docID, doc := range docIDToDoc {
			err := updateSymbols(doc.API, func(sm *internal.SymbolMeta) error {
				pkgsymToID, ok := pathToPkgsymID[path]
				if !ok {
					return fmt.Errorf("path could not be found: %q", path)
				}
				ps := packageSymbol{synopsis: sm.Synopsis, name: sm.Name, parentName: sm.ParentName}
				pkgsymID, ok := pkgsymToID[ps]
				if !ok {
					return fmt.Errorf("package symbol could not be found: %v", ps)
				}
				_, ok = docIDToPkgsymIDs[docID]
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
	//
	// Sort first to prevent deadlocks.
	var docIDs []int
	for docID := range docIDToPkgsymIDs {
		docIDs = append(docIDs, docID)
	}
	sort.Ints(docIDs)
	var values []interface{}
	for _, docID := range docIDs {
		gotSet := gotDocIDToPkgsymIDs[docID]
		for pkgsymID := range docIDToPkgsymIDs[docID] {
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
	modulePath string,
	pathToID map[string]int,
	nameToID map[string]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (_ map[string]map[packageSymbol]int, err error) {
	defer derrors.WrapStack(&err, "upsertPackageSymbolsReturningIDs(ctx, db, %q, pathToID, pathToDocIDToDoc)", modulePath)

	idToPath := map[int]string{}
	for path, id := range pathToID {
		idToPath[id] = path
	}
	var names []string
	idToSymbolName := map[int]string{}
	for name, id := range nameToID {
		idToSymbolName[id] = name
		names = append(names, name)
	}

	modulePathID := pathToID[modulePath]
	if modulePathID == 0 {
		return nil, fmt.Errorf("modulePathID cannot be 0: %q", modulePath)
	}
	pathTopkgsymToID := map[string]map[packageSymbol]int{}
	collect := func(rows *sql.Rows) error {
		var (
			id, pathID, symbolID, parentSymbolID int
			synopsis                             string
		)
		if err := rows.Scan(&id, &pathID, &symbolID, &parentSymbolID, &synopsis); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		path := idToPath[pathID]
		if _, ok := pathTopkgsymToID[path]; !ok {
			pathTopkgsymToID[path] = map[packageSymbol]int{}
		}

		sym := idToSymbolName[symbolID]
		if sym == "" {
			return fmt.Errorf("symbol name cannot be empty: %d", symbolID)
		}
		parentSym := idToSymbolName[parentSymbolID]
		if parentSym == "" {
			// A different variable of this symbol was previously inserted.
			// Don't add this to pathTopkgsymToID, since it's not the package
			// symbol that we want.
			// For example:
			// https://dev-pkg.go.dev/github.com/fastly/kingpin@v1.2.6#TokenShort
			// and
			// https://pkg.go.dev/github.com/fastly/kingpin@v1.3.7#TokenShort
			// have the same synopsis, but different parents and sections.
			return nil
		}
		pathTopkgsymToID[path][packageSymbol{
			synopsis:   synopsis,
			name:       sym,
			parentName: parentSym,
		}] = id
		return nil
	}
	// This query fetches more that just the package symbols that we want.
	// The relevant package symbols are filtered above.
	if err := db.RunQuery(ctx, `
        SELECT
            ps.id,
            ps.package_path_id,
            ps.symbol_name_id,
            ps.parent_symbol_name_id,
            ps.synopsis
        FROM package_symbols ps
		INNER JOIN symbol_names s ON ps.symbol_name_id = s.id
        WHERE module_path_id = $1 AND s.name = ANY($2);`, collect, modulePathID, pq.Array(names)); err != nil {
		return nil, err
	}

	// Sort to prevent deadlocks.
	var paths []string
	for path := range pathToDocIDToDoc {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var packageSymbols []interface{}
	for _, path := range paths {
		docs := pathToDocIDToDoc[path]
		pathID := pathToID[path]
		if pathID == 0 {
			return nil, fmt.Errorf("pathID cannot be 0: %q", path)
		}
		for _, doc := range docs {
			if err := updateSymbols(doc.API, func(sm *internal.SymbolMeta) error {
				ps := packageSymbol{synopsis: sm.Synopsis, name: sm.Name, parentName: sm.ParentName}
				symID := nameToID[sm.Name]
				if symID == 0 {
					return fmt.Errorf("symID cannot be 0: %q", sm.Name)
				}
				if sm.ParentName == "" {
					sm.ParentName = sm.Name
				}
				parentID := nameToID[sm.ParentName]
				if parentID == 0 {
					return fmt.Errorf("parentSymID cannot be 0: %q", sm.ParentName)
				}
				if _, ok := pathTopkgsymToID[path][ps]; !ok {
					packageSymbols = append(packageSymbols, pathID,
						modulePathID, symID, parentID, sm.Section, sm.Kind,
						sm.Synopsis)
				}
				return nil
			}); err != nil {
				return nil, err
			}
		}
	}
	// The order of pkgsymcols must match that of the SELECT query in the
	//collect function.
	pkgsymcols := []string{"id", "package_path_id", "symbol_name_id", "parent_symbol_name_id", "synopsis"}
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
			if err := updateSymbols(doc.API, func(sm *internal.SymbolMeta) error {
				names = append(names, sm.Name)
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

	sort.Strings(names)
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
		var (
			sm    internal.SymbolMeta
			build internal.BuildContext
		)
		if err := rows.Scan(
			&sm.Name, &sm.ParentName,
			&sm.Section, &sm.Kind, &sm.Synopsis,
			&build.GOOS, &build.GOARCH); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}

		s := &internal.Symbol{
			SymbolMeta: sm,
			GOOS:       build.GOOS,
			GOARCH:     build.GOARCH,
		}
		switch sm.Section {
		// For symbols that belong to a type, map that symbol as a children of
		// the parent type.
		case internal.SymbolSectionTypes:
			if sm.Kind == internal.SymbolKindType {
				_, ok := buildToNameToType[build]
				if !ok {
					buildToNameToType[build] = map[string]*internal.Symbol{}
				}
				buildToNameToType[build][sm.Name] = s
				buildToSymbols[build] = append(buildToSymbols[build], s)
			} else {
				nameToType, ok := buildToNameToType[build]
				if !ok {
					return fmt.Errorf("build context %v for parent type %q could not be found for symbol %q", build, sm.ParentName, sm.Name)
				}
				parent, ok := nameToType[sm.ParentName]
				if !ok {
					return fmt.Errorf("parent type %q could not be found for symbol %q", sm.ParentName, sm.Name)
				}
				parent.Children = append(parent.Children, &sm)
			}
		default:
			buildToSymbols[build] = append(buildToSymbols[build], s)
		}
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, unitID); err != nil {
		return nil, err
	}
	return buildToSymbols, nil
}

func updateSymbols(symbols []*internal.Symbol, updateFunc func(sm *internal.SymbolMeta) error) error {
	for _, s := range symbols {
		if err := updateFunc(&s.SymbolMeta); err != nil {
			return err
		}
		for _, c := range s.Children {
			if err := updateFunc(c); err != nil {
				return err
			}
		}
	}
	return nil
}
