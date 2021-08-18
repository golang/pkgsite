// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// DeleteModule deletes a Version from the database.
func (db *DB) DeleteModule(ctx context.Context, modulePath, resolvedVersion string) (err error) {
	defer derrors.WrapStack(&err, "DeleteModule(ctx, db, %q, %q)", modulePath, resolvedVersion)
	return db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
		// We only need to delete from the modules table. Thanks to ON DELETE
		// CASCADE constraints, that will trigger deletions from all other tables.
		const stmt = `DELETE FROM modules WHERE module_path=$1 AND version=$2`
		if _, err := tx.Exec(ctx, stmt, modulePath, resolvedVersion); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM version_map WHERE module_path = $1 AND resolved_version = $2`, modulePath, resolvedVersion); err != nil {
			return err
		}

		var x int
		err = tx.QueryRow(ctx, `SELECT 1 FROM modules WHERE module_path=$1 LIMIT 1`, modulePath).Scan(&x)
		if err != sql.ErrNoRows || err == nil {
			return err
		}
		// No versions of this module exist; remove it from paths and
		// imports_unique.
		//
		// Deleting from paths will also cascade a DELETE to
		// latest_module_versions. Other tables should already have removed any
		// rows referencing the paths table.
		if _, err = tx.Exec(ctx, `DELETE FROM paths WHERE path = $1`, modulePath); err != nil {
			return err
		}
		return deleteModuleFromImportsUnique(ctx, tx, modulePath)
	})
}

// deleteOtherModulePackagesFromSearchDocuments deletes all packages from search
// documents with the given module that are not in m.
func deleteOtherModulePackagesFromSearchDocuments(ctx context.Context, tx *database.DB, modulePath string, pkgPaths []string) error {
	dbPkgs, err := tx.CollectStrings(ctx, `
		SELECT package_path FROM search_documents WHERE module_path = $1
	`, modulePath)
	if err != nil {
		return err
	}
	pkgInModule := map[string]bool{}
	for _, p := range pkgPaths {
		pkgInModule[p] = true
	}
	var otherPkgs []string
	for _, p := range dbPkgs {
		if !pkgInModule[p] {
			otherPkgs = append(otherPkgs, p)
		}
	}
	if len(otherPkgs) == 0 {
		// Nothing to delete.
		return nil
	}
	return deletePackagesInModuleFromSearchDocuments(ctx, tx, otherPkgs)
}

// deleteModuleFromSearchDocuments deletes module_path from search_documents.
func deleteModuleFromSearchDocuments(ctx context.Context, tx *database.DB, modulePath string) error {
	d := squirrel.Delete("search_documents").
		Where(squirrel.Eq{"module_path": modulePath})
	q, args, err := d.PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return err
	}
	n, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	log.Infof(ctx, "deleted %d rows of module %s from search_documents", n, modulePath)
	return nil
}

// deletePackagesInModuleFromSearchDocuments deletes packages from search_documents.
func deletePackagesInModuleFromSearchDocuments(ctx context.Context, tx *database.DB, pkgPaths []string) error {
	d := squirrel.Delete("search_documents").
		Where("package_path = ANY(?)", pq.Array(pkgPaths))
	q, args, err := d.PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return err
	}
	n, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	log.Infof(ctx, "deleted %d rows from search_documents: %v", n, pkgPaths)
	return nil
}

func deleteModuleFromImportsUnique(ctx context.Context, db *database.DB, modulePath string) (err error) {
	defer derrors.Wrap(&err, "deleteModuleFromImportsUnique(%q)", modulePath)

	_, err = db.Exec(ctx, `
		DELETE FROM imports_unique
		WHERE from_module_path = $1
	`, modulePath)
	return err
}

// DeletePseudoversionsExcept deletes all pseudoversions for the module except
// the provided resolvedVersion.
func (db *DB) DeletePseudoversionsExcept(ctx context.Context, modulePath, resolvedVersion string) (err error) {
	defer derrors.WrapStack(&err, "DeletePseudoversionsExcept(ctx, db, %q, %q)", modulePath, resolvedVersion)
	return db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
		const stmt = `
			DELETE FROM modules
			WHERE version_type = 'pseudo' AND module_path=$1 AND version != $2
			RETURNING version`
		versions, err := tx.CollectStrings(ctx, stmt, modulePath, resolvedVersion)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM version_map WHERE module_path = $1 AND resolved_version = ANY($2)`,
			modulePath, pq.Array(versions))
		return err
	})
}
