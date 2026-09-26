// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// UpdateSearchDocumentsImportedByCount updates imported_by_count and
// imported_by_count_updated_at.
// It also updates imported_by_module_count and imported_by_module_count_updated_at.
//
// It does so by completely recalculating the imported-by counts
// from the imports_unique table.
//
// UpdateSearchDocumentsImportedByCount returns the number of rows updated.
func (db *DB) UpdateSearchDocumentsImportedByCount(ctx context.Context, batchSize int) (nPackageUpdated, nModuleUpdated int64, err error) {
	defer derrors.WrapStack(&err, "UpdateSearchDocumentsImportedByCount(ctx)")

	log.Infof(ctx, "updating imported-by module counts, batch size = %d", batchSize)
	nModuleUpdated, err = db.updateImportedByModuleCounts(ctx, batchSize)
	if err != nil {
		return 0, 0, err
	}

	log.Infof(ctx, "updating imported-by package counts, batch size = %d", batchSize)

	curCounts, err := db.getSearchPackages(ctx)
	if err != nil {
		return 0, nModuleUpdated, err
	}
	newCounts, err := db.computeImportedByCounts(ctx)
	if err != nil {
		return 0, nModuleUpdated, err
	}

	// Include only changed counts for packages that are in search_documents.
	changedCounts := computeChangedCounts(ctx, curCounts, newCounts)
	nPackageUpdated, err = db.UpdateSearchDocumentsImportedByCountWithCounts(ctx, changedCounts, batchSize)
	return nPackageUpdated, nModuleUpdated, err
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

func (db *DB) computeImportedByCounts(ctx context.Context) (newCounts map[string]int, err error) {
	defer derrors.WrapStack(&err, "db.computeImportedByCounts(ctx)")
	defer internal.RequestState(ctx, "computing counts")()

	newCounts = map[string]int{}
	err = db.db.RunQuery(ctx, `
		SELECT u.to_path, COUNT(DISTINCT u.from_path)
		FROM imports_unique u
		INNER JOIN search_documents s_from ON s_from.package_path = u.from_path AND s_from.module_path = u.from_module_path
		INNER JOIN search_documents s_to ON s_to.package_path = u.to_path
		WHERE NOT (
			(u.from_module_path = 'std' AND strpos(split_part(u.to_path, '/', 1), '.') = 0)
			OR starts_with(u.to_path || '/', u.from_module_path || '/')
		)
		GROUP BY u.to_path
	`, func(rows *sql.Rows) error {
		var to string
		var count int
		if err := rows.Scan(&to, &count); err != nil {
			return err
		}
		newCounts[to] = count
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
			nu, err = updateImportedByCounts(ctx, tx, "imported_by_count")
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
			package_path TEXT NOT NULL,
			count        INTEGER NOT NULL
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
	columns := []string{"package_path", "count"}
	return db.BulkInsert(ctx, "computed_imported_by_counts", columns, values, "")
}

// updateImportedByCounts updates the given count column (e.g. imported_by_count
// or imported_by_module_count) and its corresponding _updated_at column in
// search_documents for every package in computed_imported_by_counts.
func updateImportedByCounts(ctx context.Context, db *database.DB, column string) (int64, error) {
	// Lock the entire table to avoid deadlock. Without the lock, the update can
	// fail because module inserts are concurrently modifying rows of
	// search_documents.
	// See https://www.postgresql.org/docs/11/explicit-locking.html for what locks mean.
	// See https://www.postgresql.org/docs/11/sql-lock.html for the LOCK
	// statement, notably the paragraph beginning "If a transaction of this sort
	// is going to change the data...".
	updateStmt := fmt.Sprintf(`
		LOCK TABLE search_documents IN SHARE ROW EXCLUSIVE MODE;
		UPDATE search_documents s
		SET
			%[1]s = c.count,
			%[1]s_updated_at = CURRENT_TIMESTAMP
		FROM computed_imported_by_counts c
		INNER JOIN paths p ON p.path = c.package_path
		WHERE s.package_path_id = p.id;`, column)

	n, err := db.Exec(ctx, updateStmt)
	if err != nil {
		return 0, fmt.Errorf("error updating %[1]s and %[1]s_updated_at for search documents: %w", column, err)
	}
	return n, nil
}

// How stale a count has to be before we consider updating it.
// This is a var so it can be changed in tests.
var moduleCountUpdateDuration = 7 * 24 * time.Hour

// How long we'll run for. The Cloud Scheduler max timeout is 30 minutes.
// We run for much less because the package counts, which run after this, take a long time.
// TODO(jba): once we speed up package counts, we'll run both within a time budget.
const maxTime = 2 * time.Minute

// that imports P. For example, if a package is imported by N different packages,
// but all N of the packages belong to the same module, the count will be 1. This
// includes P's module as well, so we can distinguish packages that have at least
// one importer from those that have zero. (Though note that since we don't track
// test packages, a package with zero importers might still be used Update the number
// of importing modules for packages in search_documents. The number of packages
// we process is limited by maxTime, declared above. During that time, we process
// as many as we can in batches of batchSize.
//
// For each package P in search_documents, we count all unique modules that have
// a package that imports P. For example, if a package is imported by N different
// packages, but all N of the packages belong to the same module, the count will
// be 1. This includes P's module as well, so we can distinguish packages that have
// at least one importer from those that have zero. (Though note that since we don't
// track test packages, a package with zero importers might still be used in tests.)
// The number of packages we process is limited by maxTime, declared above. During
// that time, we process as many as we can in batches of batchSize.
func (db *DB) updateImportedByModuleCounts(ctx context.Context, batchSize int) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "updateImportedByModuleCounts(ctx, %d)", batchSize)

	cutoff := time.Now().Add(-moduleCountUpdateDuration)
	deadline := time.Now().Add(maxTime)
	for time.Now().Before(deadline) {
		// Update all rows in search_documents that haven't been updated in a few
		// days or that have never been updated, preferring the latter. A row is
		// updated even if it hasn't changed, in order to set its update time for
		// future runs.
		//
		// This query is efficient because there is an index on
		// imported_by_module_count_updated_at, so finding old rows is fast, and
		// because the primary key on imports_unique is (to_path, from_path, from_module_path),
		// so finding a package path is fast and its importers are contiguous, making
		// the COUNT fast.
		//
		// Computing the cutoff date outside the query, instead of using an expression
		// like "age(X) < Y", ensures that Postgres uses the index on
		// imported_by_module_count_updated_at.
		//
		// The right join ensures that we get rows for packages with no imports,
		// that therefore don't appear in imports_unique.
		//
		// The EXISTS check makes sure we only count modules in search_documents,
		// avoiding alternative modules that might be in imports_unique.
		var nu int64
		err = db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
			const query = `
			CREATE TEMPORARY TABLE computed_imported_by_counts (package_path, count) ON COMMIT DROP AS
			WITH stale AS (
				SELECT package_path
				FROM search_documents
				WHERE imported_by_module_count_updated_at < $1
				OR imported_by_module_count_updated_at IS NULL
				ORDER BY imported_by_module_count_updated_at NULLS FIRST
				LIMIT $2
			)
			SELECT stale.package_path, COUNT(DISTINCT u.from_module_path)
			FROM imports_unique u
			RIGHT JOIN stale ON (
				u.to_path = stale.package_path
				AND EXISTS (
					SELECT 1
					FROM search_documents s
					WHERE s.module_path = u.from_module_path
				)
			)
			GROUP BY 1;
		`
			if _, err := tx.Exec(ctx, query, cutoff, batchSize); err != nil {
				return fmt.Errorf("creating temp table: %w", err)
			}
			nu, err = updateImportedByCounts(ctx, tx, "imported_by_module_count")
			return err
		})
		if err != nil {
			return nUpdated, err
		}
		if nu == 0 {
			break
		}
		nUpdated += nu
	}
	return nUpdated, nil
}
