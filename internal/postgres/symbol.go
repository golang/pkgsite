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
	pkgsymToID, err := upsertPackageSymbolsReturningIDs(ctx, db, modulePath, pathToID, pathToDocs)
	if err != nil {
		return err
	}

	var (
		uniqueKeys       = map[string]string{}
		symHistoryValues []interface{}
	)
	for path, docs := range pathToDocs {
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
				nameToSymbol := buildToNameToSym[goosgoarch(build.GOOS, build.GOARCH)]
				updateSymbols(doc.API, func(s *internal.Symbol) (err error) {
					defer derrors.WrapStack(&err, "updateSymbols(%q)", s.Name)
					if !shouldUpdateSymbolHistory(s.Name, version, nameToSymbol) {
						return nil
					}
					pkgsymID := pkgsymToID[packageSymbolKey(s.Section, s.Synopsis)]
					if pkgsymID == 0 {
						return fmt.Errorf("symbolID cannot be 0: %q", s.Name)
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
				})
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

func upsertPackageSymbolsReturningIDs(ctx context.Context, db *database.DB,
	modulePath string, pathToID map[string]int, pathToDocs map[string][]*internal.Documentation) (_ map[string]int, err error) {
	defer derrors.WrapStack(&err, "upsertPackageSymbolsReturningIDs(ctx, db, %q, pathToID, pathToDocs)", modulePath)
	nameToID, err := upsertSymbolNamesReturningIDs(ctx, db, pathToDocs)
	if err != nil {
		return nil, err
	}

	var values []interface{}
	modulePathID := pathToID[modulePath]
	if modulePathID == 0 {
		return nil, fmt.Errorf("modulePathID cannot be 0: %q", modulePath)
	}
	for path, docs := range pathToDocs {
		pathID := pathToID[path]
		for _, doc := range docs {
			updateSymbols(doc.API, func(s *internal.Symbol) error {
				if s.ParentName == "" {
					s.ParentName = s.Name
				}
				values = append(values, pathID, modulePathID, nameToID[s.Name], nameToID[s.ParentName], s.Section, s.Kind, s.Synopsis)
				return nil
			})
		}
	}
	if err := db.BulkInsert(ctx, "package_symbols",
		[]string{
			"package_path_id",
			"module_path_id",
			"symbol_name_id",
			"parent_symbol_name_id",
			"section",
			"type",
			"synopsis",
		}, values, database.OnConflictDoNothing); err != nil {
		return nil, err
	}

	query := `
        SELECT
            ps.id,
            ps.section,
            ps.synopsis
        FROM package_symbols ps
        INNER JOIN symbol_names sn
        ON ps.symbol_name_id = sn.id
        WHERE module_path_id = $1;`
	pkgsymToID := map[string]int{}
	collect := func(rows *sql.Rows) error {
		var (
			id       int
			section  internal.SymbolSection
			synopsis string
		)
		if err := rows.Scan(&id, &section, &synopsis); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		pkgsymToID[packageSymbolKey(section, synopsis)] = id
		return nil
	}
	if err := db.RunQuery(ctx, query, collect, modulePathID); err != nil {
		return nil, err
	}
	for _, docs := range pathToDocs {
		for _, doc := range docs {
			updateSymbols(doc.API, func(s *internal.Symbol) error {
				if _, ok := pkgsymToID[packageSymbolKey(s.Section, s.Synopsis)]; !ok {
					return fmt.Errorf("missing package symbol for %q %q (section=%q, type=%q)", s.Name, s.Synopsis, s.Section, s.Kind)
				}
				return nil
			})
		}
	}
	return pkgsymToID, nil
}

func packageSymbolKey(section internal.SymbolSection, synopsis string) string {
	return fmt.Sprintf("section=%s_synopsis=%s", section, synopsis)
}

func upsertSymbolNamesReturningIDs(ctx context.Context, db *database.DB, pathToDocs map[string][]*internal.Documentation) (_ map[string]int, err error) {
	defer derrors.WrapStack(&err, "upsertSymbolNamesReturningIDs")
	var values []interface{}
	for _, docs := range pathToDocs {
		for _, doc := range docs {
			updateSymbols(doc.API, func(s *internal.Symbol) error {
				values = append(values, s.Name)
				return nil
			})
		}
	}

	if err := db.BulkInsert(ctx, "symbol_names", []string{"name"}, values, database.OnConflictDoNothing); err != nil {
		return nil, err
	}
	query := `
        SELECT id, name
        FROM symbol_names
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
	buildToNameToSym := map[string]map[string]*internal.Symbol{}
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
		nameToSym, ok := buildToNameToSym[goosgoarch(sh.GOOS, sh.GOARCH)]
		if !ok {
			nameToSym = map[string]*internal.Symbol{}
			buildToNameToSym[goosgoarch(sh.GOOS, sh.GOARCH)] = nameToSym
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
		data := hist[goosgoarch("linux", "amd64")]
		errs := symbol.CompareStdLib(path, apiVersions[path], data)
		if len(errs) > 0 {
			pkgToErrors[path] = errs
		}
	}
	return pkgToErrors, nil
}
