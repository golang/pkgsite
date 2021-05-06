// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Functions for cleaning the database of unwanted module versions.

package postgres

import (
	"context"
	"database/sql"

	"golang.org/x/pkgsite/internal/derrors"
)

// A ModuleVersion holds a module path and version.
type ModuleVersion struct {
	ModulePath string
	Version    string
}

func (mv ModuleVersion) String() string {
	return mv.ModulePath + "@" + mv.Version
}

// GetModuleVersionsToClean returns module versions that can be removed from the database.
// Only module versions that were updated more than daysOld days ago will be considered.
// At most limit module versions will be returned.
func (db *DB) GetModuleVersionsToClean(ctx context.Context, daysOld, limit int) (modvers []ModuleVersion, err error) {
	defer derrors.WrapStack(&err, "GetModuleVersionsToClean(%d, %d)", daysOld, limit)

	// Get all pseudo-versions that were added before the given number of days.
	// Then remove:
	// - The ones that are the latest versions for their module,
	// - The ones in search_documents (since the latest version of a package might be at an older version),
	// - The ones that the master or main branch resolves to.
	query := `
		SELECT module_path, version
		FROM modules
		WHERE version_type = 'pseudo'
		AND CURRENT_TIMESTAMP - updated_at > make_interval(days => $1)
		EXCEPT (
			SELECT p.path, l.good_version
			FROM latest_module_versions l
			INNER JOIN paths p ON p.id = l.module_path_id
			WHERE good_version != ''
		)
		EXCEPT (
			SELECT module_path, version
			FROM search_documents
		)
		EXCEPT (
			SELECT module_path, resolved_version
			FROM version_map
			WHERE requested_version IN ('master', 'main')
		)
		LIMIT $2
	`

	err = db.db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var mv ModuleVersion
		if err := rows.Scan(&mv.ModulePath, &mv.Version); err != nil {
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
func (db *DB) CleanModuleVersions(ctx context.Context, mvs []ModuleVersion, reason string) (err error) {
	defer derrors.Wrap(&err, "CleanModuleVersions(%d modules)", len(mvs))

	status := derrors.ToStatus(derrors.Cleaned)
	for _, mv := range mvs {
		if err := db.UpdateModuleVersionStatus(ctx, mv.ModulePath, mv.Version, status, reason); err != nil {
			return err
		}
		if err := db.DeleteModule(ctx, mv.ModulePath, mv.Version); err != nil {
			return err
		}
	}
	return nil
}

// CleanModule deletes all versions of the given module path from the DB and marks them
// as cleaned in module_version_states.
func (db *DB) CleanModule(ctx context.Context, modulePath, reason string) (err error) {
	defer derrors.Wrap(&err, "CleanModule(%q)", modulePath)

	var mvs []ModuleVersion
	err = db.db.RunQuery(ctx, `
		SELECT version
		FROM modules
		WHERE module_path = $1
	`, func(rows *sql.Rows) error {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		mvs = append(mvs, ModuleVersion{modulePath, v})
		return nil
	}, modulePath)
	if err != nil {
		return err
	}
	return db.CleanModuleVersions(ctx, mvs, reason)
}
