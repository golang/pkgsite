// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/sync/errgroup"
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
	INNER JOIN units u
		ON u.module_id = m.id
	LEFT JOIN documentation d
		ON d.unit_id = u.id
	WHERE
		u.v1_path = (
			SELECT u2.v1_path
			FROM units as u2
			WHERE u2.path = $1
			LIMIT 1
		)
		AND version_type in (%s)
		-- Packages must have documentation source
		AND (u.name = '' OR d.source IS NOT NULL)
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

// GetLatestInfo returns the latest information about the unit in the module.
// See internal.LatestInfo for documentation about the returned values.
func (db *DB) GetLatestInfo(ctx context.Context, unitPath, modulePath string) (latest internal.LatestInfo, err error) {
	defer derrors.Wrap(&err, "DB.GetLatestInfo(ctx, %q, %q)", unitPath, modulePath)

	group, gctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		um, err := db.GetUnitMeta(gctx, unitPath, internal.UnknownModulePath, internal.LatestVersion)
		if err != nil {
			return err
		}
		latest.MinorVersion = um.Version
		latest.MinorModulePath = um.ModulePath
		return nil
	})
	group.Go(func() (err error) {
		latest.MajorModulePath, latest.MajorUnitPath, err = db.getLatestMajorVersion(gctx, unitPath, modulePath)
		return err
	})
	group.Go(func() (err error) {
		latest.UnitExistsAtMinor, err = db.getLatestMinorModuleVersionInfo(gctx, unitPath, modulePath)
		return err
	})

	if err := group.Wait(); err != nil {
		return internal.LatestInfo{}, err
	}
	return latest, nil
}

// getLatestMajorVersion returns the latest module path and the full package path
// of the latest version found, given the fullPath and the modulePath.
// For example, in the module path "github.com/casbin/casbin", there
// is another module path with a greater major version "github.com/casbin/casbin/v3".
// This function will return "github.com/casbin/casbin/v3" or the input module path
// if no later module path was found. It also returns the full package path at the
// latest module version if it exists. If not, it returns the module path.
func (db *DB) getLatestMajorVersion(ctx context.Context, fullPath, modulePath string) (_ string, _ string, err error) {
	defer derrors.Wrap(&err, "DB.getLatestMajorVersion(ctx, %q, %q)", fullPath, modulePath)

	var (
		modID   int
		modPath string
	)
	seriesPath := internal.SeriesPathForModule(modulePath)
	q, args, err := orderByLatest(squirrel.Select("m.module_path", "m.id").
		From("modules m").
		Where(squirrel.Eq{"m.series_path": seriesPath})).
		Limit(1).
		ToSql()
	if err != nil {
		return "", "", err
	}
	row := db.db.QueryRow(ctx, q, args...)
	if err := row.Scan(&modPath, &modID); err != nil {
		return "", "", err
	}

	v1Path := internal.V1Path(fullPath, modulePath)
	row = db.db.QueryRow(ctx, `SELECT path FROM units WHERE v1_path = $1 AND module_id = $2;`, v1Path, modID)
	var path string
	switch err := row.Scan(&path); err {
	case nil:
		return modPath, path, nil
	case sql.ErrNoRows:
		return modPath, modPath, nil
	default:
		return "", "", err
	}
}

// getLatestMinorModuleVersion reports whether unitPath exists at the latest version of modulePath.
func (db *DB) getLatestMinorModuleVersionInfo(ctx context.Context, unitPath, modulePath string) (unitExists bool, err error) {
	defer derrors.Wrap(&err, "DB.getLatestMinorVersion(ctx, %q, %q)", unitPath, modulePath)

	// Find the latest version of the module path.
	var modID int
	q, args, err := orderByLatest(squirrel.Select("m.id").
		From("modules m").
		Where(squirrel.Eq{"m.module_path": modulePath})).
		Limit(1).
		ToSql()
	if err != nil {
		return false, err
	}
	row := db.db.QueryRow(ctx, q, args...)
	if err := row.Scan(&modID); err != nil {
		return false, err
	}

	// See if the unit path exists at that version.
	var x int
	err = db.db.QueryRow(ctx, `SELECT 1 FROM units WHERE path = $1 AND module_id = $2`, unitPath, modID).Scan(&x)
	switch err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}
