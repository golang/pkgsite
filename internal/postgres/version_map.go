// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

// UpsertVersionMap inserts a version_map entry into the database.
func (db *DB) UpsertVersionMap(ctx context.Context, vm *internal.VersionMap) (err error) {
	defer derrors.Wrap(&err, "DB.UpsertVersionMap(ctx, tx, %q, %q, %q)",
		vm.ModulePath, vm.RequestedVersion, vm.ResolvedVersion)

	var moduleID int
	if vm.ResolvedVersion != "" {
		if err := db.db.QueryRow(ctx, `SELECT id FROM modules WHERE module_path=$1 AND version=$2`,
			vm.ModulePath, vm.ResolvedVersion).Scan(&moduleID); err != nil && err != sql.ErrNoRows {
			return err
		}
	}

	var sortVersion string
	if vm.ResolvedVersion != "" {
		sortVersion = version.ForSorting(vm.ResolvedVersion)
	}
	_, err = db.db.Exec(ctx,
		`INSERT INTO version_map(
				module_path,
				requested_version,
				resolved_version,
				go_mod_path,
				status,
				error,
				sort_version,
				module_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (module_path, requested_version)
			DO UPDATE SET
				module_path=excluded.module_path,
				go_mod_path=excluded.go_mod_path,
				requested_version=excluded.requested_version,
				resolved_version=excluded.resolved_version,
				status=excluded.status,
				error=excluded.error,
				sort_version=excluded.sort_version,
				module_id=excluded.module_id`,
		vm.ModulePath,
		vm.RequestedVersion,
		vm.ResolvedVersion,
		vm.GoModPath,
		vm.Status,
		vm.Error,
		sortVersion,
		moduleID)
	return err
}

// GetVersionMap fetches a version_map entry corresponding to the given
// modulePath and requestedVersion.
func (db *DB) GetVersionMap(ctx context.Context, modulePath, requestedVersion string) (_ *internal.VersionMap, err error) {
	defer derrors.Wrap(&err, "DB.GetVersionMap(ctx, tx, %q, %q)", modulePath, requestedVersion)
	if modulePath == internal.UnknownModulePath {
		return nil, fmt.Errorf("modulePath must be specified: %w", derrors.InvalidArgument)
	}

	query := `
		SELECT
			module_path,
			requested_version,
			resolved_version,
			status,
			error
		FROM
			version_map
		WHERE
			module_path=$1
			AND requested_version=$2;`
	var vm internal.VersionMap
	err = db.db.QueryRow(ctx, query, modulePath, requestedVersion).Scan(
		&vm.ModulePath, &vm.RequestedVersion, &vm.ResolvedVersion, &vm.Status, &vm.Error)
	switch err {
	case nil:
		return &vm, nil
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	default:
		return nil, err
	}
}
