// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/version"
)

// InsertIndexVersions inserts new versions into the module_version_states
// table.
func (db *DB) InsertIndexVersions(ctx context.Context, versions []*internal.IndexVersion) (err error) {
	defer derrors.Wrap(&err, "InsertIndexVersions(ctx, %v)", versions)

	var vals []interface{}
	for _, v := range versions {
		vals = append(vals, v.Path, v.Version, version.ForSorting(v.Version), v.Timestamp)
	}
	cols := []string{"module_path", "version", "sort_version", "index_timestamp"}
	conflictAction := `
		ON CONFLICT
			(module_path, version)
		DO UPDATE SET
			index_timestamp=excluded.index_timestamp,
			next_processed_after=CURRENT_TIMESTAMP`
	return db.db.Transact(func(tx *sql.Tx) error {
		return database.BulkInsert(ctx, tx, "module_version_states", cols, vals, conflictAction)
	})
}

// UpsertModuleVersionState inserts or updates the module_version_state table with
// the results of a fetch operation for a given module version.
func (db *DB) UpsertModuleVersionState(ctx context.Context, modulePath, vers, appVersion string, timestamp time.Time, status int, goModPath string, fetchErr error) (err error) {
	derrors.Wrap(&err, "UpsertModuleVersionState(ctx, %q, %q, %q, %s, %d, %q, %v",
		modulePath, vers, appVersion, timestamp, status, goModPath, fetchErr)

	ctx, span := trace.StartSpan(ctx, "UpsertModuleVersionState")
	defer span.End()
	query := `
		INSERT INTO module_version_states AS mvs (module_path, version, sort_version, app_version, index_timestamp, status, go_mod_path, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (module_path, version) DO UPDATE
			SET
				app_version=excluded.app_version,
				status=excluded.status,
				go_mod_path=excluded.go_mod_path,
				error=excluded.error,
				try_count=mvs.try_count+1,
				last_processed_at=CURRENT_TIMESTAMP,
			    -- back off exponentially until 1 hour, then at constant 1-hour intervals
				next_processed_after=CASE
					WHEN mvs.last_processed_at IS NULL THEN
						CURRENT_TIMESTAMP + INTERVAL '1 minute'
					WHEN 2*(mvs.next_processed_after - mvs.last_processed_at) < INTERVAL '1 hour' THEN
						CURRENT_TIMESTAMP + 2*(mvs.next_processed_after - mvs.last_processed_at)
					ELSE
						CURRENT_TIMESTAMP + INTERVAL '1 hour'
					END;`

	var sqlErrorMsg sql.NullString
	if fetchErr != nil {
		sqlErrorMsg = sql.NullString{Valid: true, String: fetchErr.Error()}
	}
	result, err := db.db.Exec(ctx, query,
		modulePath, vers, version.ForSorting(vers), appVersion, timestamp, status, goModPath, sqlErrorMsg)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("result.RowsAffected(): %v", err)
	}
	if affected != 1 {
		return fmt.Errorf("module version state update affected %d rows, expected exactly 1", affected)
	}
	return nil
}

// LatestIndexTimestamp returns the last timestamp successfully inserted into
// the module_version_states table.
func (db *DB) LatestIndexTimestamp(ctx context.Context) (_ time.Time, err error) {
	defer derrors.Wrap(&err, "LatestIndexTimestamp(ctx)")

	query := `SELECT index_timestamp
		FROM module_version_states
		ORDER BY index_timestamp DESC
		LIMIT 1`

	var ts time.Time
	row := db.db.QueryRow(ctx, query)
	switch err := row.Scan(&ts); err {
	case sql.ErrNoRows:
		return time.Time{}, nil
	case nil:
		return ts, nil
	default:
		return time.Time{}, err
	}
}

func (db *DB) UpdateModuleVersionStatesForReprocessing(ctx context.Context, appVersion string) (err error) {
	defer derrors.Wrap(&err, "UpdateModuleVersionStatesForReprocessing(ctx, %q)", appVersion)

	query := `
		UPDATE module_version_states
		SET
			status = 505,
			next_processed_after = CURRENT_TIMESTAMP,
			last_processed_at = NULL
		WHERE
			app_version <= $1;`
	result, err := db.db.Exec(ctx, query, appVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("result.RowsAffected(): %v", err)
	}
	log.Infof(ctx, "Updated %d module version states to be reprocessed for app_version <= %q", affected, appVersion)
	return nil
}

const versionStateColumns = `
			module_path,
			version,
			index_timestamp,
			created_at,
			status,
			error,
			try_count,
			last_processed_at,
			next_processed_after,
			app_version,
			go_mod_path`

// scanModuleVersionState constructs an *internal.ModuleModuleVersionState from the given
// scanner. It expects columns to be in the order of versionStateColumns.
func scanModuleVersionState(scan func(dest ...interface{}) error) (*internal.ModuleVersionState, error) {
	var (
		modulePath, version, appVersion               string
		indexTimestamp, createdAt, nextProcessedAfter time.Time
		lastProcessedAt                               pq.NullTime
		status                                        sql.NullInt64
		errorMsg, goModPath                           sql.NullString
		tryCount                                      int
	)
	if err := scan(&modulePath, &version, &indexTimestamp, &createdAt, &status, &errorMsg,
		&tryCount, &lastProcessedAt, &nextProcessedAfter, &appVersion, &goModPath); err != nil {
		return nil, err
	}
	v := &internal.ModuleVersionState{
		ModulePath:         modulePath,
		Version:            version,
		IndexTimestamp:     indexTimestamp,
		CreatedAt:          createdAt,
		TryCount:           tryCount,
		NextProcessedAfter: nextProcessedAfter,
		AppVersion:         appVersion,
	}
	if status.Valid {
		s := int(status.Int64)
		v.Status = &s
	}
	if errorMsg.Valid {
		s := errorMsg.String
		v.Error = &s
	}
	if goModPath.Valid {
		s := goModPath.String
		v.GoModPath = &s
	}
	if lastProcessedAt.Valid {
		lp := lastProcessedAt.Time
		v.LastProcessedAt = &lp
	}
	return v, nil
}

// queryModuleVersionStates executes a query for ModuleModuleVersionState rows. It expects the
// given queryFormat be a format specifier with exactly one argument: a %s verb
// for the query columns.
func (db *DB) queryModuleVersionStates(ctx context.Context, queryFormat string, args ...interface{}) ([]*internal.ModuleVersionState, error) {
	query := fmt.Sprintf(queryFormat, versionStateColumns)
	rows, err := db.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*internal.ModuleVersionState
	for rows.Next() {
		v, err := scanModuleVersionState(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}
		versions = append(versions, v)
	}

	return versions, nil
}

// GetNextVersionsToFetch returns the next batch of versions that must be
// processed.
// Prefer release versions to prerelease, and higher versions to lower.
func (db *DB) GetNextVersionsToFetch(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	defer derrors.Wrap(&err, "GetNextVersionsToFetch(ctx, %d)", limit)

	queryFormat := `
		SELECT %s
		FROM
			module_version_states
		WHERE
			(status IS NULL OR status >= 500)
			AND next_processed_after < CURRENT_TIMESTAMP
		ORDER BY
			right(sort_version, 1) = '~' DESC,
			sort_version DESC
		LIMIT $1`
	return db.queryModuleVersionStates(ctx, queryFormat, limit)
}

// GetRecentFailedVersions returns versions that have most recently failed.
func (db *DB) GetRecentFailedVersions(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	defer derrors.Wrap(&err, "GetRecentFailedVersions(ctx, %d)", limit)

	queryFormat := `
		SELECT %s
		FROM
			module_version_states
		WHERE
		  (status >= 400)
		ORDER BY last_processed_at DESC
		LIMIT $1`
	return db.queryModuleVersionStates(ctx, queryFormat, limit)
}

// GetRecentVersions returns recent versions that have been processed.
func (db *DB) GetRecentVersions(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	defer derrors.Wrap(&err, "GetRecentVersions(ctx, %d)", limit)

	queryFormat := `
		SELECT %s
		FROM
			module_version_states
		ORDER BY created_at DESC
		LIMIT $1`
	return db.queryModuleVersionStates(ctx, queryFormat, limit)
}

// GetModuleVersionState returns the current module version state for
// modulePath and version.
func (db *DB) GetModuleVersionState(ctx context.Context, modulePath, version string) (_ *internal.ModuleVersionState, err error) {
	defer derrors.Wrap(&err, "GetModuleVersionState(ctx, %q, %q)", modulePath, version)

	query := fmt.Sprintf(`
		SELECT %s
		FROM
			module_version_states
		WHERE
			module_path = $1
			AND version = $2;`, versionStateColumns)

	row := db.db.QueryRow(ctx, query, modulePath, version)
	v, err := scanModuleVersionState(row.Scan)
	switch err {
	case nil:
		return v, nil
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	default:
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
}

// VersionStats holds statistics extracted from the module_version_states
// table.
type VersionStats struct {
	LatestTimestamp time.Time
	VersionCounts   map[int]int // from status to number of rows
}

// GetVersionStats queries the module_version_states table for aggregate
// information about the current state of module versions, grouping them by
// their current status code.
func (db *DB) GetVersionStats(ctx context.Context) (_ *VersionStats, err error) {
	defer derrors.Wrap(&err, "GetVersionStats(ctx)")

	query := `
		SELECT
			status,
			max(index_timestamp),
			count(*)
		FROM
			module_version_states
		GROUP BY status;`

	stats := &VersionStats{
		VersionCounts: make(map[int]int),
	}
	err = db.db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var (
			status         sql.NullInt64
			indexTimestamp time.Time
			count          int
		)
		if err := rows.Scan(&status, &indexTimestamp, &count); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		if indexTimestamp.After(stats.LatestTimestamp) {
			stats.LatestTimestamp = indexTimestamp
		}
		stats.VersionCounts[int(status.Int64)] = count
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}
