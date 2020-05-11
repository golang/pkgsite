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
		derrors.ToHTTPStatus(derrors.HasIncompletePackages),
		derrors.ToHTTPStatus(derrors.BadModule),
		derrors.ToHTTPStatus(derrors.AlternativeModule),
	} {
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
	}
	return nil
}

// Modules with more than this number of packages are generally different
// versions or forks of kubernetes, aws-sdk-go, azure-sdk-go, and bilibili.
var isBigModulePackageThreshold = 1500

// GetNextModulesToFetch returns the next batch of modules that need to be
// processed. We prioritize modules based on (1) whether it is the latest version,
// (2) if it is an alternative module, and (3) the number of packages it has.
// We want to leave time-consuming modules until the end and process them at
// a slower rate to reduce database load and timeouts. We also want to leave
// alternative modules towards the end, since these will incur unnecessary
// deletes otherwise.
func (db *DB) GetNextModulesToFetch(ctx context.Context, limit int) (_ []*internal.ModuleVersionState, err error) {
	defer derrors.Wrap(&err, "GetNextModulesToFetch(ctx, %d)", limit)

	var mvs []*internal.ModuleVersionState
	for _, next := range []struct {
		query    string
		statuses []int
	}{
		{
			query: getLatestModuleVersionStates,
			statuses: []int{
				derrors.ToHTTPStatus(derrors.ReprocessStatusOK),
				derrors.ToHTTPStatus(derrors.ReprocessHasIncompletePackages),
			},
		},
		{
			query: getLatestModuleVersionStates,
			statuses: []int{
				derrors.ToHTTPStatus(derrors.ReprocessBadModule),
				derrors.ToHTTPStatus(derrors.ReprocessAlternative),
			},
		},
		{
			query: getModuleVersionStates,
			statuses: []int{
				derrors.ToHTTPStatus(derrors.ReprocessStatusOK),
				derrors.ToHTTPStatus(derrors.ReprocessHasIncompletePackages),
			},
		},
		{
			query: getModuleVersionStates,
			statuses: []int{
				derrors.ToHTTPStatus(derrors.ReprocessBadModule),
				derrors.ToHTTPStatus(derrors.ReprocessAlternative),
			},
		},
		{
			query: getModuleVersionStatesRemainder,
		},
	} {
		query := constructRequeueQuery(next.query, next.statuses)
		collect := func(rows *sql.Rows) error {
			mv, err := scanModuleVersionState(rows.Scan)
			if err != nil {
				return err
			}
			mvs = append(mvs, mv)
			return nil
		}
		if err := db.db.RunQuery(ctx, query, collect, limit); err != nil {
			return nil, err
		}
		if len(mvs) > 0 {
			var msg string
			switch next.query {
			case getModuleVersionStatesRemainder:
				msg = fmt.Sprintf("modules with status=0 or status=500 or num_packages > %d", isBigModulePackageThreshold)
			case getLatestModuleVersionStates:
				msg = "latest version of modules"
			default:
				msg = "non-latest version of modules"
			}
			fmtIntp := func(p *int) string {
				if p == nil {
					return "NULL"
				}
				return strconv.Itoa(*p)
			}
			start := mvs[0]
			end := mvs[len(mvs)-1]
			pkgRange := fmt.Sprintf("%s <= num_packages <= %s", fmtIntp(start.NumPackages), fmtIntp(end.NumPackages))
			log.Infof(ctx, fmt.Sprintf("GetNextModulesToFetch (%s): num_modules=%d; statuses=%v; %s; start_module=%q; end_module=%q",
				msg, len(mvs), next.statuses, pkgRange,
				fmt.Sprintf("%s/@v/%s", start.ModulePath, start.Version),
				fmt.Sprintf("%s/@v/%s", end.ModulePath, end.Version)))
			return mvs, nil
		}
	}
	log.Infof(ctx, "No modules to requeue")
	return mvs, nil
}

func constructRequeueQuery(baseQuery string, statuses []int) string {
	where := "WHERE next_processed_after < CURRENT_TIMESTAMP"
	if baseQuery != getModuleVersionStatesRemainder {
		where += fmt.Sprintf(" AND COALESCE(num_packages, 0) < %d", isBigModulePackageThreshold)
		var s string
		for i, status := range statuses {
			s += fmt.Sprintf("status=%d", status)
			if i < len(statuses)-1 {
				s += " OR "
			}
		}
		where += fmt.Sprintf(" AND (%s)", s)
	}
	query := fmt.Sprintf(baseQuery, moduleVersionStateColumns, where)
	return query
}

// Get the latest versions of modules that previously
// returned a 20x status; process them in order of
// number of packages.
//
// We also want to prefer release to non-release
// versions. A sort_version will end in '~' if it is a
// release, and that is larger than any other character
// that can occur in a sort_version.
// So if we sort first by the last character in
// sort_version, then by sort_version itself, we will
// get releases before non-releases.  To implement that
// two-level ordering in a MAX, we construct an array
// of the two strings.
// Arrays are ordered lexicographically, so MAX will do
// just what we want.
const getLatestModuleVersionStates = `
SELECT %s
FROM (
    SELECT s.*
    FROM module_version_states s
    INNER JOIN (
        SELECT module_path,
        MAX(ARRAY[right(sort_version, 1), sort_version]) AS mv
        FROM module_version_states
        GROUP BY 1
    ) m
    ON
        s.module_path = m.module_path
        AND s.sort_version = m.mv[2]

    -- WHERE clause
    %s

    ORDER BY
        num_packages,
        sort_version DESC,
        module_path
    LIMIT $1
) foo`

// Get non-latest versions to be reprocessed.
// Start with modules that previously succeeded, then
// move onto alternative modules.
const getModuleVersionStates = `
SELECT %s
FROM module_version_states

-- WHERE clause
%s

ORDER BY
    num_packages,
    sort_version DESC,
    module_path
LIMIT $1`

const getModuleVersionStatesRemainder = `
SELECT %s
FROM module_version_states

-- WHERE clause
%s

ORDER BY
    CASE WHEN status=0 THEN 0
         WHEN (status=520 OR status=521) THEN 1
         WHEN (status=540 OR status=541) THEN 2
         ELSE 3 END,
    COALESCE(num_packages, 0),
    sort_version DESC,
    module_path
LIMIT $1`
