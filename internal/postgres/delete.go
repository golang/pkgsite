// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
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
		if _, err = tx.Exec(ctx, `DELETE FROM search_documents WHERE module_path = $1 AND version = $2`, modulePath, resolvedVersion); err != nil {
			return err
		}

		var x int
		err = tx.QueryRow(ctx, `SELECT 1 FROM modules WHERE module_path=$1 LIMIT 1`, modulePath).Scan(&x)
		if err != sql.ErrNoRows || err == nil {
			return err
		}
		// No versions of this module exist; remove it from imports_unique.
		return deleteModuleFromImportsUnique(ctx, tx, modulePath)
	})
}

// DeleteOlderVersionFromSearchDocuments deletes from search_documents every package with
// the given module path whose version is older than the given version.
// It is used when fetching a module with an alternative path. See internal/worker/fetch.go:fetchAndUpdateState.
func (db *DB) DeleteOlderVersionFromSearchDocuments(ctx context.Context, modulePath, resolvedVersion string) (err error) {
	defer derrors.WrapStack(&err, "DeleteOlderVersionFromSearchDocuments(ctx, %q, %q)", modulePath, resolvedVersion)

	return db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
		// Collect all package paths in search_documents with the given module path
		// and an older version. (package_path is the primary key of search_documents.)
		var ppaths []string
		query := `
			SELECT package_path, version
			FROM search_documents
			WHERE module_path = $1
		`
		err := tx.RunQuery(ctx, query, func(rows *sql.Rows) error {
			var ppath, v string
			if err := rows.Scan(&ppath, &v); err != nil {
				return err
			}
			if semver.Compare(v, resolvedVersion) < 0 {
				ppaths = append(ppaths, ppath)
			}
			return nil
		}, modulePath)
		if err != nil {
			return err
		}
		if len(ppaths) == 0 {
			return nil
		}

		// Delete all of those paths.
		return deleteModuleOrPackagesInModuleFromSearchDocuments(ctx, tx, modulePath, ppaths)
	})
}

// deleteOtherModulePackagesFromSearchDocuments deletes all packages from search
// documents with the given module that are not in m.
func deleteOtherModulePackagesFromSearchDocuments(ctx context.Context, tx *database.DB, m *internal.Module) error {
	dbPkgs, err := tx.CollectStrings(ctx, `
		SELECT package_path FROM search_documents WHERE module_path = $1
	`, m.ModulePath)
	if err != nil {
		return err
	}
	pkgInModule := map[string]bool{}
	for _, u := range m.Packages() {
		pkgInModule[u.Path] = true
	}
	var otherPkgs []string
	for _, p := range dbPkgs {
		if !pkgInModule[p] {
			otherPkgs = append(otherPkgs, p)
		}
	}
	return deleteModuleOrPackagesInModuleFromSearchDocuments(ctx, tx, m.ModulePath, otherPkgs)
}

// deleteModuleOrPackagesInModuleFromSearchDocuments deletes module_path from
// search_documents. If packages is non-empty, it only deletes those packages.
func deleteModuleOrPackagesInModuleFromSearchDocuments(ctx context.Context, tx *database.DB, modulePath string, packages []string) error {
	d := squirrel.Delete("search_documents").
		Where(squirrel.Eq{"module_path": modulePath})
	if len(packages) > 0 {
		d = d.Where("package_path = ANY(?)", pq.Array(packages))
	}
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
