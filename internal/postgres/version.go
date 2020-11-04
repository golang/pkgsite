// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

// GetVersionsForPath returns a list of tagged versions sorted in
// descending semver order if any exist. If none, it returns the 10 most
// recent from a list of pseudo-versions sorted in descending semver order.
func (db *DB) GetVersionsForPath(ctx context.Context, path string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetVersionsForPath(ctx, %q)", path)

	versions, err := getPathVersions(ctx, db, path, version.TypeRelease, version.TypePrerelease)
	if err != nil {
		return nil, err
	}
	if len(versions) != 0 {
		return versions, nil
	}
	versions, err = getPathVersions(ctx, db, path, version.TypePseudo)
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// getPathVersions returns a list of versions sorted in descending semver
// order. The version types included in the list are specified by a list of
// VersionTypes.
func getPathVersions(ctx context.Context, db *DB, path string, versionTypes ...version.Type) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "getPathVersions(ctx, db, %q, %v)", path, versionTypes)

	baseQuery := `
	SELECT
		m.module_path,
		m.version,
		m.commit_time,
		m.redistributable,
		m.has_go_mod,
		m.source_info
	FROM modules m
	INNER JOIN units p
		ON p.module_id = m.id
	LEFT JOIN documentation d
		ON d.unit_id = p.id
	WHERE
		p.v1_path = (
			SELECT p2.v1_path
			FROM units as p2
			WHERE p2.path = $1
			LIMIT 1
		)
		AND version_type in (%s)
		-- Packages must have documentation source
		AND (p.name = '' OR d.source IS NOT NULL)
	ORDER BY
		m.incompatible,
		m.module_path DESC,
		m.sort_version DESC %s`

	queryEnd := `;`
	if len(versionTypes) == 0 {
		return nil, fmt.Errorf("error: must specify at least one version type")
	} else if len(versionTypes) == 1 && versionTypes[0] == version.TypePseudo {
		queryEnd = `LIMIT 10;`
	}
	query := fmt.Sprintf(baseQuery, versionTypeExpr(versionTypes), queryEnd)
	var versions []*internal.ModuleInfo
	collect := func(rows *sql.Rows) error {
		mi, err := scanModuleInfo(rows.Scan)
		if err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		versions = append(versions, mi)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, path); err != nil {
		return nil, err
	}
	return versions, nil
}

// versionTypeExpr returns a comma-separated list of version types,
// for use in a clause like "WHERE version_type IN (%s)"
func versionTypeExpr(vts []version.Type) string {
	var vs []string
	for _, vt := range vts {
		vs = append(vs, fmt.Sprintf("'%s'", vt.String()))
	}
	return strings.Join(vs, ", ")
}

// GetLatestMajorVersion returns the latest major version string of a module
// path. For example, in the module path "github.com/casbin/casbin", there
// is another path with a greater major version
// "github.com/casbin/casbin/v3". This function will return "/v3" or an
// empty string if there is no major version string at the end.
func (db *DB) GetLatestMajorVersion(ctx context.Context, seriesPath string) (_ string, err error) {
	defer derrors.Wrap(&err, "DB.GetLatestMajorVersion(ctx, %q)", seriesPath)

	var latestPath string
	latestModulePathQuery := fmt.Sprintf(`
		SELECT
			m.module_path
		FROM
			modules m
		WHERE
			m.series_path = $1
		%s
		LIMIT 1;`, orderByLatest)
	row := db.db.QueryRow(ctx, latestModulePathQuery, seriesPath)
	if err := row.Scan(&latestPath); err != nil {
		return "", err
	}
	_, majorPath, ok := module.SplitPathVersion(latestPath)
	if !ok {
		return "", fmt.Errorf("module.SplitPathVersion(%q): %v", latestPath, majorPath)
	}
	return majorPath, nil
}
