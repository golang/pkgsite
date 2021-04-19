// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"golang.org/x/mod/semver"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/sync/errgroup"
)

// GetVersionsForPath returns a list of tagged versions sorted in
// descending semver order if any exist. If none, it returns the 10 most
// recent from a list of pseudo-versions sorted in descending semver order.
func (db *DB) GetVersionsForPath(ctx context.Context, path string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.WrapStack(&err, "GetVersionsForPath(ctx, %q)", path)
	defer middleware.ElapsedStat(ctx, "GetVersionsForPath")

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
	defer derrors.WrapStack(&err, "getPathVersions(ctx, db, %q, %v)", path, versionTypes)

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
	WHERE
		u.v1path_id = (
			SELECT u2.v1path_id
			FROM units as u2
			INNER JOIN paths p
			ON p.id = u2.path_id
			WHERE p.path = $1
			LIMIT 1
		)
		AND version_type in (%s)
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
	if err := populateLatestInfos(ctx, db, versions); err != nil {
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

func populateLatestInfo(ctx context.Context, db *DB, mi *internal.ModuleInfo) (err error) {
	defer derrors.WrapStack(&err, "populateLatestInfo(%q)", mi.ModulePath)

	// Get information about retractions an deprecations, and apply it.
	lmv, err := db.GetLatestModuleVersions(ctx, mi.ModulePath)
	if err != nil {
		return err
	}
	if lmv != nil {
		lmv.PopulateModuleInfo(mi)
	}
	return nil
}

func populateLatestInfos(ctx context.Context, db *DB, mis []*internal.ModuleInfo) (err error) {
	defer derrors.WrapStack(&err, "populateLatestInfos(%d ModuleInfos)", len(mis))

	// Collect the LatestModuleVersions for all modules in the list.
	lmvs := map[string]*internal.LatestModuleVersions{}
	for _, mi := range mis {
		if _, ok := lmvs[mi.ModulePath]; !ok {
			lmv, err := db.GetLatestModuleVersions(ctx, mi.ModulePath)
			if err != nil {
				return err
			}
			lmvs[mi.ModulePath] = lmv
		}
	}
	// Use the collected LatestModuleVersions to populate the ModuleInfos.
	for _, mi := range mis {
		lmv := lmvs[mi.ModulePath]
		if lmv != nil {
			lmv.PopulateModuleInfo(mi)
		}
	}
	return nil
}

// GetLatestInfo returns the latest information about the unit in the module.
// See internal.LatestInfo for documentation about the returned values.
// If latestUnitMeta is non-nil, it is the result of GetUnitMeta(unitPath, internal.UnknownModulePath, internal.LatestVersion).
// That can save a redundant call to GetUnitMeta here.
func (db *DB) GetLatestInfo(ctx context.Context, unitPath, modulePath string, latestUnitMeta *internal.UnitMeta) (latest internal.LatestInfo, err error) {
	defer derrors.WrapStack(&err, "DB.GetLatestInfo(ctx, %q, %q)", unitPath, modulePath)
	defer middleware.ElapsedStat(ctx, "DB.GetLatestInfo")()

	group, gctx := errgroup.WithContext(ctx)

	if latestUnitMeta != nil {
		latest.MinorVersion = latestUnitMeta.Version
		latest.MinorModulePath = latestUnitMeta.ModulePath
	} else {
		group.Go(func() error {
			um, err := db.GetUnitMeta(gctx, unitPath, internal.UnknownModulePath, internal.LatestVersion)
			if err != nil {
				return err
			}
			latest.MinorVersion = um.Version
			latest.MinorModulePath = um.ModulePath
			return nil
		})
	}
	group.Go(func() (err error) {
		latest.MajorModulePath, latest.MajorUnitPath, err = db.getLatestMajorVersion(gctx, unitPath, modulePath)
		return err
	})
	group.Go(func() (err error) {
		latest.UnitExistsAtMinor, err = db.unitExistsAtLatest(gctx, unitPath, modulePath)
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
//
// getLatestMajorVersion only considers tagged (non-pseudo) versions. If there are none,
// it returns empty strings.
func (db *DB) getLatestMajorVersion(ctx context.Context, fullPath, modulePath string) (modPath, pkgPath string, err error) {
	defer derrors.WrapStack(&err, "DB.getLatestMajorVersion2(%q)", modulePath)
	defer middleware.ElapsedStat(ctx, "DB.getLatestMajorVersion")()

	// Collect all the non-deprecated module paths for the series that have at
	// least one good version, along with that good version. A good version
	// is both servable (in the modules table) and not retracted.
	seriesPath := internal.SeriesPathForModule(modulePath)
	q, args, err := squirrel.Select("p.path", "l.good_version").
		From("latest_module_versions l").
		Join("paths p ON p.id = l.module_path_id").
		Where(squirrel.Eq{"l.series_path": seriesPath}).
		Where("NOT l.deprecated").
		Where(squirrel.NotEq{"l.good_version": ""}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return "", "", err
	}

	type pathver struct {
		path, version string
	}

	var pathvers []pathver
	err = db.db.RunQuery(ctx, q, func(rows *sql.Rows) error {
		var pv pathver
		if err := rows.Scan(&pv.path, &pv.version); err != nil {
			return err
		}
		pathvers = append(pathvers, pv)
		return nil
	}, args...)
	if err != nil {
		return "", "", err
	}

	// Find the highest tagged version from among the (module path, good
	// version) pairs.
	var max pathver
	for _, pv := range pathvers {
		if version.IsPseudo(pv.version) {
			continue
		}
		// Use semver.Compare, not version.Later, because we don't want to prefer
		// release to prerelease: we want v2.0.0-pre over v1.0.0.
		if max.path == "" || semver.Compare(pv.version, max.version) > 0 {
			max = pv
		}
	}

	// No highest tagged version: return empty strings.
	if max.path == "" {
		return "", "", nil
	}

	// Find the unit path at the max-version module path.
	v1Path := internal.V1Path(fullPath, modulePath)
	row := db.db.QueryRow(ctx, `
		SELECT p.path
		FROM units u
		INNER JOIN modules m ON m.id = u.module_id
		INNER JOIN paths p ON p.id = u.path_id
		INNER JOIN paths p2 ON p2.id = u.v1path_id
		WHERE p2.path = $1 AND m.module_path = $2 AND m.version = $3`,
		v1Path, max.path, max.version)
	var path string
	switch err := row.Scan(&path); err {
	case nil:
		return max.path, path, nil
	case sql.ErrNoRows:
		return max.path, max.path, nil
	default:
		return "", "", err
	}
}

// unitExistsAtLatest reports whether unitPath exists at the latest version of modulePath.
func (db *DB) unitExistsAtLatest(ctx context.Context, unitPath, modulePath string) (unitExists bool, err error) {
	defer derrors.WrapStack(&err, "DB.unitExistsAtLatest(ctx, %q, %q)", unitPath, modulePath)
	defer middleware.ElapsedStat(ctx, "DB.unitExistsAtLatest")()

	// Find the latest version of the module path in the modules table.
	var latestGoodVersion string
	lmv, err := db.GetLatestModuleVersions(ctx, modulePath)
	if err != nil {
		return false, err
	}
	if lmv != nil && lmv.GoodVersion != "" {
		// If we have latest-version info, use it.
		latestGoodVersion = lmv.GoodVersion
	} else {
		// Otherwise, query the modules table, ignoring all adjustments for incompatible and retracted versions.
		err := db.db.QueryRow(ctx, `
			SELECT version
			FROM modules
			WHERE module_path = $1
			ORDER BY
				version_type = 'release' DESC,
				sort_version DESC
			LIMIT 1
		`, modulePath).Scan(&latestGoodVersion)
		if err != nil {
			return false, err
		}
	}
	if latestGoodVersion == "" {
		return true, nil
	}
	// See if the unit path exists at that version.
	var x int
	err = db.db.QueryRow(ctx, `
		SELECT 1
		FROM units u
		INNER JOIN paths p ON p.id = u.path_id
		INNER JOIN modules m ON m.id = u.module_id
		WHERE p.path = $1 AND m.module_path = $2 AND m.version = $3
	`, unitPath, modulePath, latestGoodVersion).Scan(&x)
	switch err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}

func (db *DB) getMultiLatestModuleVersions(ctx context.Context, modulePaths []string) (lmvs []*internal.LatestModuleVersions, err error) {
	defer derrors.WrapStack(&err, "getMultiLatestModuleVersions(%v)", modulePaths)
	defer middleware.ElapsedStat(ctx, "getMultiLatestModuleVersions")()

	collect := func(rows *sql.Rows) error {
		var (
			modulePath, raw, cooked, good string
			goModBytes                    []byte
		)
		if err := rows.Scan(&modulePath, &raw, &cooked, &good, &goModBytes); err != nil {
			return err
		}
		lmv, err := internal.NewLatestModuleVersions(modulePath, raw, cooked, good, goModBytes)
		if err != nil {
			return err
		}
		lmvs = append(lmvs, lmv)
		return nil
	}

	err = db.db.RunQuery(ctx, `
		SELECT p.path, r.raw_version, r.cooked_version, r.good_version, r.raw_go_mod_bytes
		FROM latest_module_versions r
		INNER JOIN paths p ON p.id = r.module_path_id
		WHERE p.path = ANY($1)
		AND r.status = 200
		ORDER BY p.path DESC
	`, collect, pq.Array(modulePaths))
	if err != nil {
		return nil, err
	}
	return lmvs, nil
}

// getLatestGoodVersion returns the latest version of a module in the modules
// table, respecting the retractions and other information in the given
// LatestModuleVersions. If lmv is nil, it finds the latest version, favoring
// release over pre-release, including incompatible versions, and ignoring
// retractions.
func getLatestGoodVersion(ctx context.Context, tx *database.DB, modulePath string, lmv *internal.LatestModuleVersions) (_ string, err error) {
	defer derrors.WrapStack(&err, "getLatestGoodVersion(%q)", modulePath)

	// Read the versions from the modules table.
	// If the cooked latest version is incompatible, then include
	// incompatible versions. If it isn't, then either there are no
	// incompatible versions, or there are but the latest compatible version
	// has a go.mod file. Either way, ignore incompatible versions.
	q := squirrel.Select("version").
		From("modules").
		Where(squirrel.Eq{"module_path": modulePath}).
		PlaceholderFormat(squirrel.Dollar)
	if lmv != nil && !version.IsIncompatible(lmv.CookedVersion) {
		q = q.Where("NOT incompatible")
	}
	query, args, err := q.ToSql()
	if err != nil {
		return "", err
	}
	vs, err := collectStrings(ctx, tx, query, args...)
	if err != nil {
		return "", err
	}
	// Choose the latest good version from the unretracted versions.
	if lmv != nil {
		vs = version.RemoveIf(vs, lmv.IsRetracted)
	}
	return version.LatestOf(vs), nil
}

// GetLatestModuleVersions returns the row of the latest_module_versions table for modulePath.
// If the module path is not found, it returns nil, nil.
func (db *DB) GetLatestModuleVersions(ctx context.Context, modulePath string) (_ *internal.LatestModuleVersions, err error) {
	lmv, _, err := getLatestModuleVersions(ctx, db.db, modulePath)
	return lmv, err
}

func getLatestModuleVersions(ctx context.Context, db *database.DB, modulePath string) (_ *internal.LatestModuleVersions, id int, err error) {
	derrors.WrapStack(&err, "getLatestModuleVersions(%q)", modulePath)

	var (
		raw, cooked, good string
		goModBytes        []byte
		status            int
	)
	err = db.QueryRow(ctx, `
		SELECT
			r.module_path_id, r.raw_version, r.cooked_version, r.good_version, r.raw_go_mod_bytes, r.status
		FROM latest_module_versions r
		INNER JOIN paths p ON p.id = r.module_path_id
		WHERE p.path = $1`,
		modulePath).Scan(&id, &raw, &cooked, &good, &goModBytes, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	if status != 200 {
		// No information for this module path, but the ID is still useful.
		return nil, id, nil
	}
	lmv, err := internal.NewLatestModuleVersions(modulePath, raw, cooked, good, goModBytes)
	if err != nil {
		return nil, 0, err
	}
	return lmv, id, nil
}

// rawIsMoreRecent reports whether raw version v1 is more recent than v2.
// v1 is more recent if it is later according to the go command (higher semver,
// preferring release to prerelease). However, the raw latest version can go
// backwards if it was an incompatible version, but then a compatible version
// with a go.mod file is published. For example, the module starts with a
// v2.0.0+incompatible, but then the author adds a v1.0.0 with a go.mod file,
// making v1.0.0 the new latest.
func rawIsMoreRecent(v1, v2 string) bool {
	return version.Later(v1, v2) || (version.IsIncompatible(v2) && !version.IsIncompatible(v1))
}

// UpdateLatestModuleVersions upserts its argument into the latest_module_versions table
// if the row doesn't exist, or the new version is later.
// It returns the version that is in the DB when it completes.
func (db *DB) UpdateLatestModuleVersions(ctx context.Context, vNew *internal.LatestModuleVersions) (_ *internal.LatestModuleVersions, err error) {
	defer derrors.WrapStack(&err, "UpdateLatestModuleVersions(%q)", vNew.ModulePath)

	var vResult *internal.LatestModuleVersions
	// We need RepeatableRead here because the INSERT...ON CONFLICT does a read.
	err = db.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		vCur, id, err := getLatestModuleVersions(ctx, tx, vNew.ModulePath)
		if err != nil {
			return err
		}
		// Is vNew the most recent information, or does the DB already have
		//something more up to date?
		update := vCur == nil || rawIsMoreRecent(vNew.RawVersion, vCur.RawVersion) ||
			// If new versions are added, cooked can change even if raw is the same.
			(vNew.RawVersion == vCur.RawVersion && vNew.CookedVersion != vCur.CookedVersion)
		if !update {
			log.Debugf(ctx, "%s: not updating latest module versions", vNew.ModulePath)
			vResult = vCur
			return nil
		}
		if vCur == nil {
			log.Debugf(ctx, "%s: inserting latest_module_versions raw=%q, cooked=%q",
				vNew.ModulePath, vNew.RawVersion, vNew.CookedVersion)
		} else {
			log.Debugf(ctx, "%s: updating latest_module_versions raw=%q, cooked=%q to raw=%q, cooked=%q",
				vNew.ModulePath, vCur.RawVersion, vCur.CookedVersion,
				vNew.RawVersion, vNew.CookedVersion)
			// If the latest good version is now retracted, recompute it.
			if vCur.GoodVersion != "" && vNew.IsRetracted(vCur.GoodVersion) {
				good, err := getLatestGoodVersion(ctx, tx, vNew.ModulePath, vNew)
				if err != nil {
					return err
				}
				vNew.GoodVersion = good
				log.Debugf(ctx, "%s: updating latest_module_versions good=%q", vNew.ModulePath, vNew.GoodVersion)
			} else {
				vNew.GoodVersion = vCur.GoodVersion
			}
		}
		vResult = vNew
		return upsertLatestModuleVersions(ctx, tx, vNew.ModulePath, id, vNew, 200)
	})
	if err != nil {
		return nil, err
	}
	return vResult, nil
}

// UpdateLatestModuleVersionsStatus updates or inserts a failure status into the
// latest_module_versions table.
// It only updates the table if it doesn't have valid information for the module path.
func (db *DB) UpdateLatestModuleVersionsStatus(ctx context.Context, modulePath string, newStatus int) (err error) {
	defer derrors.WrapStack(&err, "UpdateLatestModuleVersionsStatus(%q, %d)", modulePath, newStatus)

	// We need RepeatableRead here because the INSERT...ON CONFLICT does a read.
	return db.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		var id, curStatus int
		err := tx.QueryRow(ctx, `
				SELECT r.module_path_id, r.status
				FROM latest_module_versions r
				INNER JOIN paths p ON p.id = r.module_path_id
				WHERE p.path = $1`,
			modulePath).Scan(&id, &curStatus)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if curStatus == 200 {
			return nil
		}
		log.Debugf(ctx, "%s: updating latest_module_versions status to %d", modulePath, newStatus)
		return upsertLatestModuleVersions(ctx, tx, modulePath, id, nil, newStatus)
	})
}

func upsertLatestModuleVersions(ctx context.Context, tx *database.DB, modulePath string, id int, lmv *internal.LatestModuleVersions, status int) (err error) {
	defer derrors.WrapStack(&err, "upsertLatestModuleVersions(%s, %d)", modulePath, status)

	// If the row doesn't exist, get a path ID for the module path.
	if id == 0 {
		id, err = upsertPath(ctx, tx, modulePath)
		if err != nil {
			return err
		}
	}
	var (
		raw, cooked, good string
		goModBytes        = []byte{} // not nil, a zero-length slice
		deprecated        bool
	)
	if lmv != nil {
		raw = lmv.RawVersion
		cooked = lmv.CookedVersion
		good = lmv.GoodVersion
		deprecated = lmv.Deprecated
		// Convert the go.mod file into bytes.
		goModBytes, err = lmv.GoModFile.Format()
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO latest_module_versions (
			module_path_id,
			series_path,
			raw_version,
			cooked_version,
			good_version,
			deprecated,
			raw_go_mod_bytes,
			status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (module_path_id)
		DO UPDATE SET
			series_path=excluded.series_path,
			raw_version=excluded.raw_version,
			cooked_version=excluded.cooked_version,
			good_version=excluded.good_version,
			deprecated=excluded.deprecated,
			raw_go_mod_bytes=excluded.raw_go_mod_bytes,
			status=excluded.status
		`,
		id, internal.SeriesPathForModule(modulePath), raw, cooked, good, deprecated, goModBytes, status)
	return err
}

// updateLatestGoodVersion updates latest_module_versions.good_version for modulePath to version.
func updateLatestGoodVersion(ctx context.Context, tx *database.DB, modulePath, version string) (err error) {
	defer derrors.WrapStack(&err, "updateLatestGoodVersion(%q, %q)", modulePath, version)

	n, err := tx.Exec(ctx, `
		UPDATE latest_module_versions
		SET good_version = $2
		WHERE module_path_id = (
			SELECT id FROM paths
			WHERE path = $1
		)`, modulePath, version)
	if err != nil {
		return err
	}
	switch n {
	case 0:
		log.Debugf(ctx, "updateLatestGoodVersion(%q, %q): no change", modulePath, version)
	case 1:
		log.Debugf(ctx, "updateLatestGoodVersion(%q, %q): updated", modulePath, version)
	default:
		return errors.New("more than one row affected")
	}
	return nil
}
