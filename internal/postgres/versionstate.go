// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/lib/pq"
	"go.opencensus.io/trace"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

// InsertIndexVersions inserts new versions into the module_version_states
// table with a status of zero.
func (db *DB) InsertIndexVersions(ctx context.Context, versions []*internal.IndexVersion) (err error) {
	defer derrors.Wrap(&err, "InsertIndexVersions(ctx, %v)", versions)

	var vals []interface{}
	for _, v := range versions {
		vals = append(vals, v.Path, v.Version, version.ForSorting(v.Version), v.Timestamp, 0, "", "", isIncompatible(v.Version))
	}
	cols := []string{"module_path", "version", "sort_version", "index_timestamp", "status", "error", "go_mod_path", "incompatible"}
	conflictAction := `
		ON CONFLICT
			(module_path, version)
		DO UPDATE SET
			index_timestamp=excluded.index_timestamp,
			next_processed_after=CURRENT_TIMESTAMP`
	return db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
		return tx.BulkInsert(ctx, "module_version_states", cols, vals, conflictAction)
	})
}

// UpsertModuleVersionState inserts or updates the module_version_state table with
// the results of a fetch operation for a given module version.
func (db *DB) UpsertModuleVersionState(ctx context.Context, modulePath, vers, appVersion string, timestamp time.Time, status int, goModPath string, fetchErr error, packageVersionStates []*internal.PackageVersionState) (err error) {
	defer derrors.Wrap(&err, "UpsertModuleVersionState(ctx, %q, %q, %q, %s, %d, %q, %v",
		modulePath, vers, appVersion, timestamp, status, goModPath, fetchErr)
	ctx, span := trace.StartSpan(ctx, "UpsertModuleVersionState")
	defer span.End()

	var numPackages *int
	if !(status >= http.StatusBadRequest && status <= http.StatusNotFound) {
		// If a module was fetched a 40x error in this range, we won't know how
		// many packages it has.
		n := len(packageVersionStates)
		numPackages = &n
	}

	return db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
		if err := upsertModuleVersionState(ctx, tx, modulePath, vers, appVersion, numPackages, timestamp, status, goModPath, fetchErr); err != nil {
			return err
		}
		// Sync modules.status if the module exists in the modules table.
		if err := updateModulesStatus(ctx, tx, modulePath, vers, status); err != nil {
			return err
		}
		if len(packageVersionStates) == 0 {
			return nil
		}
		return upsertPackageVersionStates(ctx, tx, packageVersionStates)
	})
}

func upsertModuleVersionState(ctx context.Context, db *database.DB, modulePath, vers, appVersion string, numPackages *int, timestamp time.Time, status int, goModPath string, fetchErr error) (err error) {
	defer derrors.Wrap(&err, "upsertModuleVersionState(ctx, %q, %q, %q, %s, %d, %q, %v",
		modulePath, vers, appVersion, timestamp, status, goModPath, fetchErr)
	ctx, span := trace.StartSpan(ctx, "upsertModuleVersionState")
	defer span.End()

	var sqlErrorMsg string
	if fetchErr != nil {
		sqlErrorMsg = fetchErr.Error()
	}

	affected, err := db.Exec(ctx, `
			INSERT INTO module_version_states AS mvs (
				module_path,
				version,
				sort_version,
				app_version,
				index_timestamp,
				status,
				go_mod_path,
				error,
				num_packages,
				incompatible)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (module_path, version)
			DO UPDATE
			SET
				app_version=excluded.app_version,
				status=excluded.status,
				go_mod_path=excluded.go_mod_path,
				error=excluded.error,
				num_packages=excluded.num_packages,
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
		appVersion, timestamp, status, goModPath, sqlErrorMsg, numPackages, isIncompatible(vers))
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("module version state update affected %d rows, expected exactly 1", affected)
	}
	return nil
}

// updateModulesStatus updates the status of the module with the given modulePath
// and version, if it exists, in the modules table.
func updateModulesStatus(ctx context.Context, db *database.DB, modulePath, version string, status int) (err error) {
	defer derrors.Wrap(&err, "updateModulesStatus(%q, %q, %d)", modulePath, version, status)

	query := `UPDATE modules
			SET
				status = $1
			WHERE
				module_path = $2
				AND version = $3;`
	affected, err := db.Exec(ctx, query, status, modulePath, version)
	if err != nil {
		return err
	}
	if affected > 1 {
		return fmt.Errorf("module status update affected %d rows, expected at most 1", affected)
	}
	return nil
}

func upsertPackageVersionStates(ctx context.Context, db *database.DB, packageVersionStates []*internal.PackageVersionState) (err error) {
	defer derrors.Wrap(&err, "upsertPackageVersionStates")
	ctx, span := trace.StartSpan(ctx, "upsertPackageVersionStates")
	defer span.End()

	sort.Slice(packageVersionStates, func(i, j int) bool {
		return packageVersionStates[i].PackagePath < packageVersionStates[j].PackagePath
	})
	var vals []interface{}
	for _, pvs := range packageVersionStates {
		vals = append(vals, pvs.PackagePath, pvs.ModulePath, pvs.Version, pvs.Status, pvs.Error)
	}
	return db.BulkInsert(ctx, "package_version_states",
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
			go_mod_path,
			num_packages`

// scanModuleVersionState constructs an *internal.ModuleModuleVersionState from the given
// scanner. It expects columns to be in the order of moduleVersionStateColumns.
func scanModuleVersionState(scan func(dest ...interface{}) error) (*internal.ModuleVersionState, error) {
	var (
		v               internal.ModuleVersionState
		lastProcessedAt pq.NullTime
		numPackages     sql.NullInt64
	)
	if err := scan(&v.ModulePath, &v.Version, &v.IndexTimestamp, &v.CreatedAt, &v.Status, &v.Error,
		&v.TryCount, &v.LastProcessedAt, &v.NextProcessedAfter, &v.AppVersion, &v.GoModPath, &numPackages); err != nil {
		return nil, err
	}
	if lastProcessedAt.Valid {
		lp := lastProcessedAt.Time
		v.LastProcessedAt = &lp
	}
	if numPackages.Valid {
		n := int(numPackages.Int64)
		v.NumPackages = &n
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

// GetRecentFailedVersions returns versions that have most recently failed.
func (db *DB) GetRecentFailedVersions(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	defer derrors.Wrap(&err, "GetRecentFailedVersions(ctx, %d)", limit)

	queryFormat := `
		SELECT %s
		FROM
			module_version_states
		WHERE status=500
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
