// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.opencensus.io/trace"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/log"
	"golang.org/x/discovery/internal/version"
	"golang.org/x/mod/semver"
)

// InsertIndexVersions inserts new versions into the module_version_states
// table.
func (db *DB) InsertIndexVersions(ctx context.Context, versions []*internal.IndexVersion) (err error) {
	defer derrors.Wrap(&err, "InsertIndexVersions(ctx, %v)", versions)

	var vals []interface{}
	for _, v := range versions {
		vals = append(vals, v.Path, v.Version, version.ForSorting(v.Version), v.Timestamp, 0, "", "")
	}
	cols := []string{"module_path", "version", "sort_version", "index_timestamp", "status", "error", "go_mod_path"}
	conflictAction := `
		ON CONFLICT
			(module_path, version)
		DO UPDATE SET
			index_timestamp=excluded.index_timestamp,
			next_processed_after=CURRENT_TIMESTAMP`
	return db.db.Transact(ctx, func(tx *database.DB) error {
		return tx.BulkInsert(ctx, "module_version_states", cols, vals, conflictAction)
	})
}

// UpsertModuleVersionState inserts or updates the module_version_state table with
// the results of a fetch operation for a given module version.
func (db *DB) UpsertModuleVersionState(ctx context.Context, modulePath, vers, appVersion string, timestamp time.Time, status int, goModPath string, fetchErr error, packageVersionStates []*internal.PackageVersionState) (err error) {
	derrors.Wrap(&err, "UpsertModuleVersionState(ctx, %q, %q, %q, %s, %d, %q, %v",
		modulePath, vers, appVersion, timestamp, status, goModPath, fetchErr)

	ctx, span := trace.StartSpan(ctx, "UpsertModuleVersionState")
	defer span.End()

	return db.db.Transact(ctx, func(tx *database.DB) error {
		var sqlErrorMsg string
		if fetchErr != nil {
			sqlErrorMsg = fetchErr.Error()
		}

		result, err := tx.Exec(ctx, `
			INSERT INTO module_version_states AS mvs (
				module_path,
				version,
				sort_version,
				app_version,
				index_timestamp,
				status,
				go_mod_path,
				error)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (module_path, version)
			DO UPDATE
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
					END;`,
			modulePath, vers, version.ForSorting(vers),
			appVersion, timestamp, status, goModPath, sqlErrorMsg)
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
		if len(packageVersionStates) == 0 {
			return nil
		}

		var vals []interface{}
		for _, pvs := range packageVersionStates {
			vals = append(vals, pvs.PackagePath, pvs.ModulePath, pvs.Version, pvs.Status, pvs.Error)
		}
		return tx.BulkInsert(ctx, "package_version_states",
			[]string{
				"package_path",
				"module_path",
				"version",
				"status",
				"error",
			},
			vals,
			`ON CONFLICT (module_path, package_path, version)
				DO UPDATE
				SET
					package_path=excluded.package_path,
					module_path=excluded.module_path,
					version=excluded.version,
					status=excluded.status,
					error=excluded.error`)
	})
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

const moduleVersionStateColumns = `
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
// scanner. It expects columns to be in the order of moduleVersionStateColumns.
func scanModuleVersionState(scan func(dest ...interface{}) error) (*internal.ModuleVersionState, error) {
	var (
		v               internal.ModuleVersionState
		lastProcessedAt pq.NullTime
	)
	if err := scan(&v.ModulePath, &v.Version, &v.IndexTimestamp, &v.CreatedAt, &v.Status, &v.Error,
		&v.TryCount, &v.LastProcessedAt, &v.NextProcessedAfter, &v.AppVersion, &v.GoModPath); err != nil {
		return nil, err
	}
	if lastProcessedAt.Valid {
		lp := lastProcessedAt.Time
		v.LastProcessedAt = &lp
	}
	return &v, nil
}

// queryModuleVersionStates executes a query for ModuleModuleVersionState rows. It expects the
// given queryFormat be a format specifier with exactly one argument: a %s verb
// for the query columns.
func (db *DB) queryModuleVersionStates(ctx context.Context, queryFormat string, args ...interface{}) ([]*internal.ModuleVersionState, error) {
	query := fmt.Sprintf(queryFormat, moduleVersionStateColumns)
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
func (db *DB) GetNextVersionsToFetch(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	// We want to prioritize the latest versions over other ones, and we want to
	// leave time-consuming modules until the end.
	// We run two queries: the first gets the latest versions of everything; the second
	// runs through all eligible modules, organizing them by priority.
	defer derrors.Wrap(&err, "GetNextVersionsToFetch(ctx, %d)", limit)

	latestVersions, err := db.getLatestVersionsFromModuleVersionStates(ctx)
	if err != nil {
		return nil, err
	}

	// isBig reports whether the module path refers to a big module that takes a
	// long time to process.
	isBig := func(path string) bool {
		for _, s := range []string{"kubernetes", "aws-sdk-go"} {
			if strings.HasSuffix(path, s) {
				return true
			}
		}
		return false
	}

	// Create prioritized lists of modules to process. From high to low:
	// 0: latest version, release
	// 1: latest version, non-release
	// 2: not a large zip
	// 3: the rest
	mvs := make([][]*internal.ModuleVersionState, 4)

	query := fmt.Sprintf(`
		SELECT
			%s
		FROM
			module_version_states
		WHERE
			(status=0 OR status >= 500)
		AND
			next_processed_after < CURRENT_TIMESTAMP
		ORDER BY
			sort_version DESC
	`, moduleVersionStateColumns)

	err = db.db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		// If the highest-priority list is full, we're done.
		if len(mvs[0]) >= limit {
			return io.EOF
		}
		mv, err := scanModuleVersionState(rows.Scan)
		if err != nil {
			return err
		}
		var prio int
		switch {
		case mv.Version == latestVersions[mv.ModulePath]:
			if semver.Prerelease(mv.Version) == "" {
				prio = 0 // latest release version
			} else {
				prio = 1 // latest non-release version
			}
		case !isBig(mv.ModulePath):
			prio = 2
		default:
			prio = 3
		}
		mvs[prio] = append(mvs[prio], mv)
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}

	// Combine the four prioritized lists into one.
	var r []*internal.ModuleVersionState
	for _, mv := range mvs {
		if len(r)+len(mv) > limit {
			return append(r, mv[:limit-len(r)]...), nil
		}
		r = append(r, mv...)
	}
	return r, nil
}

// getLatestVersions returns a map from module path to latest version in module_version_states.
func (db *DB) getLatestVersionsFromModuleVersionStates(ctx context.Context) (map[string]string, error) {
	m := map[string]string{}
	// We want to prefer release to non-release versions. A sort_version will end in '~' if it
	// is a release, and that is larger than any other character that can occur in a sort_version.
	// So if we sort first by the last character in sort_version, then by sort_version itself,
	// we will get releases before non-releases.
	//   To implement that two-level ordering in a MAX, we construct an array of the two strings.
	// Arrays are ordered lexicographically, so MAX will do just what we want.
	err := db.db.RunQuery(ctx, `
		SELECT
			s.module_path, s.version
		FROM
			module_version_states s
		INNER JOIN (
			SELECT module_path,
			MAX(ARRAY[right(sort_version, 1), sort_version]) AS mv
			FROM module_version_states
			GROUP BY 1) m
		ON
			s.module_path = m.module_path
		AND
			s.sort_version = m.mv[2]`,
		func(rows *sql.Rows) error {
			var mp, v string
			if err := rows.Scan(&mp, &v); err != nil {
				return err
			}
			m[mp] = v
			return nil
		})
	if err != nil {
		return nil, err
	}
	return m, nil
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
			AND version = $2;`, moduleVersionStateColumns)

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

// GetPackageVersionStatesForModule returns the current package version states
// for modulePath and version.
func (db *DB) GetPackageVersionStatesForModule(ctx context.Context, modulePath, version string) (_ []*internal.PackageVersionState, err error) {
	defer derrors.Wrap(&err, "GetPackageVersionState(ctx, %q, %q)", modulePath, version)

	query := `
		SELECT
			package_path,
			module_path,
			version,
			status,
			error
		FROM
			package_version_states
		WHERE
			module_path = $1
			AND version = $2;`

	var states []*internal.PackageVersionState
	collect := func(rows *sql.Rows) error {
		var s internal.PackageVersionState
		if err := rows.Scan(&s.PackagePath, &s.ModulePath, &s.Version,
			&s.Status, &s.Error); err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		states = append(states, &s)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, modulePath, version); err != nil {
		return nil, err
	}
	return states, nil
}

// GetPackageVersionState returns the current package version state for
// pkgPath, modulePath and version.
func (db *DB) GetPackageVersionState(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.PackageVersionState, err error) {
	defer derrors.Wrap(&err, "GetPackageVersionState(ctx, %q, %q, %q)", pkgPath, modulePath, version)

	query := `
		SELECT
			package_path,
			module_path,
			version,
			status,
			error
		FROM
			package_version_states
		WHERE
			package_path = $1
			AND module_path = $2
			AND version = $3;`

	var pvs internal.PackageVersionState
	err = db.db.QueryRow(ctx, query, pkgPath, modulePath, version).Scan(
		&pvs.PackagePath, &pvs.ModulePath, &pvs.Version,
		&pvs.Status, &pvs.Error)
	switch err {
	case nil:
		return &pvs, nil
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
