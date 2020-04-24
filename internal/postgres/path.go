// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal/derrors"
)

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
