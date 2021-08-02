// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
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
		_, err = tx.Exec(ctx, `DELETE FROM imports_unique WHERE from_module_path = $1`, modulePath)
		return err
	})
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
