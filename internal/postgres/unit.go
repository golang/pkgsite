// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
func (db *DB) GetUnitMeta(ctx context.Context, path, requestedModulePath, requestedVersion string) (_ *internal.UnitMeta, err error) {
	defer derrors.Wrap(&err, "DB.GetUnitMeta(ctx, %q, %q, %q)", path, requestedModulePath, requestedVersion)
	defer middleware.ElapsedStat(ctx, "GetUnitMeta")()

	var (
		constraints []string
		joinStmt    string
	)
	args := []interface{}{path}
	if requestedModulePath != internal.UnknownModulePath {
		constraints = append(constraints, fmt.Sprintf("AND m.module_path = $%d", len(args)+1))
		args = append(args, requestedModulePath)
	}
	switch requestedVersion {
	case internal.LatestVersion:
	case internal.MasterVersion:
		joinStmt = "INNER JOIN version_map vm ON (vm.module_id = m.id)"
		constraints = append(constraints, "AND vm.requested_version = 'master'")
	default:
		constraints = append(constraints, fmt.Sprintf("AND m.version = $%d", len(args)+1))
		args = append(args, requestedVersion)
	}

	var (
		licenseTypes []string
		licensePaths []string
		um           = internal.UnitMeta{Path: path}
	)
	query := fmt.Sprintf(`
		SELECT
			m.module_path,
			m.version,
			m.commit_time,
			m.source_info,
			u.name,
			u.redistributable,
			u.license_types,
			u.license_paths
		FROM units u
		INNER JOIN modules m ON (u.module_id = m.id)
		%s
		WHERE u.path = $1
		%s
		%s
		LIMIT 1
	`, joinStmt, strings.Join(constraints, " "), orderByLatest)
	err = db.db.QueryRow(ctx, query, args...).Scan(
		&um.ModulePath,
		&um.Version,
		&um.CommitTime,
		jsonbScanner{&um.SourceInfo},
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

// orderByLatest orders paths according to the go command.
// Versions are ordered by:
// (1) release (non-incompatible)
// (2) prerelease (non-incompatible)
// (3) release, incompatible
// (4) prerelease, incompatible
// (5) pseudo
// They are then sorted based on semver, then decreasing module path length (so
// that nested modules are preferred).
const orderByLatest = `
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

// GetUnit returns a unit from the database, along with all of the
// data associated with that unit.
// TODO(golang/go#39629): remove pID.
func (db *DB) GetUnit(ctx context.Context, um *internal.UnitMeta, fields internal.FieldSet) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(ctx, %q, %q, %q)", um.Path, um.ModulePath, um.Version)
	if experiment.IsActive(ctx, internal.ExperimentGetUnitWithOneQuery) && fields&internal.WithDocumentation|fields&internal.WithReadme != 0 {
		return db.getUnitWithAllFields(ctx, um)
	}

	defer middleware.ElapsedStat(ctx, "GetUnit")()
	unitID, err := db.getUnitID(ctx, um.Path, um.ModulePath, um.Version)
	if err != nil {
		return nil, err
	}

	u := &internal.Unit{UnitMeta: *um}
	if fields&internal.WithReadme != 0 {
		var readme *internal.Readme
		if experiment.IsActive(ctx, internal.ExperimentUnitPage) {
			readme, err = db.getReadme(ctx, unitID)
		} else {
			readme, err = db.getModuleReadme(ctx, u.ModulePath, u.Version)
		}
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		u.Readme = readme
	}
	if fields&internal.WithDocumentation != 0 {
		doc, err := db.getDocumentation(ctx, unitID)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		u.Documentation = doc
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
	if fields&internal.WithSubdirectories != 0 {
		pkgs, err := db.getPackagesInUnit(ctx, u.Path, u.ModulePath, u.Version)
		if err != nil {
			return nil, err
		}
		u.Subdirectories = pkgs
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

// getDocumentation returns the documentation corresponding to unitID.
func (db *DB) getDocumentation(ctx context.Context, unitID int) (_ *internal.Documentation, err error) {
	defer derrors.Wrap(&err, "getDocumentation(ctx, %d)", unitID)
	defer middleware.ElapsedStat(ctx, "getDocumentation")()
	var (
		doc     internal.Documentation
		docHTML string
	)
	err = db.db.QueryRow(ctx, `
		SELECT
			d.goos,
			d.goarch,
			d.synopsis,
			d.html,
			d.source
		FROM documentation d
		WHERE
		    d.unit_id=$1;`, unitID).Scan(
		database.NullIsEmpty(&doc.GOOS),
		database.NullIsEmpty(&doc.GOARCH),
		database.NullIsEmpty(&doc.Synopsis),
		database.NullIsEmpty(&docHTML),
		&doc.Source,
	)
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		doc.HTML = convertDocumentation(docHTML)
		return &doc, nil
	default:
		return nil, err
	}
}

// getReadme returns the README corresponding to the modulePath and version.
func (db *DB) getReadme(ctx context.Context, unitID int) (_ *internal.Readme, err error) {
	defer derrors.Wrap(&err, "getReadme(ctx, %d)", unitID)
	defer middleware.ElapsedStat(ctx, "getReadme")()
	var readme internal.Readme
	err = db.db.QueryRow(ctx, `
		SELECT file_path, contents
		FROM readmes
		WHERE unit_id=$1;`, unitID).Scan(&readme.Filepath, &readme.Contents)
	switch err {
	case sql.ErrNoRows:
		return nil, derrors.NotFound
	case nil:
		return &readme, nil
	default:
		return nil, err
	}
}

// getModuleReadme returns the README corresponding to the modulePath and version.
func (db *DB) getModuleReadme(ctx context.Context, modulePath, resolvedVersion string) (_ *internal.Readme, err error) {
	defer derrors.Wrap(&err, "getModuleReadme(ctx, %q, %q)", modulePath, resolvedVersion)
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
		INNER JOIN documentation d
		ON d.unit_id = u.id
		WHERE
			m.module_path = $1
			AND m.version = $2
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
			&pkg.Synopsis,
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
