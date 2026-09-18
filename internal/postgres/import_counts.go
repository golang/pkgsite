// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/stdlib"
)

// UpdateSearchDocumentsImportedByCount updates imported_by_count and
// imported_by_count_updated_at.
//
// It does so by completely recalculating the imported-by counts
// from the imports_unique table.
//
// UpdateSearchDocumentsImportedByCount returns the number of rows updated.
func (db *DB) UpdateSearchDocumentsImportedByCount(ctx context.Context, batchSize int) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "UpdateSearchDocumentsImportedByCount(ctx)")

	log.Infof(ctx, "updating imported-by counts, batch size = %d", batchSize)

	curCounts, curModCounts, err := db.getSearchPackages(ctx)
	if err != nil {
		return 0, err
	}
	newCounts, newModCounts, err := db.computeImportedByCounts(ctx, curCounts)
	if err != nil {
		return 0, err
	}

	// Include only changed counts for packages that are in search_documents.
	changedCounts := computeChangedCounts(ctx, "packages", curCounts, newCounts)
	changedModCounts := computeChangedCounts(ctx, "modules", curModCounts, newModCounts)
	return db.UpdateSearchDocumentsImportedByCountWithCounts(ctx, changedCounts, changedModCounts, batchSize)
}

// getSearchPackages returns the set of package paths that are in the search_documents table,
// along with their current imported-by count and imported-by-module count.
func (db *DB) getSearchPackages(ctx context.Context) (counts, modcounts map[string]int, err error) {
	defer derrors.WrapStack(&err, "DB.getSearchPackages(ctx)")
	defer internal.RequestState(ctx, "reading search_packages table")()
	counts = map[string]int{}
	modcounts = map[string]int{}
	err = db.db.RunQuery(ctx, `
		SELECT package_path, imported_by_count, imported_by_module_count
		FROM search_documents
	`, func(rows *sql.Rows) error {
		var (
			p     string
			c, mc int
		)
		if err := rows.Scan(&p, &c, &mc); err != nil {
			return err
		}
		counts[p] = c
		modcounts[p] = mc
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return counts, modcounts, nil
}

func (db *DB) computeImportedByCounts(ctx context.Context, curCounts map[string]int) (newCounts, newModCounts map[string]int, err error) {
	defer derrors.WrapStack(&err, "db.computeImportedByCounts(ctx)")
	defer internal.RequestState(ctx, "computing counts")()

	newCounts = map[string]int{}

	// We don't want to double-count modules. so keep a set
	// from to_path to from_module_path.
	modSets := map[string]map[string]struct{}{}
	// Get all (from_path, to_path) pairs, deduped.
	// Also get the from_path's module path.
	err = db.db.RunQuery(ctx, `
		SELECT DISTINCT from_path, from_module_path, to_path
		FROM imports_unique
	`, func(rows *sql.Rows) error {
		var from, fromMod, to string
		if err := rows.Scan(&from, &fromMod, &to); err != nil {
			return err
		}
		// Don't count an importer if it's not in search_documents.
		if _, ok := curCounts[from]; !ok {
			return nil
		}

		// Count an importing module even if it's the same module as the package itself.
		// This lets us distinguish packages that are truly unused from those that are only used
		// within their module.
		m := modSets[to]
		if m == nil {
			m = map[string]struct{}{}
			modSets[to] = m
		}
		m[fromMod] = struct{}{}
		// Don't count an importing package if it's in the same module as what it's importing.
		// Unlike with modules, there is too much opportunity to inflate the count.
		// Approximate that check by seeing if from_module_path is a prefix of to_path.
		// (In some cases, e.g. when to_path is in a nested module, that is not correct.)
		if (fromMod == stdlib.ModulePath && stdlib.Contains(to)) || strings.HasPrefix(to+"/", fromMod+"/") {
			return nil
		}
		newCounts[to]++
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	newModCounts = map[string]int{}
	for to, modSet := range modSets {
		newModCounts[to] = len(modSet)
	}
	return newCounts, newModCounts, nil
}

func computeChangedCounts(ctx context.Context, prefix string, curCounts, newCounts map[string]int) map[string]int {
	// Find all counts that have changed, including those that have changed to zero
	// because there are no longer any importers in imports_unique.
	changedCounts := map[string]int{}
	for p, cc := range curCounts {
		nc := newCounts[p] // nc is 0 if not present in newCounts
		if cc != nc {
			changedCounts[p] = nc
		}
	}

	pct := 0
	if len(curCounts) > 0 {
		pct = len(changedCounts) * 100 / len(curCounts)
	}
	log.Debugf(ctx, "update-imported-by-counts: %s: %d changed (%d%%)", prefix, len(changedCounts), pct)
	return changedCounts
}

func (db *DB) UpdateSearchDocumentsImportedByCountWithCounts(ctx context.Context, pkgCounts, modCounts map[string]int, batchSize int) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "UpdateSearchDocumentsImportedByCountWithCounts")
	defer internal.RequestState(ctx, "updating search_documents")()
	total := len(pkgCounts) + len(modCounts)
	for len(pkgCounts) > 0 {
		var nu int64
		err := db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
			if err := insertImportedByCounts(ctx, tx, "package", pkgCounts, batchSize); err != nil {
				return err
			}
			nu, err = updateImportedByCounts(ctx, tx, "package")
			return err
		})
		if err != nil {
			return nUpdated, err
		}
		nUpdated += nu
		internal.RequestState(ctx, fmt.Sprintf("updating search_documents: %d/%d", nUpdated, total))
	}
	for len(modCounts) > 0 {
		var nu int64
		err := db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
			if err := insertImportedByCounts(ctx, tx, "module", modCounts, batchSize); err != nil {
				return err
			}
			nu, err = updateImportedByCounts(ctx, tx, "module")
			return err
		})
		if err != nil {
			return nUpdated, err
		}
		nUpdated += nu
		internal.RequestState(ctx, fmt.Sprintf("updating search_documents: %d/%d", nUpdated, total))
	}
	return nUpdated, nil
}

// insertImportedByCounts creates a temporary table and inserts at most limit
// rows into it, where each row is a key and value from the counts map. The
// inserted keys are deleted from counts.
func insertImportedByCounts(ctx context.Context, db *database.DB, kind string, counts map[string]int, limit int) (err error) {
	defer derrors.WrapStack(&err, "insertImportedByCounts(ctx, db, counts)")

	tableName := "computed_" + kind + "_counts"
	createTableQuery := `
		CREATE TEMPORARY TABLE ` + tableName + ` (
			package_path      TEXT NOT NULL,
			imported_by_count INTEGER NOT NULL
		) ON COMMIT DROP;
    `
	if _, err := db.Exec(ctx, createTableQuery); err != nil {
		return fmt.Errorf("CREATE TABLE: %v", err)
	}
	var values []any
	i := 0
	for p, c := range counts {
		if i >= limit {
			break
		}
		values = append(values, p, c)
		delete(counts, p)
		i++
	}
	columns := []string{"package_path", "imported_by_count"}
	return db.BulkInsert(ctx, tableName, columns, values, "")
}

// updateImportedByCounts updates the imported_by_count or imported_by_module_count
// column in search_documents for every package in computed_[kind]_counts.
//
// Rows that don't change aren't updated.
//
// Note that if a package is never imported, its imported_by_count column will
// be the default (0) and its imported_by_count_updated_at column will never be set.
func updateImportedByCounts(ctx context.Context, db *database.DB, kind string) (int64, error) {
	// Lock the entire table to avoid deadlock. Without the lock, the update can
	// fail because module inserts are concurrently modifying rows of
	// search_documents.
	// See https://www.postgresql.org/docs/11/explicit-locking.html for what locks mean.
	// See https://www.postgresql.org/docs/11/sql-lock.html for the LOCK
	// statement, notably the paragraph beginning "If a transaction of this sort
	// is going to change the data...".

	var setStmt string
	switch kind {
	case "package":
		setStmt = `
			imported_by_count = c.imported_by_count,
			imported_by_count_updated_at = CURRENT_TIMESTAMP`
	case "module":
		setStmt = `
			imported_by_module_count = c.imported_by_count,
			imported_by_module_count_updated_at = CURRENT_TIMESTAMP`
	default:
		return 0, fmt.Errorf("unknown kind %q", kind)
	}

	updateStmt := fmt.Sprintf(`
		LOCK TABLE search_documents IN SHARE ROW EXCLUSIVE MODE;
		UPDATE search_documents s
		SET %s
		FROM computed_%s_counts c
		INNER JOIN paths p ON p.path = c.package_path
		WHERE s.package_path_id = p.id;`, setStmt, kind)

	n, err := db.Exec(ctx, updateStmt)
	if err != nil {
		return 0, fmt.Errorf("error updating imported-by counts (%s) for search documents: %v", kind, err)
	}
	return n, nil
}
