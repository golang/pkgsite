// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Functions for cleaning the database of unwanted module versions.

package postgres

import (
	"context"
	"database/sql"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// GetModuleVersionsToClean returns module versions that can be removed from the database.
// Only module versions that were updated more than daysOld days ago will be considered.
// At most limit module versions will be returned.
func (db *DB) GetModuleVersionsToClean(ctx context.Context, daysOld, limit int) (modvers []internal.Modver, err error) {
	defer derrors.WrapStack(&err, "GetModuleVersionsToClean(%d, %d)", daysOld, limit)

	// Get all pseudo-versions that were added before the given number of days.
	// Then remove:
	// - The ones that are the latest versions for their module,
	// - The ones in search_documents (since the latest version of a package might be at an older version),
	// - The ones that the master or main branch resolves to.
	query := `
		SELECT
			m.module_path,
			m.version
		FROM
			modules m
		LEFT JOIN
			(
				SELECT p.path, l.good_version
				FROM latest_module_versions l
				JOIN paths p ON p.id = l.module_path_id
				WHERE l.good_version != ''
			) latest ON m.module_path = latest.path AND m.version = latest.good_version
		LEFT JOIN
			search_documents sd ON m.module_path = sd.module_path AND m.version = sd.version
		LEFT JOIN
			(
				SELECT module_path, resolved_version
				FROM version_map
				WHERE requested_version IN ('master', 'main', 'dev.fuzz')
			) vm_filtered ON m.module_path = vm_filtered.module_path AND m.version = vm_filtered.resolved_version
		WHERE
			m.version_type = 'pseudo'
			AND CURRENT_TIMESTAMP - m.updated_at > make_interval(days => $1)
			AND latest.path IS NULL
			AND sd.module_path IS NULL
			AND vm_filtered.module_path IS NULL
		LIMIT $2
	`

	err = db.db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var mv internal.Modver
		if err := rows.Scan(&mv.Path, &mv.Version); err != nil {
			return err
		}
		modvers = append(modvers, mv)
		return nil
	}, daysOld, limit)
	if err != nil {
		return nil, err
	}
	return modvers, nil
}

// CleanModuleVersions deletes each module version from the DB and marks it as cleaned
// in module_version_states.
func (db *DB) CleanModuleVersions(ctx context.Context, mvs []internal.Modver, reason string) (err error) {
	defer derrors.Wrap(&err, "CleanModuleVersions(%d modules)", len(mvs))

	status := derrors.ToStatus(derrors.Cleaned)
	for _, mv := range mvs {
		if err := db.UpdateModuleVersionStatus(ctx, mv.Path, mv.Version, status, reason); err != nil {
			return err
		}
		if err := db.DeleteModule(ctx, mv.Path, mv.Version); err != nil {
			return err
		}
	}
	return nil
}

// CleanAllModuleVersions deletes all versions of the given module path from the DB and marks them
// as cleaned in module_version_states.
func (db *DB) CleanAllModuleVersions(ctx context.Context, modulePath, reason string) (err error) {
	defer derrors.Wrap(&err, "CleanModule(%q)", modulePath)

	var mvs []internal.Modver
	err = db.db.RunQuery(ctx, `
		SELECT version
		FROM modules
		WHERE module_path = $1
	`, func(rows *sql.Rows) error {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		mvs = append(mvs, internal.Modver{Path: modulePath, Version: v})
		return nil
	}, modulePath)
	if err != nil {
		return err
	}
	return db.CleanModuleVersions(ctx, mvs, reason)
}
