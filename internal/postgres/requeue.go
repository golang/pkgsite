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
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
)

// UpdateModuleVersionStatesForReprocessing marks modules to be reprocessed
// that were processed prior to the provided appVersion.
func (db *DB) UpdateModuleVersionStatesForReprocessing(ctx context.Context, appVersion string) (err error) {
	defer derrors.WrapStack(&err, "UpdateModuleVersionStatesForReprocessing(ctx, %q)", appVersion)

	for _, status := range []int{
		http.StatusOK,
		derrors.ToStatus(derrors.HasIncompletePackages),
		derrors.ToStatus(derrors.DBModuleInsertInvalid),
	} {
		if err := db.UpdateModuleVersionStatesWithStatus(ctx, status, appVersion); err != nil {
			return err
		}
	}
	return nil
}

// UpdateModuleVersionStatesForReprocessingReleaseVersionsOnly marks modules to be
// reprocessed that were processed prior to the provided appVersion.
func (db *DB) UpdateModuleVersionStatesForReprocessingReleaseVersionsOnly(ctx context.Context, appVersion string) (err error) {
	query := `
		UPDATE module_version_states mvs
		SET
			status = (
				CASE WHEN status=200 THEN 520
					 WHEN status=290 THEN 521
					 END
				),
			next_processed_after = CURRENT_TIMESTAMP,
			last_processed_at = NULL
		WHERE
			app_version < $1
			AND (status = 200 OR status = 290)
			AND right(sort_version, 1) = '~' -- release versions only
			AND NOT incompatible;`
	affected, err := db.db.Exec(ctx, query, appVersion)
	if err != nil {
		return err
	}
	log.Infof(ctx, "Updated release and non-incompatible versions of module_version_states with status=200 and status=290 and app_version < %q; %d affected", appVersion, affected)
	return nil
}

// UpdateModuleVersionStatesForReprocessingLatestOnly marks modules to be
// reprocessed that were processed prior to the provided appVersion.
func (db *DB) UpdateModuleVersionStatesForReprocessingLatestOnly(ctx context.Context, appVersion string) (err error) {
	query := `
		UPDATE module_version_states mvs
		SET
			status = (
				CASE WHEN status=200 THEN 520
					 WHEN status=290 THEN 521
					 END
				),
			next_processed_after = CURRENT_TIMESTAMP,
			last_processed_at = NULL
		FROM (
			SELECT DISTINCT ON (module_path) module_path, version
			FROM module_version_states
			ORDER BY
				module_path,
				incompatible,
				right(sort_version, 1) = '~' DESC, -- prefer release versions
				sort_version DESC
		) latest
		WHERE
			app_version < $1
			AND (status = 200 OR status = 290)
			AND latest.module_path = mvs.module_path
			AND latest.version = mvs.version;`
	affected, err := db.db.Exec(ctx, query, appVersion)
	if err != nil {
		return err
	}
	log.Infof(ctx, "Updated latest version of module_version_states with status=200 and status=290 and app_version < %q; %d affected", appVersion, affected)
	return nil
}

// UpdateModuleVersionStatesForReprocessingSearchDocumentsOnly marks modules to be
// reprocessed that are in the search_documents table.
func (db *DB) UpdateModuleVersionStatesForReprocessingSearchDocumentsOnly(ctx context.Context, appVersion string) (err error) {
	query := `
		UPDATE module_version_states mvs
		SET
			status = (
				CASE WHEN status=200 THEN 520
					 WHEN status=290 THEN 521
					 END
				),
			next_processed_after = CURRENT_TIMESTAMP
		FROM (
			SELECT
				module_path,
				version
			FROM search_documents
			GROUP BY 1, 2
			ORDER BY 1, 2
		) sd
		WHERE
			app_version < $1
			AND (mvs.status = 200 OR mvs.status = 290)
			AND mvs.module_path = sd.module_path
			AND mvs.version = sd.version;`
	affected, err := db.db.Exec(ctx, query, appVersion)
	if err != nil {
		return err
	}
	log.Infof(ctx, "Updated module versions in search_documents to be reprocessed", appVersion, affected)
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
	affected, err := db.db.Exec(ctx, query, appVersion, derrors.ToReprocessStatus(status), status)
	if err != nil {
		return err
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
var largeModulesLimit = config.GetEnvInt(context.Background(), "GO_DISCOVERY_LARGE_MODULES_LIMIT", 100)

// GetNextModulesToFetch returns the next batch of modules that need to be
// processed. We prioritize modules based on (1) whether it has status zero
// (never processed), (2) whether it is the latest version, (3) if it is an
// alternative module, and (4) the number of packages it has. We want to leave
// time-consuming modules until the end and process them at a slower rate to
// reduce database load and timeouts. We also want to leave alternative modules
// towards the end, since these will incur unnecessary deletes otherwise.
func (db *DB) GetNextModulesToFetch(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	defer derrors.WrapStack(&err, "GetNextModulesToFetch(ctx, %d)", limit)
	queryFmt := nextModulesToProcessQuery

	var mvs []*internal.ModuleVersionState
	query := fmt.Sprintf(queryFmt, moduleVersionStateColumns)

	collect := func(rows *sql.Rows) error {
		// Scan the last two columns separately; they are in the query only for sorting.
		scan := func(dests ...interface{}) error {
			var npkg int
			return rows.Scan(append(dests, &npkg)...)
		}
		mv, err := scanModuleVersionState(scan)
		if err != nil {
			return err
		}
		mvs = append(mvs, mv)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, limit); err != nil {
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
	// largeModulePackageThreshold packages, or of modules with status zero.
	nLargeOrZero := 0
	for i, m := range mvs {
		if m.Status == 0 || (m.NumPackages != nil && *m.NumPackages >= largeModulePackageThreshold) {
			nLargeOrZero++
		}
		if nLargeOrZero > largeModulesLimit {
			return mvs[:i], nil
		}
	}
	return mvs, nil
}

// This query prioritizes latest versions, but other than that, it tries
// to avoid grouping modules in any way except by latest and status code:
// processing is much smoother when they are enqueued in random order.
//
// To make the result deterministic for testing, we hash the module path and version
// rather than actually choosing a random number. md5 is built in to postgres and
// is an adequate hash for this purpose.
const nextModulesToProcessQuery = `
	SELECT %s, npkg
	FROM (
		SELECT
			%[1]s,
			COALESCE(num_packages, 0) AS npkg
		FROM module_version_states
	) s
	WHERE next_processed_after < CURRENT_TIMESTAMP
		AND (status = 0 OR status >= 500)
	ORDER BY
		CASE
			-- new modules
			WHEN status = 0 THEN 0
			WHEN status = 503 or status = 520 OR status = 521 THEN 3
			WHEN status = 540 OR status = 541 OR status = 542 THEN 4
			ELSE 5
		END,
		md5(module_path||version) -- deterministic but effectively random
	LIMIT $1
`
