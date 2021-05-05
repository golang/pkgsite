// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"

	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

// upsertSymbolHistory upserts data into the symbol_history table.
func upsertSymbolHistory(ctx context.Context, ddb *database.DB,
	modulePath, ver string,
	nameToID map[string]int,
	pathToID map[string]int,
	pathToPkgsymID map[string]map[packageSymbol]int,
	pathToDocIDToDoc map[string]map[int]*internal.Documentation,
) (err error) {
	defer derrors.WrapStack(&err, "upsertSymbolHistory")

	versionType, err := version.ParseType(ver)
	if err != nil {
		return err
	}
	if versionType != version.TypeRelease || version.IsIncompatible(ver) {
		return nil
	}

	if _, err := ddb.Exec(ctx, `LOCK TABLE symbol_history IN EXCLUSIVE MODE`); err != nil {
		return err
	}
	for packagePath, docIDToDoc := range pathToDocIDToDoc {
		sh, err := GetSymbolHistoryFromTable(ctx, ddb, packagePath, modulePath)
		if err != nil {
			return err
		}
		for _, doc := range docIDToDoc {
			var values []interface{}
			builds := []internal.BuildContext{{GOOS: doc.GOOS, GOARCH: doc.GOARCH}}
			if doc.GOOS == internal.All {
				builds = internal.BuildContexts
			}
			for _, b := range builds {
				dbNameToVersion := map[string]string{}
				for _, v := range sh.Versions() {
					nts := sh.SymbolsAtVersion(v)
					for name, stu := range nts {
						for _, us := range stu {
							if us.SupportsBuild(b) {
								dbNameToVersion[name] = v
							}
						}
					}
				}
				seen := map[string]bool{}
				if err := updateSymbols(doc.API, func(sm *internal.SymbolMeta) error {
					// While a package with duplicate symbol names won't build,
					// the documentation for these packages are currently
					// rendered on pkg.go.dev, so doc.API may contain more than
					// one symbol with the same name.
					//
					// For the purpose of symbol_history, just use the first
					// symbol name we see.
					if seen[sm.Name] {
						return nil
					}
					seen[sm.Name] = true

					if shouldUpdateSymbolHistory(sm.Name, ver, dbNameToVersion) {
						values, err = appendSymbolHistoryRow(sm, values,
							packagePath, modulePath, ver, b, pathToID, nameToID,
							pathToPkgsymID)
						if err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					return err
				}
			}

			cols := []string{
				"symbol_name_id",
				"parent_symbol_name_id",
				"package_path_id",
				"module_path_id",
				"package_symbol_id",
				"since_version",
				"sort_version",
				"goos",
				"goarch",
			}
			if err := ddb.BulkInsert(ctx, "symbol_history", cols, values,
				`ON CONFLICT (package_path_id, module_path_id, symbol_name_id, goos, goarch)
				DO UPDATE
				SET
					symbol_name_id=excluded.symbol_name_id,
					parent_symbol_name_id=excluded.parent_symbol_name_id,
					package_path_id=excluded.package_path_id,
					module_path_id=excluded.module_path_id,
					package_symbol_id=excluded.package_symbol_id,
					since_version=excluded.since_version,
					sort_version=excluded.sort_version,
					goos=excluded.goos,
					goarch=excluded.goarch
				WHERE
					symbol_history.sort_version > excluded.sort_version`); err != nil {
				return err
			}
		}
	}
	return nil
}

func appendSymbolHistoryRow(sm *internal.SymbolMeta, values []interface{},
	packagePath, modulePath, ver string, build internal.BuildContext,
	pathToID, symToID map[string]int,
	pathToPkgsymID map[string]map[packageSymbol]int) (_ []interface{}, err error) {
	defer derrors.WrapStack(&err, "appendSymbolHistoryRow(%q, %q, %q, %q)", sm.Name, packagePath, modulePath, ver)
	symbolID := symToID[sm.Name]
	if symbolID == 0 {
		return nil, fmt.Errorf("symbolID cannot be 0: %q", sm.Name)
	}
	if sm.ParentName == "" {
		sm.ParentName = sm.Name
	}
	parentID := symToID[sm.ParentName]
	if parentID == 0 {
		return nil, fmt.Errorf("parentSymbolID cannot be 0: %q", sm.ParentName)
	}
	packagePathID := pathToID[packagePath]
	if packagePathID == 0 {
		return nil, fmt.Errorf("packagePathID cannot be 0: %q", packagePathID)
	}
	modulePathID := pathToID[modulePath]
	if modulePathID == 0 {
		return nil, fmt.Errorf("modulePathID cannot be 0: %q", modulePathID)
	}
	pkgsymID := pathToPkgsymID[packagePath][packageSymbol{synopsis: sm.Synopsis, name: sm.Name, parentName: sm.ParentName}]
	return append(values,
		symbolID,
		parentID,
		packagePathID,
		modulePathID,
		pkgsymID,
		ver,
		version.ForSorting(ver),
		build.GOOS,
		build.GOARCH), nil
}

// shouldUpdateSymbolHistory reports whether the row for the given symbolName
// should be updated. oldHist contains all of the current symbols in the
// database for the same package and GOOS/GOARCH.
//
// shouldUpdateSymbolHistory reports true if the symbolName does not currently
// exist, or if the newVersion is older than or equal to the current database version.
func shouldUpdateSymbolHistory(symbolName, newVersion string, oldHist map[string]string) bool {
	oldVersion, ok := oldHist[symbolName]
	if !ok {
		return true
	}
	return semver.Compare(newVersion, oldVersion) <= 0
}
