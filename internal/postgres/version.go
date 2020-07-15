// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
)

// LegacyGetTaggedVersionsForPackageSeries returns a list of tagged versions sorted in
// descending semver order. This list includes tagged versions of packages that
// have the same v1path.
func (db *DB) LegacyGetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*internal.ModuleInfo, error) {
	return getPackageVersions(ctx, db, pkgPath, []version.Type{version.TypeRelease, version.TypePrerelease})
}

// LegacyGetPsuedoVersionsForPackageSeries returns the 10 most recent from a list of
// pseudo-versions sorted in descending semver order. This list includes
// pseudo-versions of packages that have the same v1path.
func (db *DB) LegacyGetPsuedoVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*internal.ModuleInfo, error) {
	return getPackageVersions(ctx, db, pkgPath, []version.Type{version.TypePseudo})
}

// getPackageVersions returns a list of versions sorted in descending semver
// order. The version types included in the list are specified by a list of
// VersionTypes.
func getPackageVersions(ctx context.Context, db *DB, pkgPath string, versionTypes []version.Type) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "DB.getPackageVersions(ctx, db, %q, %v)", pkgPath, versionTypes)

	baseQuery := `
		SELECT
			p.module_path,
			p.version,
			m.commit_time
		FROM
			packages p
		INNER JOIN
			modules m
		ON
			p.module_path = m.module_path
			AND p.version = m.version
		WHERE
			p.v1_path = (
				SELECT v1_path
				FROM packages
				WHERE path = $1
				LIMIT 1
			)
			AND version_type in (%s)
		ORDER BY
			m.sort_version DESC %s`
	queryEnd := `;`
	if len(versionTypes) == 0 {
		return nil, fmt.Errorf("error: must specify at least one version type")
	} else if len(versionTypes) == 1 && versionTypes[0] == version.TypePseudo {
		queryEnd = `LIMIT 10;`
	}
	query := fmt.Sprintf(baseQuery, versionTypeExpr(versionTypes), queryEnd)

	rows, err := db.db.Query(ctx, query, pkgPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versionHistory []*internal.ModuleInfo
	for rows.Next() {
		var mi internal.ModuleInfo
		if err := rows.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		versionHistory = append(versionHistory, &mi)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(): %v", err)
	}

	return versionHistory, nil
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

// LegacyGetTaggedVersionsForModule returns a list of tagged versions sorted in
// descending semver order.
func (db *DB) LegacyGetTaggedVersionsForModule(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	return getModuleVersions(ctx, db, modulePath, []version.Type{version.TypeRelease, version.TypePrerelease})
}

// LegacyGetPsuedoVersionsForModule returns the 10 most recent from a list of
// pseudo-versions sorted in descending semver order.
func (db *DB) LegacyGetPsuedoVersionsForModule(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	return getModuleVersions(ctx, db, modulePath, []version.Type{version.TypePseudo})
}

// getModuleVersions returns a list of versions sorted in descending semver
// order. The version types included in the list are specified by a list of
// VersionTypes.
func getModuleVersions(ctx context.Context, db *DB, modulePath string, versionTypes []version.Type) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "getModuleVersions(ctx, db, %q, %v)", modulePath, versionTypes)

	baseQuery := `
	SELECT
		module_path, version, commit_time
    FROM
		modules
	WHERE
		series_path = $1
	    AND version_type in (%s)
	ORDER BY
		sort_version DESC %s`

	queryEnd := `;`
	if len(versionTypes) == 0 {
		return nil, fmt.Errorf("error: must specify at least one version type")
	} else if len(versionTypes) == 1 && versionTypes[0] == version.TypePseudo {
		queryEnd = `LIMIT 10;`
	}
	query := fmt.Sprintf(baseQuery, versionTypeExpr(versionTypes), queryEnd)
	var vinfos []*internal.ModuleInfo
	collect := func(rows *sql.Rows) error {
		var mi internal.ModuleInfo
		if err := rows.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime); err != nil {
			return err
		}
		vinfos = append(vinfos, &mi)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, internal.SeriesPathForModule(modulePath)); err != nil {
		return nil, err
	}
	return vinfos, nil
}

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
		m.version_type,
		m.redistributable,
		m.has_go_mod,
		m.source_info
	FROM modules m
	INNER JOIN paths p
	ON p.module_id = m.id
	WHERE
		p.v1_path = (
			SELECT p2.v1_path
			FROM paths as p2
			WHERE p2.path = $1
			LIMIT 1
		)
		AND version_type in (%s)
	ORDER BY
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
		var mi internal.ModuleInfo
		if err := rows.Scan(
			&mi.ModulePath,
			&mi.Version,
			&mi.CommitTime,
			&mi.VersionType,
			&mi.IsRedistributable,
			&mi.HasGoMod,
			jsonbScanner{&mi.SourceInfo},
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		versions = append(versions, &mi)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, path); err != nil {
		return nil, err
	}
	return versions, nil
}
