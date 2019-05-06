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
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
)

// InsertIndexVersions inserts new versions into the module_version_states
// table.
func (db *DB) InsertIndexVersions(ctx context.Context, versions []*internal.IndexVersion) error {
	var vals []interface{}
	for _, v := range versions {
		vals = append(vals, v.Path, v.Version, v.Timestamp)
	}
	cols := []string{"module_path", "version", "index_timestamp"}
	return db.Transact(func(tx *sql.Tx) error {
		return bulkInsert(ctx, tx, "module_version_states", cols, vals, true)
	})
}

// LatestIndexTimestamp returns the last timestamp successfully inserted into
// the module_version_states table.
func (db *DB) LatestIndexTimestamp(ctx context.Context) (time.Time, error) {
	query := `SELECT index_timestamp
		FROM module_version_states
		ORDER BY index_timestamp DESC
		LIMIT 1`

	var ts time.Time
	row := db.QueryRowContext(ctx, query)
	switch err := row.Scan(&ts); err {
	case sql.ErrNoRows:
		return time.Time{}, nil
	case nil:
		return ts, nil
	default:
		return time.Time{}, err
	}
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
			next_processed_after`

// scanVersionState constructs an *internal.VersionState from the given
// scanner. It expects columns to be in the order of versionStateColumns.
func scanVersionState(scan func(dest ...interface{}) error) (*internal.VersionState, error) {
	var (
		modulePath, version                           string
		indexTimestamp, createdAt, nextProcessedAfter time.Time
		lastProcessedAt                               pq.NullTime
		status                                        sql.NullInt64
		errorMsg                                      sql.NullString
		tryCount                                      int
	)
	if err := scan(&modulePath, &version, &indexTimestamp, &createdAt, &status, &errorMsg,
		&tryCount, &lastProcessedAt, &nextProcessedAfter); err != nil {
		return nil, err
	}
	v := &internal.VersionState{
		ModulePath:         modulePath,
		Version:            version,
		IndexTimestamp:     indexTimestamp,
		CreatedAt:          createdAt,
		TryCount:           tryCount,
		NextProcessedAfter: nextProcessedAfter,
	}
	if status.Valid {
		s := int(status.Int64)
		v.Status = &s
	}
	if errorMsg.Valid {
		s := errorMsg.String
		v.Error = &s
	}
	if lastProcessedAt.Valid {
		lp := lastProcessedAt.Time
		v.LastProcessedAt = &lp
	}
	return v, nil
}

// queryVersionStates executes a query for VersionState rows. It expects the
// given queryFormat be a format specifier with exactly one argument: a %s verb
// for the query columns.
func (db *DB) queryVersionStates(ctx context.Context, queryFormat string, args ...interface{}) ([]*internal.VersionState, error) {
	query := fmt.Sprintf(queryFormat, versionStateColumns)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %v): %v", query, args, err)
	}

	var versions []*internal.VersionState
	for rows.Next() {
		v, err := scanVersionState(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("rows.Scan(): %v", err)
		}
		versions = append(versions, v)
	}

	return versions, nil
}

// GetNextVersionsToFetch returns the next batch of versions that must be
// processed.
func (db *DB) GetNextVersionsToFetch(ctx context.Context, limit int) ([]*internal.VersionState, error) {
	queryFormat := `
		SELECT %s
		FROM
			module_version_states
		WHERE
			(status IS NULL OR status >= 500)
			AND next_processed_after < CURRENT_TIMESTAMP
		ORDER BY
			next_processed_after ASC, index_timestamp DESC
		LIMIT $1`
	return db.queryVersionStates(ctx, queryFormat, limit)
}

// GetRecentFailedVersions returns versions that have most recently failed.
func (db *DB) GetRecentFailedVersions(ctx context.Context, limit int) ([]*internal.VersionState, error) {
	queryFormat := `
		SELECT %s
		FROM
			module_version_states
		WHERE
		  (status >= 400)
		ORDER BY last_processed_at DESC
		LIMIT $1`
	return db.queryVersionStates(ctx, queryFormat, limit)
}

// encodeDuration formats the given duration for use in Postgres queries. It
// encodes with microsecond precision.
func encodeDuration(d time.Duration) string {
	const usPerSecond = int64(time.Second / time.Microsecond)

	prefix := ""
	micros := int64(d / time.Microsecond)
	if micros < 0 {
		prefix = "-"
		micros = -micros
	}
	seconds, micros := micros/usPerSecond, micros%usPerSecond
	return fmt.Sprintf("%s%d.%d", prefix, seconds, micros)
}

// UpdateVersionState updates a version state following a call to the fetch
// service.
func (db *DB) UpdateVersionState(ctx context.Context, modulePath, version string, status int, errorMsg string, backOff time.Duration) error {
	stmt := `
		UPDATE
			module_version_states
		SET
			status=$1,
			error=$2,
			try_count=try_count+1,
			last_processed_at=CURRENT_TIMESTAMP,
			next_processed_after=CASE
				WHEN last_processed_at IS NULL
					THEN CURRENT_TIMESTAMP + $5
				ELSE
					CURRENT_TIMESTAMP + $5
				END
		WHERE
			module_path = $3
			AND version = $4;`
	result, err := db.ExecContext(ctx, stmt, status, errorMsg, modulePath, version, encodeDuration(backOff))
	if err != nil {
		return fmt.Errorf("db.ExecContext(ctx, %q, %q, %q, %q): %v", stmt, errorMsg, modulePath, version, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("result.RowsAffected(): %v", err)
	}
	if affected != 1 {
		return fmt.Errorf("version state update affected %d rows, expected exactly 1", affected)
	}
	return nil
}

// GetVersionState returns the current version state for modulePath and
// version.
func (db *DB) GetVersionState(ctx context.Context, modulePath, version string) (*internal.VersionState, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM
			module_version_states
		WHERE
			module_path = $1
			AND version = $2;`, versionStateColumns)

	row := db.QueryRowContext(ctx, query, modulePath, version)
	v, err := scanVersionState(row.Scan)
	switch err {
	case nil:
		return v, nil
	case sql.ErrNoRows:
		return nil, derrors.NotFound("version not found")
	default:
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
}
