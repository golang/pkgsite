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

	curCounts, err := db.getSearchPackages(ctx)
	if err != nil {
		return 0, err
	}
	newCounts, err := db.computeImportedByCounts(ctx, curCounts)
	if err != nil {
		return 0, err
	}

	// Include only changed counts for packages that are in search_documents.
	changedCounts := computeChangedCounts(ctx, curCounts, newCounts)
	return db.UpdateSearchDocumentsImportedByCountWithCounts(ctx, changedCounts, batchSize)
}

// getSearchPackages returns the set of package paths that are in the search_documents table,
// along with their current imported-by count.
func (db *DB) getSearchPackages(ctx context.Context) (counts map[string]int, err error) {
	defer derrors.WrapStack(&err, "DB.getSearchPackages(ctx)")
	defer internal.RequestState(ctx, "reading search_packages table")()
	counts = map[string]int{}
	err = db.db.RunQuery(ctx, `
		SELECT package_path, imported_by_count
		FROM search_documents
	`, func(rows *sql.Rows) error {
		var (
			p string
			c int
		)
		if err := rows.Scan(&p, &c); err != nil {
			return err
		}
		counts[p] = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return counts, nil
}

func (db *DB) computeImportedByCounts(ctx context.Context, curCounts map[string]int) (newCounts map[string]int, err error) {
	defer derrors.WrapStack(&err, "db.computeImportedByCounts(ctx)")
	defer internal.RequestState(ctx, "computing counts")()

	newCounts = map[string]int{}
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
		// Don't count an importer if it's in the same module as what it's importing.
		// Approximate that check by seeing if from_module_path is a prefix of to_path.
		// (In some cases, e.g. when to_path is in a nested module, that is not correct.)
		if (fromMod == stdlib.ModulePath && stdlib.Contains(to)) || strings.HasPrefix(to+"/", fromMod+"/") {
			return nil
		}
		newCounts[to]++
		return nil
	})
	if err != nil {
		return nil, err
	}
	return newCounts, nil
}

func computeChangedCounts(ctx context.Context, curCounts, newCounts map[string]int) map[string]int {
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
	log.Debugf(ctx, "update-imported-by-counts: %d changed (%d%%)", len(changedCounts), pct)
	return changedCounts
}

func (db *DB) UpdateSearchDocumentsImportedByCountWithCounts(ctx context.Context, counts map[string]int, batchSize int) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "UpdateSearchDocumentsImportedByCountWithCounts")
	defer internal.RequestState(ctx, "updating search_documents")()
	total := len(counts)
	for len(counts) > 0 {
		var nu int64
		err := db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
			if err := insertImportedByCounts(ctx, tx, counts, batchSize); err != nil {
				return err
			}
			nu, err = updateImportedByCounts(ctx, tx)
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
func insertImportedByCounts(ctx context.Context, db *database.DB, counts map[string]int, limit int) (err error) {
	defer derrors.WrapStack(&err, "insertImportedByCounts(ctx, db, counts)")

	const createTableQuery = `
		CREATE TEMPORARY TABLE computed_imported_by_counts (
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
	return db.BulkInsert(ctx, "computed_imported_by_counts", columns, values, "")
}

// updateImportedByCounts updates the imported_by_count column in search_documents
// for every package in computed_imported_by_counts.
//
// Rows that don't change aren't updated.
//
// Note that if a package is never imported, its imported_by_count column will
// be the default (0) and its imported_by_count_updated_at column will never be set.
func updateImportedByCounts(ctx context.Context, db *database.DB) (int64, error) {
	// Lock the entire table to avoid deadlock. Without the lock, the update can
	// fail because module inserts are concurrently modifying rows of
	// search_documents.
	// See https://www.postgresql.org/docs/11/explicit-locking.html for what locks mean.
	// See https://www.postgresql.org/docs/11/sql-lock.html for the LOCK
	// statement, notably the paragraph beginning "If a transaction of this sort
	// is going to change the data...".
	const updateStmt = `
		LOCK TABLE search_documents IN SHARE ROW EXCLUSIVE MODE;
		UPDATE search_documents s
		SET
			imported_by_count = c.imported_by_count,
			imported_by_count_updated_at = CURRENT_TIMESTAMP
		FROM computed_imported_by_counts c
		INNER JOIN paths p ON p.path = c.package_path
		WHERE s.package_path_id = p.id;`

	n, err := db.Exec(ctx, updateStmt)
	if err != nil {
		return 0, fmt.Errorf("error updating imported_by_count and imported_by_count_updated_at for search documents: %v", err)
	}
	return n, nil
}
