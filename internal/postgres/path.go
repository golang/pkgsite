// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
)

// GetLatestMajorPathForV1Path reports the latest unit path in the series for
// the given v1path.
func (db *DB) GetLatestMajorPathForV1Path(ctx context.Context, v1path string) (_ string, _ int, err error) {
	defer derrors.Wrap(&err, "DB.GetLatestPathForV1Path(ctx, %q)", v1path)
	q := `
		SELECT p.path, m.series_path
		FROM paths p
		INNER JOIN units u ON u.path_id = p.id
		INNER JOIN modules m ON u.module_id = m.id
		WHERE u.v1path_id = (
			SELECT p.id
			FROM paths p
			INNER JOIN units u ON u.v1path_id = p.id
			WHERE p.path = $1
			ORDER BY p.path DESC
			LIMIT 1
		);`
	paths := map[string]string{}
	err = db.db.RunQuery(ctx, q, func(rows *sql.Rows) error {
		var p, sp string
		if err := rows.Scan(&p, &sp); err != nil {
			return err
		}
		paths[p] = sp
		return nil
	}, v1path)
	if err != nil {
		return "", 0, err
	}

	var (
		maj     int
		majPath string
	)
	for p, sp := range paths {
		// Trim the series path and suffix from the unit path.
		// Keep only the N following vN.
		suffix := internal.Suffix(v1path, sp)
		v := strings.TrimSuffix(strings.TrimPrefix(
			strings.TrimSuffix(strings.TrimPrefix(p, sp), suffix), "/v"), "/")
		var i int
		if v != "" {
			i, err = strconv.Atoi(v)
			if err != nil {
				return "", 0, fmt.Errorf("strconv.Atoi(%q): %v", v, err)
			}
		}
		if maj <= i {
			maj = i
			majPath = p
		}
	}
	if maj == 0 {
		// Return 1 as the major version for all v0 or v1 majPaths.
		maj = 1
	}
	return majPath, maj, nil
}
