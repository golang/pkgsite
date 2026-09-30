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

// ImporterCountOptions are parameters for UpdateSearchDocumentsImportedByCount.
type ImporterCountOptions struct {
	// Number of rows to update in search_documents at once
	// (with the table lock held).
	// If zero, a reasonable default is used.
	BatchSize int
	// Maximum time to spend updating. If zero, then no max.
	MaxTime time.Duration
	// How old a count has to be before updating it.
	// If zero, update everything.
	Staleness time.Duration
}

// UpdateSearchDocumentsImportedByCount updates imported_by_count and
// imported_by_count_updated_at.
// It also updates imported_by_module_count and imported_by_module_count_updated_at.
//
// It does so by completely recalculating the imported-by counts
// from the imports_unique table.
//
// UpdateSearchDocumentsImportedByCount returns the number of rows updated.
func (db *DB) UpdateSearchDocumentsImportedByCount(ctx context.Context, opts *ImporterCountOptions) (nPackageUpdated, nModuleUpdated int64, err error) {
	defer derrors.WrapStack(&err, "UpdateSearchDocumentsImportedByCount(ctx, %+v)", opts)

	var o ImporterCountOptions
	if opts != nil {
		o = *opts
	}
	if o.BatchSize == 0 {
		o.BatchSize = 1000
	}
	// As per the doc, a zero MaxTime means (effectively) no max.
	if o.MaxTime == 0 {
		o.MaxTime = 24 * 365 * 10 * time.Hour
	}
	// Adjust the total time to give each part (modules, packages) half
	// the time.
	o.MaxTime /= 2

	log.Infof(ctx, "updating imported-by module counts, opts = %+v", o)
	nModuleUpdated, err = db.updateImportedByModuleCounts(ctx, o)
	if err != nil {
		return 0, 0, err
	}
	log.Infof(ctx, "updated %d imported-by module counts", nModuleUpdated)

	log.Infof(ctx, "updating imported-by package counts, opts = %+v", o)
	nPackageUpdated, err = db.updateImportedByPackageCounts(ctx, o)
	if err != nil {
		return 0, nModuleUpdated, err
	}
	log.Infof(ctx, "updated %d imported-by package counts", nPackageUpdated)
	return nPackageUpdated, nModuleUpdated, nil
}

// TODO(jba): remove this after rewriting the call in tests/search/main.go.
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

	nUpdated, err := db.Exec(ctx, updateStmt)
	if err != nil {
		return 0, fmt.Errorf("error updating %[1]s and %[1]s_updated_at for search documents: %w", column, err)
	}
	return nUpdated, nil
}

func (db *DB) updateImportedByPackageCounts(ctx context.Context, opts ImporterCountOptions) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "updateImportedByPackageCounts(ctx, %+v)", opts)

	// See the comment on the query in updateImportedByModuleCounts for an explanation of this query.
	// The important difference is the exclusion of packages in the same module, approximated by
	// checking whether one import path is a prefix of the other. We could now get a precise answer
	// by joining with search_documents and comparing module_paths directly, but that would change
	// some numbers and we want to keep these counts the same while we introduce module counts.
	// TODO(golang/go#81962): improve the logic for seeing if two packages are in the same module.
	const query = `
	CREATE TEMPORARY TABLE computed_imported_by_counts (package_path, count) ON COMMIT DROP AS
	WITH stale AS (
		SELECT package_path
		FROM search_documents
		WHERE imported_by_count_updated_at < $1
		OR imported_by_count_updated_at IS NULL
		ORDER BY imported_by_count_updated_at NULLS FIRST
		LIMIT $2
	)
	SELECT stale.package_path, COUNT(DISTINCT u.from_path)
	FROM imports_unique u
	RIGHT JOIN stale ON (
		u.to_path = stale.package_path
		AND NOT (
			(u.from_module_path = 'std' AND strpos(split_part(u.to_path, '/', 1), '.') = 0)
			OR starts_with(u.to_path || '/', u.from_module_path || '/')
		)
		AND EXISTS (
			SELECT 1
			FROM search_documents s
			WHERE s.package_path = u.from_path
		)
	)
	GROUP BY 1;
	`
	return db.updateImporterCounts(ctx, query, "imported_by_count", opts)
}

// Update the number of importing modules for packages in search_documents.
//
// For each package P in search_documents, we count all unique modules that have
// a package that imports P. For example, if a package is imported by N different
// packages, but all N of the packages belong to the same module, the count will
// be 1. This includes P's module as well, so we can distinguish packages that have
// at least one importer from those that have zero. (Though note that since we don't
// track test packages, a package with zero importers might still be used in tests.)
func (db *DB) updateImportedByModuleCounts(ctx context.Context, opts ImporterCountOptions) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "updateImportedByModuleCounts(ctx, %+v)", opts)
	// This query is (mostly) efficient because the primary key on imports_unique is (to_path, from_path, from_module_path),
	// so finding a package path is fast and its importers are contiguous, making
	// the COUNT fast. But the NULLS FIRST means we can't use the index on the X_updated_at
	// column. TODO: if this query is slow, use a UNION to write the two conditions
	// as separate queries.
	//
	// The right join ensures that we get rows for packages with no imports,
	// that therefore don't appear in imports_unique.
	//
	// The EXISTS check makes sure we only count modules in search_documents,
	// avoiding alternative modules that might be in imports_unique.
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
	return db.updateImporterCounts(ctx, query, "imported_by_module_count", opts)
}

// Update all rows in search_documents that haven't been updated in opts.Staleness.
// A row is updated even if it hasn't changed, in order to set its
// update time for future runs.
//
// Run the query to update importer counts repeatedly within opts.MaxTime.
// countCol is the column in search_documents to update.
//
// The query must have two parameters.
// $1 is the cutoff time, the current time minus opts.Staleness.
// Computing the cutoff date outside the query, instead of using an expression like
// "age(X) < Y", ensures that Postgres uses the index on the updated_at column.
//
// $2 is opts.BatchSize, the maximum number of rows it should process.
func (db *DB) updateImporterCounts(ctx context.Context, query, countCol string, opts ImporterCountOptions) (nUpdated int64, err error) {
	defer derrors.WrapStack(&err, "updateImporterCounts(ctx, %+v)", opts)
	cutoff := time.Now().Add(-opts.Staleness)
	deadline := time.Now().Add(opts.MaxTime)
	for time.Now().Before(deadline) {
		var nu int64
		err = db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
			if _, err := tx.Exec(ctx, query, cutoff, opts.BatchSize); err != nil {
				return fmt.Errorf("creating temp table: %w", err)
			}
			nu, err = updateImportedByCounts(ctx, tx, countCol)
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
