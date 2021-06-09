// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
)

// GetLatestMajorPathForV1Path reports the latest unit path in the series for
// the given v1path. It also returns the major version for that path.
func (db *DB) GetLatestMajorPathForV1Path(ctx context.Context, v1path string) (_ string, _ int, err error) {
	defer derrors.WrapStack(&err, "DB.GetLatestPathForV1Path(ctx, %q)", v1path)
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
	paths := map[string]string{} // from unit path to series path
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
		modPath := strings.TrimSuffix(p, "/"+suffix)
		_, i := internal.SeriesPathAndMajorVersion(modPath)
		if i == 0 {
			return "", 0, fmt.Errorf("bad module path %q", modPath)
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

// upsertPath adds path into the paths table if it does not exist, and returns
// its ID either way.
// It assumes it is running inside a transaction.
func upsertPath(ctx context.Context, tx *database.DB, path string) (id int, err error) {
	// Doing the select first and then the insert led to uniqueness constraint
	// violations even with fully serializable transactions; see
	// https://www.postgresql.org/message-id/CAOqyxwL4E_JmUScYrnwd0_sOtm3bt4c7G%2B%2BUiD2PnmdGJFiqyQ%40mail.gmail.com.
	// If the upsert is done first and then the select, then everything works
	// fine.
	defer derrors.WrapStack(&err, "upsertPath(%q)", path)

	if _, err := tx.Exec(ctx, `LOCK TABLE paths IN EXCLUSIVE MODE`); err != nil {
		return 0, err
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO paths (path) VALUES ($1) ON CONFLICT DO NOTHING RETURNING id`,
		path).Scan(&id)
	if err == sql.ErrNoRows {
		err = tx.QueryRow(ctx,
			`SELECT id FROM paths WHERE path = $1`,
			path).Scan(&id)
		if err == sql.ErrNoRows {
			return 0, errors.New("got no rows; shouldn't happen")
		}
	}
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, errors.New("zero ID")
	}
	return id, nil
}

// upsertPaths adds all the paths to the paths table if they aren't already
// there, and returns their ID either way.
// It assumes it is running inside a transaction.
func upsertPaths(ctx context.Context, db *database.DB, paths []string) (pathToID map[string]int, err error) {
	defer derrors.WrapStack(&err, "upsertPaths(%d paths)", len(paths))

	// Read all existing paths for this module, to avoid a large bulk upsert.
	// (We've seen these bulk upserts hang for so long that they time out (10
	// minutes)).
	pathToID = map[string]int{}
	collect := func(rows *sql.Rows) error {
		var (
			pathID int
			path   string
		)
		if err := rows.Scan(&pathID, &path); err != nil {
			return err
		}
		pathToID[path] = pathID
		return nil
	}

	if err := db.RunQuery(ctx, `SELECT id, path FROM paths WHERE path = ANY($1)`,
		collect, pq.Array(paths)); err != nil {
		return nil, err
	}

	// Insert any unit paths that we don't already have.
	var values []interface{}
	for _, v := range paths {
		if _, ok := pathToID[v]; !ok {
			values = append(values, v)
		}
	}
	if len(values) > 0 {
		// Sort to avoid deadlock.
		sort.Slice(values, func(i, j int) bool { return values[i].(string) < values[j].(string) })
		// Insert data into the paths table.
		pathCols := []string{"path"}
		returningPathCols := []string{"id", "path"}
		if err := db.BulkInsertReturning(ctx, "paths", pathCols, values,
			database.OnConflictDoNothing, returningPathCols, collect); err != nil {
			return nil, err
		}
	}
	return pathToID, nil
}

func GetPathID(ctx context.Context, ddb *database.DB, path string) (id int, err error) {
	err = ddb.QueryRow(ctx,
		`SELECT id FROM paths WHERE path = $1`,
		path).Scan(&id)
	return id, err
}
