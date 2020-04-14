// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/version"
)

// UpsertVersionMap inserts a version_map entry into the database.
func (db *DB) UpsertVersionMap(ctx context.Context, vm *internal.VersionMap) (err error) {
	defer derrors.Wrap(&err, "DB.UpsertVersionMap(ctx, tx, %q, %q, %q)",
		vm.ModulePath, vm.RequestedVersion, vm.ResolvedVersion)

	var sortVersion string
	if vm.ResolvedVersion != "" {
		sortVersion = version.ForSorting(vm.ResolvedVersion)
	}
	_, err = db.db.Exec(ctx,
		`INSERT INTO version_map(
				module_path,
				requested_version,
				resolved_version,
				status,
				error,
				sort_version,
				module_id)
			VALUES($1,$2,$3,$4,$5,$6,(SELECT id FROM modules WHERE module_path=$1 AND version=$3))
			ON CONFLICT (module_path, requested_version)
			DO UPDATE SET
				module_path=excluded.module_path,
				requested_version=excluded.requested_version,
				resolved_version=excluded.resolved_version,
				status=excluded.status,
				error=excluded.error,
				sort_version=excluded.sort_version,
				module_id=excluded.module_id`,
		vm.ModulePath,
		vm.RequestedVersion,
		vm.ResolvedVersion,
		vm.Status,
		vm.Error,
		sortVersion,
	)
	return err
}

// GetVersionMap fetches a version_map entry corresponding to the given path,
// modulePath and requestedVersion.
func (db *DB) GetVersionMap(ctx context.Context, path, modulePath, requestedVersion string) (_ *internal.VersionMap, err error) {
	defer derrors.Wrap(&err, "DB.GetVersionMap(ctx, tx, %q, %q, %q)", path, modulePath, requestedVersion)

	var (
		query string
		args  []interface{}
	)

	if requestedVersion == internal.LatestVersion && modulePath == internal.UnknownModulePath {
		// Return the version_map for the latest resolved_version at
		// the longest module path.
		//
		// In order to determine if a path exists in our database, and
		// the module path corresponding to it, we use
		// packages.tsv_parent_directories when module_path is not
		// specified.
		query = `
			SELECT
				vm.module_path,
				vm.requested_version,
				vm.resolved_version,
				vm.status,
				vm.error
			FROM
				packages p
			INNER JOIN
				version_map vm
			ON
				p.module_path = vm.module_path
				AND p.version = vm.resolved_version
			WHERE
				p.tsv_parent_directories @@ $1::tsquery
			ORDER BY
				module_path DESC,
				sort_version DESC
			LIMIT 1;`
		args = []interface{}{path}
	} else if requestedVersion != internal.LatestVersion && modulePath == internal.UnknownModulePath {
		// Return the version_map for the specified requested version at the
		// longest module path.
		query = `
			SELECT
				vm.module_path,
				vm.requested_version,
				vm.resolved_version,
				vm.status,
				vm.error
			FROM
				packages p
			INNER JOIN
				version_map vm
			ON
				p.module_path = vm.module_path
				AND p.version = vm.resolved_version
			WHERE
				p.tsv_parent_directories @@ $1::tsquery
				AND vm.requested_version = $2
			ORDER BY
				module_path DESC;`
		args = []interface{}{path, requestedVersion}
	} else if requestedVersion == internal.LatestVersion && modulePath != internal.UnknownModulePath {
		// Return the version map for the latest resolved_version and
		// specified module_path.
		query = `
			SELECT
				module_path,
				requested_version,
				resolved_version,
				status,
				error
			FROM
				version_map vm
			WHERE
				module_path = $1
			ORDER BY
				sort_version DESC
			LIMIT 1;`
		args = []interface{}{modulePath}
	} else {
		// Return the version_map for the specified requested version and module path.
		query = `
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
		args = []interface{}{modulePath, requestedVersion}
	}
	var vm internal.VersionMap
	switch db.db.QueryRow(ctx, query, args...).Scan(
		&vm.ModulePath, &vm.RequestedVersion, &vm.ResolvedVersion, &vm.Status, &vm.Error) {
	case nil:
		return &vm, nil
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	default:
		return nil, err
	}
}
