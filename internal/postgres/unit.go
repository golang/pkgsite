// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/version"
)

// GetUnitMeta returns information about the "best" entity (module, path or directory) with
// the given path. The module and version arguments provide additional constraints.
// If the module is unknown, pass internal.UnknownModulePath; if the version is unknown, pass
// internal.LatestVersion.
//
// The rules for picking the best are:
// 1. If the version is known but the module path is not, choose the longest module path
//    at that version that contains fullPath.
// 2. Otherwise, find the latest "good" version (in the modules table) that contains fullPath.
//    a. First, follow the algorithm of the go command: prefer longer module paths, and
//       find the latest unretracted version, using semver but preferring release to pre-release.
//    b. If no modules have latest-version information, find the latest by sorting the versions
//       we do have: again first by module path length, then by version.
func (db *DB) GetUnitMeta(ctx context.Context, fullPath, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.WrapStack(&err, "DB.GetUnitMeta(ctx, %q, %q, %q)", fullPath, requestedModulePath, requestedVersion)
	defer middleware.ElapsedStat(ctx, "GetUnitMeta")()

	if experiment.IsActive(ctx, internal.ExperimentUnitMetaWithLatest) {
		modulePath := requestedModulePath
		version := requestedVersion
		var lmv *internal.LatestModuleVersions
		if requestedVersion == internal.LatestVersion {
			modulePath, version, lmv, err = db.getLatestUnitVersion(ctx, fullPath, requestedModulePath)
			if err != nil {
				return nil, err
			}
		}
		return db.getUnitMetaWithKnownLatestVersion(ctx, fullPath, modulePath, version, lmv)
	}
	return db.legacyGetUnitMeta(ctx, fullPath, requestedModulePath, requestedVersion)
}

func (db *DB) getUnitMetaWithKnownLatestVersion(ctx context.Context, fullPath, modulePath, version string, lmv *internal.LatestModuleVersions) (_ *internal.UnitMeta, err error) {
	defer derrors.WrapStack(&err, "getUnitMetaKnownVersion")

	query := squirrel.Select(
		"m.module_path",
		"m.version",
		"m.commit_time",
		"m.source_info",
		"m.has_go_mod",
		"m.redistributable",
		"u.name",
		"u.redistributable",
		"u.license_types",
		"u.license_paths").
		From("modules m").
		Join("units u on u.module_id = m.id").
		Join("paths p ON p.id = u.path_id").Where(squirrel.Eq{"p.path": fullPath}).
		PlaceholderFormat(squirrel.Dollar)

	if internal.DefaultBranches[version] {
		query = query.
			Join("version_map vm ON m.id = vm.module_id").
			Where("vm.requested_version = ?", version)
	} else {
		query = query.Where(squirrel.Eq{"version": version})
	}
	if modulePath == internal.UnknownModulePath {
		// If we don't know the module, look for the one  with the longest series path.
		query = query.OrderBy("m.series_path DESC").Limit(1)
	} else {
		query = query.Where(squirrel.Eq{"m.module_path": modulePath})
	}

	q, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}
	var (
		licenseTypes []string
		licensePaths []string
		um           = internal.UnitMeta{Path: fullPath}
	)
	err = db.db.QueryRow(ctx, q, args...).Scan(
		&um.ModulePath,
		&um.Version,
		&um.CommitTime,
		jsonbScanner{&um.SourceInfo},
		&um.HasGoMod,
		&um.ModuleInfo.IsRedistributable,
		&um.Name,
		&um.IsRedistributable,
		pq.Array(&licenseTypes),
		pq.Array(&licensePaths))
	if err == sql.ErrNoRows {
		return nil, derrors.NotFound
	}
	if err != nil {
		return nil, err
	}

	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}

	if db.bypassLicenseCheck {
		um.IsRedistributable = true
	}
	um.Licenses = lics

	if experiment.IsActive(ctx, internal.ExperimentRetractions) {
		// If we don't have the latest version information, try to get it.
		// We can be here if there is really no info (in which case we are repeating
		// some work, but it's fast), or if we are ignoring the info (for instance,
		// if all versions were retracted).
		if lmv == nil {
			lmv, err = db.GetLatestModuleVersions(ctx, um.ModulePath)
			if err != nil {
				return nil, err
			}
		}
		if lmv != nil {
			lmv.PopulateModuleInfo(&um.ModuleInfo)
		}
	}
	return &um, nil
}

// getLatestUnitVersion gets the latest version of requestedModulePath that contains fullPath.
// See GetUnitMeta for more details.
func (db *DB) getLatestUnitVersion(ctx context.Context, fullPath, requestedModulePath string) (
	modulePath, latestVersion string, lmv *internal.LatestModuleVersions, err error) {

	defer derrors.WrapStack(&err, "getLatestUnitVersion(%q, %q)", fullPath, requestedModulePath)

	modPaths := []string{requestedModulePath}
	// If we don't know the module path, try each possible module path from longest to shortest.
	if requestedModulePath == internal.UnknownModulePath {
		modPaths = internal.CandidateModulePaths(fullPath)
	}
	// Get latest-version information for all possible modules, from longest
	// to shortest path.
	lmvs, err := db.getMultiLatestModuleVersions(ctx, modPaths)
	if err != nil {
		return "", "", nil, err
	}
	for _, lmv = range lmvs {
		// Collect all the versions of this module that contain fullPath.
		query := squirrel.Select("m.version").
			From("modules m").
			Join("units u on u.module_id = m.id").
			Join("paths p ON p.id = u.path_id").
			Where(squirrel.Eq{"m.module_path": lmv.ModulePath}).
			Where(squirrel.Eq{"p.path": fullPath})
		q, args, err := query.PlaceholderFormat(squirrel.Dollar).ToSql()
		if err != nil {
			return "", "", nil, err
		}
		allVersions, err := collectStrings(ctx, db.db, q, args...)
		if err != nil {
			return "", "", nil, err
		}
		// Remove retracted versions.
		unretractedVersions := version.RemoveIf(allVersions, lmv.IsRetracted)
		// If there are no unretracted versions, move on. If we fall out of the
		// loop we will pick the latest retracted version.
		if len(unretractedVersions) == 0 {
			continue
		}
		// Choose the latest version.
		// If the cooked latest version is compatible, then by the logic of
		// internal/version.Latest (which matches the go command), either
		// incompatible versions should be ignored or there were no incompatible
		// versions. In either case, remove them.
		if !version.IsIncompatible(lmv.CookedVersion) {
			unretractedVersions = version.RemoveIf(unretractedVersions, version.IsIncompatible)
		}
		latestVersion = version.LatestOf(unretractedVersions)
		break
	}
	if latestVersion != "" {
		return lmv.ModulePath, latestVersion, lmv, nil
	}
	// If we don't have latest-version info for any path (or there are no
	// unretracted versions for paths where we do), fall back to finding the
	// latest good version from the longest path. We can't determine
	// deprecations or retractions, and the "go get" command won't download the
	// module unless a specific version is supplied. But we can still show the
	// latest version we have.
	query := squirrel.Select("m.module_path", "m.version").
		From("modules m").
		Join("units u on u.module_id = m.id").
		Join("paths p ON p.id = u.path_id").
		Where(squirrel.Eq{"p.path": fullPath}).
		// Like the go command, order first by path length, then by release
		// version, then prerelease. Without latest-version information, we
		// ignore all adjustments for incompatible and retracted versions.
		OrderBy("m.series_path DESC", "m.version_type = 'release' DESC", "m.sort_version DESC").
		Limit(1)
	q, args, err := query.PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return "", "", nil, err
	}
	err = db.db.QueryRow(ctx, q, args...).Scan(&modulePath, &latestVersion)
	if err == sql.ErrNoRows {
		return "", "", nil, derrors.NotFound
	}
	if err != nil {
		return "", "", nil, err
	}
	return modulePath, latestVersion, nil, nil
}

func (db *DB) legacyGetUnitMeta(ctx context.Context, fullPath, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.WrapStack(&err, "DB.legacyGetUnitMeta(ctx, %q, %q, %q)", fullPath, requestedModulePath, requestedVersion)

	var (
		q    string
		args []interface{}
	)
	q, args, err = getUnitMetaQuery(fullPath, requestedModulePath, requestedVersion).PlaceholderFormat(squirrel.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("squirrel.ToSql: %v", err)
	}
	var (
		licenseTypes []string
		licensePaths []string
		um           = internal.UnitMeta{Path: fullPath}
	)
	err = db.db.QueryRow(ctx, q, args...).Scan(
		&um.ModulePath,
		&um.Version,
		&um.CommitTime,
		jsonbScanner{&um.SourceInfo},
		&um.HasGoMod,
		&um.ModuleInfo.IsRedistributable,
		&um.Name,
		&um.IsRedistributable,
		pq.Array(&licenseTypes),
		pq.Array(&licensePaths))
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return nil, err
		}

		if db.bypassLicenseCheck {
			um.IsRedistributable = true
		}

		um.Licenses = lics
		if err := populateLatestInfo(ctx, db, &um.ModuleInfo); err != nil {
			return nil, err
		}
		return &um, nil
	default:
		return nil, err
	}
}

func getUnitMetaQuery(fullPath, requestedModulePath, requestedVersion string) squirrel.SelectBuilder {
	query := squirrel.Select(
		"m.module_path",
		"m.version",
		"m.commit_time",
		"m.source_info",
		"m.has_go_mod",
		"m.redistributable",
		"u.name",
		"u.redistributable",
		"u.license_types",
		"u.license_paths",
	)
	if requestedVersion != internal.LatestVersion {
		query = query.From("modules m").
			Join("units u on u.module_id = m.id").
			Join("paths p ON p.id = u.path_id").Where(squirrel.Eq{"p.path": fullPath})
		if requestedModulePath != internal.UnknownModulePath {
			query = query.Where(squirrel.Eq{"m.module_path": requestedModulePath})
		}
		if internal.DefaultBranches[requestedVersion] {
			query = query.Join("version_map vm ON m.id = vm.module_id").Where("vm.requested_version = ? ", requestedVersion)
		} else if requestedVersion != internal.LatestVersion {
			query = query.Where(squirrel.Eq{"version": requestedVersion})
		}
		return orderByLatest(query).Limit(1)
	}

	// Use a nested select to fetch the latest version of the unit, then JOIN
	// on units to fetch other relevant information. This allows us to use the
	// index on units.id and paths.path to get the latest path. We can then
	// look up only the relevant information from the units table.
	nestedSelect := orderByLatest(squirrel.Select(
		"m.id",
		"m.module_path",
		"m.version",
		"m.commit_time",
		"m.source_info",
		"m.has_go_mod",
		"m.redistributable",
		"u.id AS unit_id",
	).From("modules m").
		Join("units u ON u.module_id = m.id").
		Join("paths p ON p.id = u.path_id").
		Where(squirrel.Eq{"p.path": fullPath}))
	if requestedModulePath != internal.UnknownModulePath {
		nestedSelect = nestedSelect.Where(squirrel.Eq{"m.module_path": requestedModulePath})
	}
	nestedSelect = nestedSelect.Limit(1)
	return query.From("units u").JoinClause(nestedSelect.Prefix("JOIN (").Suffix(") m ON u.id = m.unit_id"))
}

// orderByLatest orders paths according to the go command.
// Versions are ordered by:
// (1) release (non-incompatible)
// (2) prerelease (non-incompatible)
// (3) release, incompatible
// (4) prerelease, incompatible
// (5) pseudo
// They are then sorted based on semver, then decreasing module path length (so
// that nested modules are preferred).
func orderByLatest(q squirrel.SelectBuilder) squirrel.SelectBuilder {
	return q.OrderBy(
		`CASE
			WHEN m.version_type = 'release' AND NOT m.incompatible THEN 1
			WHEN m.version_type = 'prerelease' AND NOT m.incompatible THEN 2
			WHEN m.version_type = 'release' THEN 3
			WHEN m.version_type = 'prerelease' THEN 4
			ELSE 5
		END`,
		"m.series_path DESC",
		"m.sort_version DESC",
	).PlaceholderFormat(squirrel.Dollar)
}

// GetUnit returns a unit from the database, along with all of the data
// associated with that unit.
func (db *DB) GetUnit(ctx context.Context, um *internal.UnitMeta, fields internal.FieldSet) (_ *internal.Unit, err error) {
	defer derrors.WrapStack(&err, "GetUnit(ctx, %q, %q, %q)", um.Path, um.ModulePath, um.Version)

	u := &internal.Unit{UnitMeta: *um}
	if fields&internal.WithMain != 0 {
		u, err = db.getUnitWithAllFields(ctx, um)
		if err != nil {
			return nil, err
		}
	}
	if fields&internal.WithImports == 0 &&
		fields&internal.WithLicenses == 0 &&
		fields&internal.WithSymbols == 0 {
		return u, nil
	}

	defer middleware.ElapsedStat(ctx, "GetUnit")()
	unitID, err := db.getUnitID(ctx, um.Path, um.ModulePath, um.Version)
	if err != nil {
		return nil, err
	}

	if fields&internal.WithSymbols != 0 {
		symbols, err := getUnitSymbols(ctx, db.db, unitID)
		if err != nil {
			return nil, err
		}
		// Return nil if there are no symbols, instead of an empty map.
		if len(u.Symbols) > 0 {
			u.Symbols = symbols
		}
	}
	if fields&internal.WithImports != 0 {
		imports, err := db.getImports(ctx, unitID)
		if err != nil {
			return nil, err
		}
		if len(imports) > 0 {
			u.Imports = imports
			u.NumImports = len(imports)
		}
	}
	if fields&internal.WithLicenses != 0 {
		lics, err := db.getLicenses(ctx, u.Path, u.ModulePath, unitID)
		if err != nil {
			return nil, err
		}
		u.LicenseContents = lics
	}
	if db.bypassLicenseCheck {
		u.IsRedistributable = true
	} else {
		u.RemoveNonRedistributableData()
	}
	return u, nil
}

func (db *DB) getUnitID(ctx context.Context, fullPath, modulePath, resolvedVersion string) (_ int, err error) {
	defer derrors.WrapStack(&err, "getUnitID(ctx, %q, %q, %q)", fullPath, modulePath, resolvedVersion)
	defer middleware.ElapsedStat(ctx, "getUnitID")()
	var unitID int
	query := `
		SELECT u.id
		FROM units u
		INNER JOIN paths p ON (p.id = u.path_id)
		INNER JOIN modules m ON (u.module_id = m.id)
		WHERE
			p.path = $1
			AND m.module_path = $2
			AND m.version = $3;`
	err = db.db.QueryRow(ctx, query, fullPath, modulePath, resolvedVersion).Scan(&unitID)
	switch err {
	case sql.ErrNoRows:
		return 0, derrors.NotFound
	case nil:
		return unitID, nil
	default:
		return 0, err
	}
}

// getImports returns the imports corresponding to unitID.
func (db *DB) getImports(ctx context.Context, unitID int) (_ []string, err error) {
	defer derrors.WrapStack(&err, "getImports(ctx, %d)", unitID)
	defer middleware.ElapsedStat(ctx, "getImports")()
	return collectStrings(ctx, db.db, `
		SELECT to_path
		FROM package_imports
		WHERE unit_id = $1`, unitID)
}

// getPackagesInUnit returns all of the packages in a unit from a
// module version, including the package that lives at fullPath, if present.
func (db *DB) getPackagesInUnit(ctx context.Context, fullPath, modulePath, resolvedVersion string) (_ []*internal.PackageMeta, err error) {
	defer derrors.WrapStack(&err, "DB.getPackagesInUnit(ctx, %q, %q, %q)", fullPath, modulePath, resolvedVersion)
	defer middleware.ElapsedStat(ctx, "getPackagesInUnit")()

	query := `
		SELECT
			p.path,
			u.name,
			u.redistributable,
			d.synopsis,
			d.GOOS,
			d.GOARCH,
			u.license_types,
			u.license_paths
		FROM modules m
		INNER JOIN units u
		ON u.module_id = m.id
		INNER JOIN paths p
		ON p.id = u.path_id
		LEFT JOIN documentation d
		ON d.unit_id = u.id
		WHERE
			m.module_path = $1
			AND m.version = $2
			AND u.name != '';`

	// If a package has more than build context (GOOS/GOARCH pair), it will have
	// more than one row in documentation, and this query will produce multiple
	// rows for that package. If we could sort the build contexts in SQL we
	// could deal with that in the query, but we must sort in code, so we read
	// all the rows and pick the right one afterwards.
	type pmbc struct {
		pm *internal.PackageMeta
		bc internal.BuildContext
	}
	packagesByPath := map[string][]pmbc{}
	collect := func(rows *sql.Rows) error {
		var (
			pkg          internal.PackageMeta
			licenseTypes []string
			licensePaths []string
			bc           internal.BuildContext
		)
		if err := rows.Scan(
			&pkg.Path,
			&pkg.Name,
			&pkg.IsRedistributable,
			database.NullIsEmpty(&pkg.Synopsis),
			database.NullIsEmpty(&bc.GOOS),
			database.NullIsEmpty(&bc.GOARCH),
			pq.Array(&licenseTypes),
			pq.Array(&licensePaths),
		); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		if fullPath == stdlib.ModulePath || pkg.Path == fullPath || strings.HasPrefix(pkg.Path, fullPath+"/") {
			lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
			if err != nil {
				return err
			}
			pkg.Licenses = lics
			packagesByPath[pkg.Path] = append(packagesByPath[pkg.Path], pmbc{&pkg, bc})
		}
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, modulePath, resolvedVersion); err != nil {
		return nil, err
	}

	var packages []*internal.PackageMeta
	for _, ps := range packagesByPath {
		sort.Slice(ps, func(i, j int) bool { return internal.CompareBuildContexts(ps[i].bc, ps[j].bc) < 0 })
		packages = append(packages, ps[0].pm)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Path < packages[j].Path })
	for _, p := range packages {
		if db.bypassLicenseCheck {
			p.IsRedistributable = true
		} else {
			p.RemoveNonRedistributableData()
		}
	}
	return packages, nil
}

func (db *DB) getUnitWithAllFields(ctx context.Context, um *internal.UnitMeta) (_ *internal.Unit, err error) {
	defer derrors.WrapStack(&err, "getUnitWithAllFields(ctx, %q, %q, %q)", um.Path, um.ModulePath, um.Version)
	defer middleware.ElapsedStat(ctx, "getUnitWithAllFields")()

	// Get README and import counts.
	query := `
        SELECT
			u.id,
			r.file_path,
			r.contents,
			COALESCE((
				SELECT COUNT(unit_id)
				FROM package_imports
				WHERE unit_id = u.id
				GROUP BY unit_id
				), 0) AS num_imports,
			COALESCE((
				SELECT imported_by_count
				FROM search_documents
				-- Only package_path is needed b/c it is the PK for
				-- search_documents.
				WHERE package_path = $1
				), 0) AS num_imported_by
		FROM units u
		INNER JOIN paths p
		ON p.id = u.path_id
		INNER JOIN modules m
		ON u.module_id = m.id
		LEFT JOIN readmes r
		ON r.unit_id = u.id
		WHERE
			p.path = $1
			AND m.module_path = $2
			AND m.version = $3;`

	var (
		unitID int
		r      internal.Readme
		u      internal.Unit
	)
	err = db.db.QueryRow(ctx, query, um.Path, um.ModulePath, um.Version).Scan(
		&unitID,
		database.NullIsEmpty(&r.Filepath),
		database.NullIsEmpty(&r.Contents),
		&u.NumImports,
		&u.NumImportedBy,
	)
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		if r.Filepath != "" && um.ModulePath != stdlib.ModulePath {
			u.Readme = &r
		}
	default:
		return nil, err
	}

	// Get documentation. There can be multiple rows.
	query = `
		SELECT goos, goarch, synopsis, source
		FROM documentation
		WHERE unit_id = $1
	`
	err = db.db.RunQuery(ctx, query, func(rows *sql.Rows) error {
		var d internal.Documentation
		if err := rows.Scan(&d.GOOS, &d.GOARCH, &d.Synopsis, &d.Source); err != nil {
			return err
		}
		u.Documentation = append(u.Documentation, &d)
		return nil
	}, unitID)
	if err != nil {
		return nil, err
	}
	// Sort documentation by GOOS/GOARCH.
	sort.Slice(u.Documentation, func(i, j int) bool {
		ci := u.Documentation[i].BuildContext()
		cj := u.Documentation[j].BuildContext()
		return internal.CompareBuildContexts(ci, cj) < 0
	})

	// Get other info.
	pkgs, err := db.getPackagesInUnit(ctx, um.Path, um.ModulePath, um.Version)
	if err != nil {
		return nil, err
	}
	u.Subdirectories = pkgs
	u.UnitMeta = *um
	return &u, nil
}

type dbPath struct {
	id              int64
	path            string
	moduleID        int64
	name            string
	licenseTypes    []string
	licensePaths    []string
	redistributable bool
}

func (db *DB) getPathsInModule(ctx context.Context, modulePath, resolvedVersion string) (_ []*dbPath, err error) {
	defer derrors.WrapStack(&err, "DB.getPathsInModule(ctx, %q, %q)", modulePath, resolvedVersion)
	query := `
	SELECT
		u.id,
		p.path,
		u.module_id,
		u.name,
		u.license_types,
		u.license_paths,
		u.redistributable
	FROM
		units u
	INNER JOIN
		paths p
	ON
		p.id = u.path_id
	INNER JOIN
		modules m
	ON
		u.module_id = m.id
	WHERE
		m.module_path = $1
		AND m.version = $2
	ORDER BY path;`

	var paths []*dbPath
	collect := func(rows *sql.Rows) error {
		var p dbPath
		if err := rows.Scan(&p.id, &p.path, &p.moduleID, &p.name, pq.Array(&p.licenseTypes),
			pq.Array(&p.licensePaths), &p.redistributable); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		paths = append(paths, &p)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, modulePath, resolvedVersion); err != nil {
		return nil, err
	}
	return paths, nil
}

// GetModuleReadme returns the README corresponding to the modulePath and version.
func (db *DB) GetModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (_ *internal.Readme, err error) {
	defer derrors.WrapStack(&err, "GetModuleReadme(ctx, %q, %q)", modulePath, resolvedVersion)
	var readme internal.Readme
	err = db.db.QueryRow(ctx, `
		SELECT file_path, contents
		FROM modules m
		INNER JOIN units u
		ON u.module_id = m.id
		INNER JOIN paths p
		ON u.path_id = p.id
		INNER JOIN readmes r
		ON u.id = r.unit_id
		WHERE
		    m.module_path=$1
			AND m.version=$2
			AND m.module_path=p.path`, modulePath, resolvedVersion).Scan(&readme.Filepath, &readme.Contents)
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		return &readme, nil
	default:
		return nil, err
	}
}
