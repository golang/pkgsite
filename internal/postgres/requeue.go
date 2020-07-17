// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// UpdateModuleVersionStatesForReprocessing marks modules to be reprocessed
// that were processed prior to the provided appVersion.
func (db *DB) UpdateModuleVersionStatesForReprocessing(ctx context.Context, appVersion string) (err error) {
	defer derrors.Wrap(&err, "UpdateModuleVersionStatesForReprocessing(ctx, %q)", appVersion)

	for _, status := range []int{
		http.StatusOK,
		derrors.ToStatus(derrors.HasIncompletePackages),
		derrors.ToStatus(derrors.BadModule),
		derrors.ToStatus(derrors.AlternativeModule),
		derrors.ToStatus(derrors.DBModuleInsertInvalid),
	} {
		if err := db.UpdateModuleVersionStatesWithStatus(ctx, status, appVersion); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) UpdateModuleVersionStatesWithStatus(ctx context.Context, status int, appVersion string) (err error) {
	query := `UPDATE module_version_states
			SET
				status = $2,
				next_processed_after = CURRENT_TIMESTAMP,
				last_processed_at = NULL
			WHERE
				app_version < $1
				AND status = $3;`
	result, err := db.db.Exec(ctx, query, appVersion,
		derrors.ToReprocessStatus(status), status)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("result.RowsAffected(): %v", err)
	}
	log.Infof(ctx,
		"Updated module_version_states with status=%d and app_version < %q to status=%d; %d affected",
		status, appVersion, derrors.ToReprocessStatus(status), affected)
	return nil
}

// largeModulePackageThresold represents the package threshold at which it
// becomes difficult to process packages. Modules with more than this number
// of packages are generally different versions or forks of kubernetes,
// aws-sdk-go, azure-sdk-go, and bilibili.
const largeModulePackageThreshold = 1500

// largeModulesLimit represents the number of large modules that we are
// willing to enqueue at a given time.
// var for testing.
var largeModulesLimit = 100

// GetNextModulesToFetch returns the next batch of modules that need to be
// processed. We prioritize modules based on (1) whether it has status zero
// (never processed), (2) whether it is the latest version, (3) if it is an
// alternative module, and (4) the number of packages it has. We want to leave
// time-consuming modules until the end and process them at a slower rate to
// reduce database load and timeouts. We also want to leave alternative modules
// towards the end, since these will incur unnecessary deletes otherwise.
func (db *DB) GetNextModulesToFetch(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	defer derrors.Wrap(&err, "GetNextModulesToFetch(ctx, %d)", limit)

	var mvs []*internal.ModuleVersionState
	query := fmt.Sprintf(nextModulesToProcessQuery, moduleVersionStateColumns)

	collect := func(rows *sql.Rows) error {
		// Scan the last two columns separately; they are in the query only for sorting.
		scan := func(dests ...interface{}) error {
			var latest bool
			var npkg int
			return rows.Scan(append(dests, &latest, &npkg)...)
		}
		mv, err := scanModuleVersionState(scan)
		if err != nil {
			return err
		}
		mvs = append(mvs, mv)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, largeModulePackageThreshold, limit); err != nil {
		return nil, err
	}
	if len(mvs) == 0 {
		log.Infof(ctx, "No modules to requeue")
	} else {
		fmtIntp := func(p *int) string {
			if p == nil {
				return "NULL"
			}
			return strconv.Itoa(*p)
		}
		start := mvs[0]
		end := mvs[len(mvs)-1]
		pkgRange := fmt.Sprintf("%s <= num_packages <= %s", fmtIntp(start.NumPackages), fmtIntp(end.NumPackages))
		log.Infof(ctx, "GetNextModulesToFetch: num_modules=%d; %s; start_module=%q; end_module=%q",
			len(mvs), pkgRange,
			fmt.Sprintf("%s/@v/%s", start.ModulePath, start.Version),
			fmt.Sprintf("%s/@v/%s", end.ModulePath, end.Version))
	}

	// Don't return more than largeModulesLimit of modules that have more than
	// largeModulePackageThreshold packages.
	nLarge := 0
	for i, m := range mvs {
		if m.NumPackages != nil && *m.NumPackages >= largeModulePackageThreshold {
			nLarge++
		}
		if nLarge > largeModulesLimit {
			return mvs[:i], nil
		}
	}
	return mvs, nil
}

const nextModulesToProcessQuery = `
    -- Make a table of the latest versions of each module.
	WITH latest_versions AS (
		SELECT DISTINCT ON (module_path) module_path, version
		FROM module_version_states
		ORDER BY
			module_path,
			right(sort_version, 1) = '~' DESC, -- prefer release versions
			sort_version DESC
	)
	SELECT %s, latest, npkg
	FROM (
		SELECT
			%[1]s,
			((module_path, version) IN (SELECT * FROM latest_versions)) AS latest,
			COALESCE(num_packages, 0) AS npkg
		FROM module_version_states
	) s
	WHERE next_processed_after < CURRENT_TIMESTAMP
		AND (status = 0 OR status >= 500)
	ORDER BY
		CASE
			 -- new modules
			 WHEN status = 0 THEN 0
			 WHEN npkg < $1 THEN		-- non-large modules
				CASE
					WHEN latest THEN	-- latest version
						CASE
							-- with ReprocessStatusOK or ReprocessHasIncompletePackages
							WHEN status = 520 OR status = 521 THEN 1
							-- with ReprocessBadModule or ReprocessAlternative or ReprocessDBModuleInsertInvalid
							WHEN status = 540 OR status = 541 OR status = 542 THEN 2
							ELSE 9
						END
					ELSE				-- non-latest version
						CASE
							WHEN status = 520 OR status = 521 THEN 3
							WHEN status = 540 OR status = 541 OR status = 542 THEN 4
							ELSE 9
						END
				END
			ELSE						-- large modules
				CASE
					WHEN latest THEN	-- latest version
						CASE
							WHEN status = 520 OR status = 521 THEN 5
							WHEN status = 540 OR status = 541 OR status = 542 THEN 6
							ELSE 9
						END
					ELSE				-- non-latest version
						CASE
							WHEN status = 520 OR status = 521 THEN 7
							WHEN status = 540 OR status = 541 OR status = 542 THEN 8
							ELSE 9
						END
				END
		END,
		npkg,  -- prefer fewer packages
		module_path,
		version -- for reproducibility
	LIMIT $2
`
