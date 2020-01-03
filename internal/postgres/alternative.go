// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package postgres

import (
	"context"
	"database/sql"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/derrors"
)

// InsertAlternativeModulePath inserts the alternative module path into the alternative_module_paths table.
func (db *DB) InsertAlternativeModulePath(ctx context.Context, alternative *internal.AlternativeModulePath) (err error) {
	derrors.Wrap(&err, "DB.InsertAlternativeModulePath(ctx, %v)", alternative)
	_, err = db.db.Exec(ctx, `
		INSERT INTO alternative_module_paths (alternative, canonical)
		VALUES($1, $2) ON CONFLICT DO NOTHING;`,
		alternative.Alternative, alternative.Canonical)
	return err
}

// DeleteAlternatives deletes all modules with the given path.
func (db *DB) DeleteAlternatives(ctx context.Context, alternativePath string) (err error) {
	derrors.Wrap(&err, "DB.DeleteAlternatives(ctx)")

	return db.db.Transact(func(tx *sql.Tx) error {
		if _, err := database.ExecTx(ctx, tx,
			`DELETE FROM versions WHERE module_path = $1;`, alternativePath); err != nil {
			return err
		}
		if _, err := database.ExecTx(ctx, tx,
			`DELETE FROM imports_unique WHERE from_module_path = $1;`, alternativePath); err != nil {
			return err
		}
		if _, err := database.ExecTx(ctx, tx,
			`UPDATE module_version_states SET status = $1 WHERE module_path = $2;`,
			derrors.ToHTTPStatus(derrors.AlternativeModule), alternativePath); err != nil {
			return err
		}
		return nil
	})
}

// IsAlternativePath reports whether the path represents the canonical path for a
// package, module or directory.
func (db *DB) IsAlternativePath(ctx context.Context, path string) (_ bool, err error) {
	defer derrors.Wrap(&err, "IsAlternativePath(ctx, %q)", path)
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM alternative_module_paths
			WHERE $1 LIKE alternative || '%'
		);`
	row := db.db.QueryRow(ctx, query, path)
	var isAlternative bool
	err = row.Scan(&isAlternative)
	return isAlternative, err
}

func (db *DB) getAlternativeModulePath(ctx context.Context, alternativePath string) (_ *internal.AlternativeModulePath, err error) {
	defer derrors.Wrap(&err, "GetAlternativeModulePath(ctx, %q)", alternativePath)
	query := `
		SELECT alternative, canonical
		FROM alternative_module_paths
		WHERE alternative = $1;`
	row := db.db.QueryRow(ctx, query, alternativePath)
	var alternative, canonical string
	if err := row.Scan(&alternative, &canonical); err != nil {
		return nil, err
	}
	return &internal.AlternativeModulePath{
		Alternative: alternative,
		Canonical:   canonical,
	}, nil
}
