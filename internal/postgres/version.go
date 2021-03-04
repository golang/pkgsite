// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/version"
	"golang.org/x/sync/errgroup"
)

// GetVersionsForPath returns a list of tagged versions sorted in
// descending semver order if any exist. If none, it returns the 10 most
// recent from a list of pseudo-versions sorted in descending semver order.
func (db *DB) GetVersionsForPath(ctx context.Context, path string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.WrapStack(&err, "GetVersionsForPath(ctx, %q)", path)

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
		m.deprecated_comment,
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

	if experiment.IsActive(ctx, internal.ExperimentRetractions) {
		// Get information about retractions an deprecations, and apply it.
		start := time.Now()
		lmv, err := db.GetLatestModuleVersions(ctx, mi.ModulePath)
		if err != nil {
			return err
		}
		if lmv != nil {
			lmv.PopulateModuleInfo(mi)
		}
		log.Debugf(ctx, "latest info fetched and applied in %dms", time.Since(start).Milliseconds())
	}
	return nil
}

func populateLatestInfos(ctx context.Context, db *DB, mis []*internal.ModuleInfo) (err error) {
	defer derrors.WrapStack(&err, "populateLatestInfos(%d ModuleInfos)", len(mis))

	if experiment.IsActive(ctx, internal.ExperimentRetractions) {
		start := time.Now()
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
		log.Debugf(ctx, "latest info fetched and applied in %dms", time.Since(start).Milliseconds())
	}
	return nil
}

// GetLatestInfo returns the latest information about the unit in the module.
// See internal.LatestInfo for documentation about the returned values.
func (db *DB) GetLatestInfo(ctx context.Context, unitPath, modulePath string) (latest internal.LatestInfo, err error) {
	defer derrors.WrapStack(&err, "DB.GetLatestInfo(ctx, %q, %q)", unitPath, modulePath)

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
	defer derrors.WrapStack(&err, "DB.getLatestMajorVersion(ctx, %q, %q)", fullPath, modulePath)

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
	row = db.db.QueryRow(ctx, `
		SELECT p.path
		FROM units u
		INNER JOIN paths p ON p.id = u.path_id
		INNER JOIN paths p2 ON p2.id = u.v1path_id
		WHERE p2.path = $1 AND module_id = $2;`, v1Path, modID)
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
	defer derrors.WrapStack(&err, "DB.getLatestMinorVersion(ctx, %q, %q)", unitPath, modulePath)

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
	err = db.db.QueryRow(ctx, `
		SELECT 1
		FROM units u
		INNER JOIN paths p ON p.id = u.path_id
		WHERE p.path = $1 AND u.module_id = $2`, unitPath, modID).Scan(&x)
	switch err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, err
	}
}

func shouldUpdateRawLatest(newVersion, curVersion string) bool {
	// Only update if the new one is later according to version.Later
	// (semver except that release > prerelease). that avoids a race
	// condition where worker 1 sees a version, but worker 2 sees a
	// newer version and updates before worker 1 proceeds.
	//
	// However, the raw latest version can go backwards if it was an
	// incompatible version, but then a compatible version with a go.mod
	// file is published. For example, the module starts with a
	// v2.0.0+incompatible, but then the author adds a v1.0.0 with a
	// go.mod file, making v1.0.0 the new latest. Take that case into
	// account.
	return version.Later(newVersion, curVersion) ||
		(version.IsIncompatible(curVersion) && !version.IsIncompatible(newVersion))
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
	)
	err = db.QueryRow(ctx, `
		SELECT r.module_path_id, r.raw_version, r.cooked_version, r.good_version, r.raw_go_mod_bytes
		FROM latest_module_versions r
		INNER JOIN paths p ON p.id = r.module_path_id
		WHERE p.path = $1`,
		modulePath).Scan(&id, &raw, &cooked, &good, &goModBytes)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	lmv, err := internal.NewLatestModuleVersions(modulePath, raw, cooked, good, goModBytes)
	if err != nil {
		return nil, 0, err
	}
	return lmv, id, nil
}

// UpdateLatestModuleVersions upserts its argument into the latest_module_versions table
// if the row doesn't exist, or the new version is later.
// It also updates lmv.GoodVersion to the latest good version, always.
func (db *DB) UpdateLatestModuleVersions(ctx context.Context, vNew *internal.LatestModuleVersions) (err error) {
	defer derrors.WrapStack(&err, "UpdateLatestModuleVersions(%q)", vNew.ModulePath)

	// We need RepeatableRead here because the INSERT...ON CONFLICT does a read.
	return db.db.Transact(ctx, sql.LevelRepeatableRead, func(tx *database.DB) error {
		vCur, id, err := getLatestModuleVersions(ctx, tx, vNew.ModulePath)
		if err != nil {
			return err
		}
		// Is vNew the most recent information, or does the DB already have
		//something more up to date?
		update := vCur == nil || shouldUpdateRawLatest(vNew.RawVersion, vCur.RawVersion)
		// Set lmv to the information that is now in force.
		lmv := vCur
		if update {
			lmv = vNew
		}

		// Even if we don't update the raw and cooked versions, we might need to
		// update the good version. For instance, we could have just added a
		// version that is later than all the other good ones but still not the
		// same as the raw or cooked version. So find the latest good version,
		// using the most up-to-date information.
		latestGoodVersion, err := getLatestGoodVersion(ctx, tx, lmv)
		if err != nil {
			return err
		}
		// Update the good version if it differs from the current value.
		update = update || latestGoodVersion != vCur.GoodVersion
		if !update {
			log.Debugf(ctx, "%s: not updating latest module versions", vNew.ModulePath)
			return nil
		}
		lmv.GoodVersion = latestGoodVersion
		if vCur == nil {
			log.Debugf(ctx, "%s: inserting raw=%q, cooked=%q, good=%q",
				lmv.ModulePath, lmv.RawVersion, lmv.CookedVersion, lmv.GoodVersion)
		} else {
			log.Debugf(ctx, "%s: updating raw=%q, cooked=%q, good=%q to raw=%q, cooked=%q, good=%q",
				lmv.ModulePath, vCur.RawVersion, vCur.CookedVersion, vCur.GoodVersion,
				lmv.RawVersion, lmv.CookedVersion, lmv.GoodVersion)
		}
		return upsertLatestModuleVersions(ctx, tx, id, lmv)
	})
}

// getLatestGoodVersion returns the latest version of a module in the modules table,
// respecting the retractions and other information in the given LatestModuleVersions.
func getLatestGoodVersion(ctx context.Context, tx *database.DB, lmv *internal.LatestModuleVersions) (_ string, err error) {
	defer derrors.WrapStack(&err, "getLatestGoodVersion(%q)", lmv.ModulePath)

	// Read the versions from the modules table.
	// If the cooked latest version is incompatible, then include
	// incompatible versions. If it isn't, then either there are no
	// incompatible versions, or there are but the latest compatible version
	// has a go.mod file. Either way, ignore incompatible versions.
	q := squirrel.Select("version").
		From("modules").
		Where(squirrel.Eq{"module_path": lmv.ModulePath}).
		PlaceholderFormat(squirrel.Dollar)
	if !version.IsIncompatible(lmv.CookedVersion) {
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
	return version.LatestOf(version.RemoveIf(vs, lmv.IsRetracted)), nil
}

func upsertLatestModuleVersions(ctx context.Context, tx *database.DB, id int, lmv *internal.LatestModuleVersions) (err error) {
	defer derrors.WrapStack(&err, "upsertLatestModuleVersions(%q)", lmv.ModulePath)

	// If the row doesn't exist, get a path ID for the module path.
	if id == 0 {
		id, err = upsertPath(ctx, tx, lmv.ModulePath)
		if err != nil {
			return err
		}
	}
	// Convert the go.mod file into bytes.
	goModBytes, err := lmv.GoModFile.Format()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO latest_module_versions (
			module_path_id,
			raw_version,
			cooked_version,
			good_version,
			raw_go_mod_bytes
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (module_path_id)
		DO UPDATE SET
			raw_version=excluded.raw_version,
			cooked_version=excluded.cooked_version,
			good_version=excluded.good_version,
			raw_go_mod_bytes=excluded.raw_go_mod_bytes
		`,
		id, lmv.RawVersion, lmv.CookedVersion, lmv.GoodVersion, goModBytes)
	return err
}
