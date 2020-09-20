// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/stdlib"
)

const orderByLatest = `
			ORDER BY
				m.incompatible,
				CASE
				    -- Order the versions by release then prerelease then pseudo.
				    WHEN m.version_type = 'release' THEN 1
				    WHEN m.version_type = 'prerelease' THEN 2
				    ELSE 3
				END,
				m.sort_version DESC,
				m.module_path DESC`

// GetUnitMeta returns information about the "best" entity (module, path or directory) with
// the given path. The module and version arguments provide additional constraints.
// If the module is unknown, pass internal.UnknownModulePath; if the version is unknown, pass
// internal.LatestVersion.
//
// The rules for picking the best are:
// 1. Match the module path and or version, if they are provided;
// 2. Prefer newer module versions to older, and release to pre-release;
// 3. In the unlikely event of two paths at the same version, pick the longer module path.
func (db *DB) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "DB.GetUnitMeta(ctx, %q, %q, %q)", path, requestedModulePath, requestedVersion)

	var (
		constraints []string
		joinStmt    string
	)
	args := []interface{}{path}
	if requestedModulePath != internal.UnknownModulePath {
		constraints = append(constraints, fmt.Sprintf("AND m.module_path = $%d", len(args)+1))
		args = append(args, requestedModulePath)
	}
	switch requestedVersion {
	case internal.LatestVersion:
	case internal.MasterVersion:
		joinStmt = "INNER JOIN version_map vm ON (vm.module_id = m.id)"
		constraints = append(constraints, "AND vm.requested_version = 'master'")
	default:
		constraints = append(constraints, fmt.Sprintf("AND m.version = $%d", len(args)+1))
		args = append(args, requestedVersion)
	}

	var (
		licenseTypes []string
		licensePaths []string
		um           = internal.UnitMeta{Path: path}
	)
	query := fmt.Sprintf(`
		SELECT
		    m.module_path,
		    m.version,
		    m.commit_time,
		    m.source_info,
		    p.name,
		    p.redistributable,
		    p.license_types,
		    p.license_paths
		FROM paths p
		INNER JOIN modules m ON (p.module_id = m.id)
		%s
		WHERE p.path = $1
		%s
		%s
		LIMIT 1
	`, joinStmt, strings.Join(constraints, " "), orderByLatest)
	err = db.db.QueryRow(ctx, query, args...).Scan(
		&um.ModulePath,
		&um.Version,
		&um.CommitTime,
		jsonbScanner{&um.SourceInfo},
		&um.Name,
		&um.IsRedistributable,
		pq.Array(&licenseTypes),
		pq.Array(&licensePaths))
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return nil, err
		}
		um.Licenses = lics
		return &um, nil
	default:
		return nil, err
	}
}

type dbPath struct {
	id              int64
	path            string
	moduleID        int64
	v1Path          string
	name            string
	licenseTypes    []string
	licensePaths    []string
	redistributable bool
}

func (db *DB) getPathsInModule(ctx context.Context, modulePath, resolvedVersion string) (_ []*dbPath, err error) {
	defer derrors.Wrap(&err, "DB.getPathsInModule(ctx, %q, %q)", modulePath, resolvedVersion)
	query := `
	SELECT
		p.id,
		p.path,
		p.module_id,
		p.v1_path,
		p.name,
		p.license_types,
		p.license_paths,
		p.redistributable
	FROM
		paths p
	INNER JOIN
		modules m
	ON
		p.module_id = m.id
	WHERE
		m.module_path = $1
		AND m.version = $2
	ORDER BY path;`

	var paths []*dbPath
	collect := func(rows *sql.Rows) error {
		var p dbPath
		if err := rows.Scan(&p.id, &p.path, &p.moduleID, &p.v1Path, &p.name, pq.Array(&p.licenseTypes),
			pq.Array(&p.licensePaths), &p.redistributable); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		paths = append(paths, &p)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, modulePath, resolvedVersion); err != nil {
		return nil, err
	}
	return paths, nil
}

// GetStdlibPathsWithSuffix returns information about all paths in the latest version of the standard
// library whose last component is suffix. A path that exactly match suffix is not included;
// the path must end with "/" + suffix.
//
// We are only interested in actual standard library packages: not commands, which we happen to include
// in the stdlib module, and not directories (paths that do not contain a package).
func (db *DB) GetStdlibPathsWithSuffix(ctx context.Context, suffix string) (paths []string, err error) {
	defer derrors.Wrap(&err, "DB.GetStdlibPaths(ctx, %q)", suffix)

	q := `
		SELECT path
		FROM paths
		WHERE module_id = (
			-- latest release version of stdlib
			SELECT id
			FROM modules
			WHERE module_path = $1
			ORDER BY
				version_type = 'release' DESC,
				sort_version DESC
			LIMIT 1)
			AND name != ''
			AND path NOT LIKE 'cmd/%'
			AND path LIKE '%/' || $2
		ORDER BY path
	`
	err = db.db.RunQuery(ctx, q, func(rows *sql.Rows) error {
		var p string
		if err := rows.Scan(&p); err != nil {
			return err
		}
		paths = append(paths, p)
		return nil
	}, stdlib.ModulePath, suffix)
	if err != nil {
		return nil, err
	}
	return paths, nil
}
