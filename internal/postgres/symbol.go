// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/version"
)

func insertSymbols(ctx context.Context, tx *database.DB, modulePath, v string,
	isLatest bool,
	pathToID map[string]int,
	pathToUnitID map[string]int,
	pathToDocs map[string][]*internal.Documentation) (err error) {
	defer derrors.WrapStack(&err, "insertSymbols(ctx, db, %q, %q, pathToID, pathToDocs)", modulePath, v)

	// Only update symbol history if the version type is release.
	versionType, err := version.ParseType(v)
	if err != nil {
		return err
	}
	if versionType != version.TypeRelease && !isLatest {
		return nil
	}
	modulePathID := pathToID[modulePath]
	if modulePathID == 0 {
		return fmt.Errorf("modulePathID cannot be 0: %q", modulePath)
	}
	pathToDocIDToDoc, err := getDocIDsForPath(ctx, tx, pathToUnitID, pathToDocs)
	if err != nil {
		return err
	}
	nameToID, err := upsertSymbolNamesReturningIDs(ctx, tx, pathToDocIDToDoc)
	if err != nil {
		return err
	}
	pathToPkgsymToID, err := upsertPackageSymbolsReturningIDs(ctx, tx, modulePathID, pathToID, nameToID, pathToDocIDToDoc)
	if err != nil {
		return err
	}
	if err := upsertDocumentationSymbols(ctx, tx, pathToPkgsymToID, pathToDocIDToDoc); err != nil {
		return err
	}
	if versionType == version.TypeRelease {
		if err := upsertSymbolHistory(ctx, tx, modulePath, v, nameToID,
			pathToID, pathToPkgsymToID, pathToDocIDToDoc); err != nil {
			return err
		}
	}
	if isLatest {
		return deleteOldSymbolSearchDocuments(ctx, tx, modulePathID, pathToID, pathToDocIDToDoc, pathToPkgsymToID)
	}
	return nil
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
			// The package_symbol_id in the documentation_symbols table does
			// not match the one we want to insert. This can happen if we
			// change the package_symbol_id. In that case, do not add this to
			// the map, so that we can upsert below.
			//
			// See https://go-review.googlesource.com/c/pkgsite/+/315309
			// and https://go-review.googlesource.com/c/pkgsite/+/315310
			// where the package_symbol_id was potentially changed.
			return nil
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
	// Upsert the rows.
	// Note that the order of pkgsymcols must match that of the SELECT query in
	// the collect function.
	docsymcols := []string{"documentation_id", "package_symbol_id"}
	if err := db.BulkInsert(ctx, "documentation_symbols", docsymcols,
		values, `
			ON CONFLICT (documentation_id, package_symbol_id)
			DO UPDATE SET
				documentation_id=excluded.documentation_id,
				package_symbol_id=excluded.package_symbol_id`); err != nil {
		return err
	}
	return nil
}

func upsertPackageSymbolsReturningIDs(ctx context.Context, db *database.DB,
	modulePathID int,
	pathToID map[string]int,
	nameToID map[string]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation) (_ map[string]map[packageSymbol]int, err error) {
	defer derrors.WrapStack(&err, "upsertPackageSymbolsReturningIDs(ctx, db, %d, pathToID, pathToDocIDToDoc)", modulePathID)

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
		parentSym, ok := idToSymbolName[parentSymbolID]
		if !ok {
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
			// Sort to prevent deadlocks.
			sort.Slice(doc.API, func(i, j int) bool {
				return doc.API[i].Name < doc.API[j].Name
			})

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
	sort.Strings(names)

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
	query := `
		SELECT id, name
		FROM symbol_names
		WHERE name = ANY($1);`
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

func deleteOldSymbolSearchDocuments(ctx context.Context, db *database.DB,
	modulePathID int,
	pathToID map[string]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation,
	latestPathToPkgsymToID map[string]map[packageSymbol]int) (err error) {
	defer derrors.WrapStack(&err, "deleteOldSymbolSearchDocuments(ctx, db, %q, pathToID, pathToDocIDToDoc)", modulePathID)

	// Get all package_symbol_ids for the latest module (the current one we are
	// trying to insert).
	latestPkgsymIDs := map[int]bool{}
	for path := range pathToID {
		docs := pathToDocIDToDoc[path]
		pathID := pathToID[path]
		if pathID == 0 {
			return fmt.Errorf("pathID cannot be 0: %q", path)
		}
		for _, doc := range docs {
			err := updateSymbols(doc.API, func(sm *internal.SymbolMeta) error {
				pkgsymToID, ok := latestPathToPkgsymToID[path]
				if !ok {
					return fmt.Errorf("path could not be found: %q", path)
				}
				ps := packageSymbol{synopsis: sm.Synopsis, name: sm.Name, parentName: sm.ParentName}
				pkgsymID, ok := pkgsymToID[ps]
				if !ok {
					return fmt.Errorf("package symbol could not be found: %v", ps)
				}
				latestPkgsymIDs[pkgsymID] = true
				return nil
			})
			if err != nil {
				return err
			}
		}
	}

	var pathIDs []int
	for _, id := range pathToID {
		pathIDs = append(pathIDs, id)
	}
	// Fetch package_symbol_id currently in symbol_search_documents.
	dbPkgSymIDs, err := database.Collect1[int](ctx, db, `
		SELECT package_symbol_id
		FROM symbol_search_documents
		WHERE package_path_id = ANY($1);`,
		pq.Array(pathIDs))
	if err != nil {
		return err
	}

	var toDelete []int
	for _, id := range dbPkgSymIDs {
		if _, ok := latestPkgsymIDs[id]; !ok {
			toDelete = append(toDelete, id)
		}
	}

	// Delete stale rows.
	q, args, err := squirrel.Delete("symbol_search_documents").
		Where("package_symbol_id = ANY(?)", pq.Array(toDelete)).
		PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return err
	}
	n, err := db.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	log.Infof(ctx, "deleted %d rows from symbol_search_documents", n)
	return nil
}
