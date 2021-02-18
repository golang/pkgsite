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
	pathToID map[string]int, pathToDocs map[string][]*internal.Documentation) (err error) {
	defer derrors.WrapStack(&err, "insertSymbols(ctx, db, %q, %q, pathToID, pathToDocs)", modulePath, version)
	if !experiment.IsActive(ctx, internal.ExperimentInsertSymbolHistory) {
		return nil
	}

	symToID, err := upsertSymbolsReturningIDs(ctx, db, pathToDocs)
	if err != nil {
		return err
	}
	var symHistoryValues []interface{}
	for path, docs := range pathToDocs {
		buildToNameToSym, err := getSymbolHistory(ctx, db, path, modulePath)
		if err != nil {
			return err
		}
		for _, doc := range docs {
			nameToSymbol := buildToNameToSym[goosgoarch(doc.GOOS, doc.GOARCH)]
			for _, s := range doc.API {
				symHistoryValues, err = appendSymbolHistoryRow(s, symHistoryValues, path, modulePath, version,
					pathToID, symToID, nameToSymbol)
				if err != nil {
					return err
				}

				for _, s := range s.Children {
					symHistoryValues, err = appendSymbolHistoryRow(s, symHistoryValues, path, modulePath, version,
						pathToID, symToID, nameToSymbol)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	uniqueSymCols := []string{"package_path_id", "module_path_id", "symbol_id", "goos", "goarch"}
	symCols := append(uniqueSymCols, "type", "parent_symbol_id", "since_version", "section", "synopsis")
	return db.BulkUpsert(ctx, "symbol_history", symCols, symHistoryValues, uniqueSymCols)
}

func appendSymbolHistoryRow(s *internal.Symbol, values []interface{},
	packagePath, modulePath, version string,
	pathToID, symToID map[string]int,
	dbHist map[string]*internal.Symbol) (_ []interface{}, err error) {
	defer derrors.WrapStack(&err, "symbolHistoryRow(%q, %q, %q, %q)", s.Name, packagePath, modulePath, version)
	if !shouldUpdateSymbolHistory(s.Name, version, dbHist) {
		return values, nil
	}

	symbolID := symToID[s.Name]
	if symbolID == 0 {
		return nil, fmt.Errorf("symbolID cannot be 0: %q", s.Name)
	}
	if s.ParentName == "" {
		s.ParentName = s.Name
	}
	parentID := symToID[s.ParentName]
	if parentID == 0 {
		return nil, fmt.Errorf("parentSymbolID cannot be 0: %q", s.ParentName)
	}
	packagePathID := pathToID[packagePath]
	if packagePathID == 0 {
		return nil, fmt.Errorf("packagePathID cannot be 0: %q", packagePathID)
	}
	modulePathID := pathToID[modulePath]
	if modulePathID == 0 {
		return nil, fmt.Errorf("modulePathID cannot be 0: %q", modulePathID)
	}
	return append(values,
		packagePathID, modulePathID, symbolID, s.GOOS, s.GOARCH, s.Kind, parentID,
		version, s.Section, s.Synopsis), nil
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

func upsertSymbolsReturningIDs(ctx context.Context, db *database.DB, pathToDocs map[string][]*internal.Documentation) (map[string]int, error) {
	var values []interface{}
	for _, docs := range pathToDocs {
		for _, doc := range docs {
			for _, s := range doc.API {
				values = append(values, s.Name)
				if len(s.Children) > 0 {
					for _, s := range s.Children {
						values = append(values, s.Name)
					}
				}
			}
		}
	}

	if err := db.BulkInsert(ctx, "symbols", []string{"name"}, values, database.OnConflictDoNothing); err != nil {
		return nil, err
	}
	query := `
        SELECT id, name
        FROM symbols
        WHERE name = ANY($1);`
	symbols := map[string]int{}
	collect := func(rows *sql.Rows) error {
		var name string
		var id int
		if err := rows.Scan(&id, &name); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		symbols[name] = id
		if id == 0 {
			return fmt.Errorf("id can't be 0: %q", name)
		}
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, pq.Array(values)); err != nil {
		return nil, err
	}
	return symbols, nil
}

func getSymbolHistory(ctx context.Context, db *database.DB, packagePath, modulePath string) (_ map[string]map[string]*internal.Symbol, err error) {
	defer derrors.Wrap(&err, "getSymbolHistoryForPath(ctx, db, %q, %q)", packagePath, modulePath)
	query := `
        SELECT
            s1.name AS symbol_name,
            s2.name AS parent_symbol_name,
            sh.since_version,
            sh.section,
            sh.type,
            sh.synopsis,
            sh.goos,
            sh.goarch
        FROM symbol_history sh
        INNER JOIN paths p1 ON sh.package_path_id = p1.id
        INNER JOIN paths p2 ON sh.module_path_id = p2.id
        INNER JOIN symbols s1 ON sh.symbol_id = s1.id
        INNER JOIN symbols s2 ON sh.parent_symbol_id = s2.id
        WHERE p1.path = $1 AND p2.path = $2;`

	// Map from GOOS/GOARCH to (map from symbol name to symbol).
	buildToNameToSym := map[string]map[string]*internal.Symbol{}
	collect := func(rows *sql.Rows) error {
		var (
			sh     internal.Symbol
			goos   string
			goarch string
		)
		if err := rows.Scan(
			&sh.Name,
			&sh.ParentName,
			&sh.SinceVersion,
			&sh.Section,
			&sh.Kind,
			&sh.Synopsis,
			&goos,
			&goarch,
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		nameToSym, ok := buildToNameToSym[goosgoarch(goos, goarch)]
		if !ok {
			nameToSym = map[string]*internal.Symbol{}
			buildToNameToSym[goosgoarch(goos, goarch)] = nameToSym
		}
		nameToSym[sh.Name] = &sh
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, packagePath, modulePath); err != nil {
		return nil, err
	}
	return buildToNameToSym, nil
}

func goosgoarch(goos, goarch string) string {
	return fmt.Sprintf("goos=%s_goarch=%s", goos, goarch)
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
		data := hist[goosgoarch("linux", "amd64")]
		errs := symbol.CompareStdLib(path, apiVersions[path], data)
		if len(errs) > 0 {
			pkgToErrors[path] = errs
		}
	}
	return pkgToErrors, nil
}
