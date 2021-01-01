// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/stdlib"
)

// GetUnitMeta returns information about the "best" entity (module, path or directory) with
// the given path. The module and version arguments provide additional constraints.
// If the module is unknown, pass internal.UnknownModulePath; if the version is unknown, pass
// internal.LatestVersion.
//
// The rules for picking the best are:
// 1. Match the module path and or version, if they are provided;
// 2. Prefer newer module versions to older, and release to pre-release;
// 3. In the unlikely event of two paths at the same version, pick the longer module path.
func (db *DB) GetUnitMeta(ctx context.Context, fullPath, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "DB.GetUnitMeta(ctx, %q, %q, %q)", fullPath, requestedModulePath, requestedVersion)
	defer middleware.ElapsedStat(ctx, "GetUnitMeta")()

	var (
		q    string
		args []interface{}
	)
	if experiment.IsActive(ctx, internal.ExperimentGetUnitMetaQuery) {
		q, args, err = getUnitMetaQuery(fullPath, requestedModulePath, requestedVersion).PlaceholderFormat(squirrel.Dollar).ToSql()
	} else {
		q, args, err = legacyGetUnitMetaQuery(fullPath, requestedModulePath, requestedVersion).PlaceholderFormat(squirrel.Dollar).ToSql()
	}
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

func legacyGetUnitMetaQuery(fullPath, requestedModulePath, requestedVersion string) squirrel.SelectBuilder {
	query := squirrel.Select(
		"m.module_path",
		"m.version",
		"m.commit_time",
		"m.source_info",
		"m.has_go_mod",
		"u.name",
		"u.redistributable",
		"u.license_types",
		"u.license_paths",
	).From("modules m").
		Join("units u on u.module_id = m.id").
		Where(squirrel.Eq{"u.path": fullPath})
	if requestedModulePath != internal.UnknownModulePath {
		query = query.Where(squirrel.Eq{"m.module_path": requestedModulePath})
	}
	if _, ok := internal.DefaultBranches[requestedVersion]; ok {
		query = query.Join("version_map vm ON m.id = vm.module_id").Where("vm.requested_version = ? ", requestedVersion)
	} else if requestedVersion != internal.LatestVersion {
		query = query.Where(squirrel.Eq{"version": requestedVersion})
	}
	return orderByLatest(query)
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
		"m.module_path DESC",
		"m.sort_version DESC",
	).PlaceholderFormat(squirrel.Dollar)
}

const orderByLatestStmt = `
			ORDER BY
				CASE
					WHEN m.version_type = 'release' AND NOT m.incompatible THEN 1
					WHEN m.version_type = 'prerelease' AND NOT m.incompatible THEN 2
					WHEN m.version_type = 'release' THEN 3
					WHEN m.version_type = 'prerelease' THEN 4
					ELSE 5
				END,
				m.sort_version DESC,
				m.module_path DESC`

// GetUnit returns a unit from the database, along with all of the data
// associated with that unit.
func (db *DB) GetUnit(ctx context.Context, um *internal.UnitMeta, fields internal.FieldSet) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(ctx, %q, %q, %q)", um.Path, um.ModulePath, um.Version)

	u := &internal.Unit{UnitMeta: *um}
	if fields&internal.WithMain != 0 {
		u, err = db.getUnitWithAllFields(ctx, um)
		if err != nil {
			return nil, err
		}
	}
	if fields&internal.WithImports == 0 && fields&internal.WithLicenses == 0 {
		return u, nil
	}

	defer middleware.ElapsedStat(ctx, "GetUnit")()
	unitID, err := db.getUnitID(ctx, um.Path, um.ModulePath, um.Version)
	if err != nil {
		return nil, err
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
	defer derrors.Wrap(&err, "getPathID(ctx, %q, %q, %q)", fullPath, modulePath, resolvedVersion)
	defer middleware.ElapsedStat(ctx, "getPathID")()
	var unitID int
	query := `
		SELECT u.id
		FROM units u
		INNER JOIN modules m ON (u.module_id = m.id)
		WHERE
			u.path = $1
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
	defer derrors.Wrap(&err, "getImports(ctx, %d)", unitID)
	defer middleware.ElapsedStat(ctx, "getImports")()
	var imports []string
	collect := func(rows *sql.Rows) error {
		var path string
		if err := rows.Scan(&path); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		imports = append(imports, path)
		return nil
	}
	if err := db.db.RunQuery(ctx, `
		SELECT to_path
		FROM package_imports
		WHERE unit_id = $1`, collect, unitID); err != nil {
		return nil, err
	}
	return imports, nil
}

// getPackagesInUnit returns all of the packages in a unit from a
// module version, including the package that lives at fullPath, if present.
func (db *DB) getPackagesInUnit(ctx context.Context, fullPath, modulePath, resolvedVersion string) (_ []*internal.PackageMeta, err error) {
	defer derrors.Wrap(&err, "DB.getPackagesInUnit(ctx, %q, %q, %q)", fullPath, modulePath, resolvedVersion)
	defer middleware.ElapsedStat(ctx, "getPackagesInUnit")()

	query := `
		SELECT
			u.path,
			u.name,
			u.redistributable,
			d.synopsis,
			u.license_types,
			u.license_paths
		FROM modules m
		INNER JOIN units u
		ON u.module_id = m.id
		LEFT JOIN documentation d
		ON d.unit_id = u.id
		WHERE
			m.module_path = $1
			AND m.version = $2
			AND u.name != ''
		ORDER BY path;`
	var packages []*internal.PackageMeta
	collect := func(rows *sql.Rows) error {
		var (
			pkg          internal.PackageMeta
			licenseTypes []string
			licensePaths []string
		)
		if err := rows.Scan(
			&pkg.Path,
			&pkg.Name,
			&pkg.IsRedistributable,
			database.NullIsEmpty(&pkg.Synopsis),
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
			packages = append(packages, &pkg)
		}
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, modulePath, resolvedVersion); err != nil {
		return nil, err
	}
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
	defer derrors.Wrap(&err, "getUnitWithAllFields(ctx, %q, %q, %q)", um.Path, um.ModulePath, um.Version)
	defer middleware.ElapsedStat(ctx, "getUnitWithAllFields")()

	query := `
        SELECT
			d.goos,
			d.goarch,
			d.synopsis,
			d.source,
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
		INNER JOIN modules m
		ON u.module_id = m.id
		LEFT JOIN documentation d
		ON d.unit_id = u.id
		LEFT JOIN readmes r
		ON r.unit_id = u.id
		WHERE
			u.path = $1
			AND m.module_path = $2
			AND m.version = $3;`

	var (
		d internal.Documentation
		r internal.Readme
		u internal.Unit
	)
	err = db.db.QueryRow(ctx, query, um.Path, um.ModulePath, um.Version).Scan(
		database.NullIsEmpty(&d.GOOS),
		database.NullIsEmpty(&d.GOARCH),
		database.NullIsEmpty(&d.Synopsis),
		&d.Source,
		database.NullIsEmpty(&r.Filepath),
		database.NullIsEmpty(&r.Contents),
		&u.NumImports,
		&u.NumImportedBy,
	)
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		if d.GOOS != "" {
			u.Documentation = &d
		}
		if r.Filepath != "" {
			u.Readme = &r
		}
	default:
		return nil, err
	}
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
	v1Path          string
	name            string
	licenseTypes    []string
	licensePaths    []string
	redistributable bool
}

func (db *DB) getPathsInModule(ctx context.Context, modulePath, resolvedVersion string) (_ []*dbPath, err error) {
	defer derrors.Wrap(&err, "DB.getPathsInModule(ctx, %q, %q)", modulePath, resolvedVersion)
	query := `
	SELECT
		u.id,
		u.path,
		u.module_id,
		u.v1_path,
		u.name,
		u.license_types,
		u.license_paths,
		u.redistributable
	FROM
		units u
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
		if err := rows.Scan(&p.id, &p.path, &p.moduleID, &p.v1Path, &p.name, pq.Array(&p.licenseTypes),
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
	defer derrors.Wrap(&err, "GetModuleReadme(ctx, %q, %q)", modulePath, resolvedVersion)
	var readme internal.Readme
	err = db.db.QueryRow(ctx, `
		SELECT file_path, contents
		FROM modules m
		INNER JOIN units u
		ON u.module_id = m.id
		INNER JOIN readmes r
		ON u.id = r.unit_id
		WHERE
		    m.module_path=$1
			AND m.version=$2
			AND m.module_path=u.path`, modulePath, resolvedVersion).Scan(&readme.Filepath, &readme.Contents)
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		return &readme, nil
	default:
		return nil, err
	}
}
