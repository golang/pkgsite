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
)

// GetPathInfo returns information about the "best" entity (module, path or directory) with
// the given path. The module and version arguments provide additional constraints.
// If the module is unknown, pass internal.UnknownModulePath; if the version is unknown, pass
// internal.LatestVersion.
//
// The rules for picking the best are:
// 1. Match the module path and or version, if they are provided;
// 2. Prefer newer module versions to older, and release to pre-release;
// 3. In the unlikely event of two paths at the same version, pick the longer module path.
func (db *DB) GetPathInfo(ctx context.Context, path, inModulePath, inVersion string) (outModulePath, outVersion string, isPackage bool, err error) {
	defer derrors.Wrap(&err, "DB.GetPathInfo(ctx, %q, %q, %q)", path, inModulePath, inVersion)

	var constraints []string
	args := []interface{}{path}
	if inModulePath != internal.UnknownModulePath {
		constraints = append(constraints, fmt.Sprintf("AND m.module_path = $%d", len(args)+1))
		args = append(args, inModulePath)
	}
	if inVersion != internal.LatestVersion {
		constraints = append(constraints, fmt.Sprintf("AND m.version = $%d", len(args)+1))
		args = append(args, inVersion)
	}
	query := fmt.Sprintf(`
		SELECT m.module_path, m.version, p.name != ''
		FROM paths p
		INNER JOIN modules m ON (p.module_id = m.id)
		WHERE p.path = $1
		%s
		ORDER BY
			m.version_type = 'release' DESC,
			m.sort_version DESC,
			m.module_path DESC
		LIMIT 1
	`, strings.Join(constraints, " "))
	err = db.db.QueryRow(ctx, query, args...).Scan(&outModulePath, &outVersion, &isPackage)
	switch err {
	case sql.ErrNoRows:
		return "", "", false, derrors.NotFound
	case nil:
		return outModulePath, outVersion, isPackage, nil
	default:
		return "", "", false, err
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

func (db *DB) getPathsInModule(ctx context.Context, modulePath, version string) (_ []*dbPath, err error) {
	defer derrors.Wrap(&err, "DB.getPathsInModule(ctx, %q, %q)", modulePath, version)
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
	if err := db.db.RunQuery(ctx, query, collect, modulePath, version); err != nil {
		return nil, err
	}
	return paths, nil
}
