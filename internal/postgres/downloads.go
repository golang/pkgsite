// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	"github.com/jackc/pgx/v5"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
)

// DownloadInfo holds daily download and unique IP counts for a module.
type DownloadInfo struct {
	ModulePath string
	Date       civil.Date
	Downloads  int64
	UniqueIPs  int64
}

// UpsertModuleDownloadState inserts or updates a row in module_download_states for date,
// setting tries to 1 on insert and incrementing tries on conflict.
// It returns a derrors.InvalidArgument error if date is invalid.
func (db *DB) UpsertModuleDownloadState(ctx context.Context, date civil.Date, status int, statusErr string) (err error) {
	defer derrors.WrapStack(&err, "UpsertModuleDownloadState(ctx, %s, %d, %q)", date, status, statusErr)

	if !date.IsValid() {
		return fmt.Errorf("invalid date %s: %w", date, derrors.InvalidArgument)
	}

	const query = `
		INSERT INTO module_download_states (date, status, error, tries, updated_at)
		VALUES ($1, $2, $3, 1, CURRENT_TIMESTAMP)
		ON CONFLICT (date) DO UPDATE SET
			status = EXCLUDED.status,
			error = EXCLUDED.error,
			tries = module_download_states.tries + 1,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err = db.db.Exec(ctx, query, date.In(time.UTC), status, statusErr)
	return err
}

// UpsertModuleDownloads inserts or updates download counts in module_downloads.
// Any module paths not already present in the paths table are inserted.
// It returns a derrors.InvalidArgument error if any entry has an invalid date
// or if infos contains duplicate (ModulePath, Date) pairs.
func (db *DB) UpsertModuleDownloads(ctx context.Context, infos []DownloadInfo) (err error) {
	defer derrors.WrapStack(&err, "UpsertModuleDownloads(ctx, [%d infos])", len(infos))

	if len(infos) == 0 {
		return nil
	}

	type key struct {
		path string
		date civil.Date
	}
	seenKeys := make(map[key]bool, len(infos))
	seenPaths := make(map[string]bool, len(infos))
	paths := make([]string, 0, len(infos))
	for _, info := range infos {
		if !info.Date.IsValid() {
			return fmt.Errorf("invalid date %s for module %q: %w", info.Date, info.ModulePath, derrors.InvalidArgument)
		}
		k := key{info.ModulePath, info.Date}
		if seenKeys[k] {
			return fmt.Errorf("duplicate entry for module %q on %s: %w", info.ModulePath, info.Date, derrors.InvalidArgument)
		}
		seenKeys[k] = true
		if !seenPaths[info.ModulePath] {
			seenPaths[info.ModulePath] = true
			paths = append(paths, info.ModulePath)
		}
	}

	// Insert paths first in a separate transaction so locks on the paths table
	// are released before CopyUpsert runs. Without RepeatableRead, upsertPaths
	// can fail to return paths inserted concurrently. For details, see
	// saveModule in insert_module.go and https://golang.org/cl/290269.
	var pathToID map[string]int
	err = db.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		var err error
		pathToID, err = upsertPaths(ctx, tx, paths)
		return err
	})
	if err != nil {
		return err
	}

	return db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
		// Reusing row across calls is safe because pgx's CopyFrom encodes each
		// row before requesting the next one.
		row := make([]any, 4)
		src := pgx.CopyFromSlice(len(infos), func(i int) ([]any, error) {
			info := infos[i]
			id, ok := pathToID[info.ModulePath]
			if !ok {
				return nil, fmt.Errorf("no ID for path %q; shouldn't happen", info.ModulePath)
			}
			row[0] = id
			// Pass time.Time to avoid pgx's slow driver.Valuer string fallback in binary COPY.
			row[1] = info.Date.In(time.UTC)
			row[2] = info.Downloads
			row[3] = info.UniqueIPs
			return row, nil
		})
		columns := []string{"module_path_id", "date", "downloads", "unique_ips"}
		conflictColumns := []string{"module_path_id", "date"}
		return tx.CopyUpsert(ctx, "module_downloads", columns, src, conflictColumns, "")
	})
}
