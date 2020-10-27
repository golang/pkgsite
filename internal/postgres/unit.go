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

// GetUnit returns a unit from the database, along with all of the
// data associated with that unit.
// TODO(golang/go#39629): remove pID.
func (db *DB) GetUnit(ctx context.Context, um *internal.UnitMeta, fields internal.FieldSet) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(ctx, %q, %q, %q)", um.Path, um.ModulePath, um.Version)
	if experiment.IsActive(ctx, internal.ExperimentGetUnitWithOneQuery) && fields&internal.WithDocumentation|fields&internal.WithReadme != 0 {
		return db.getUnitWithAllFields(ctx, um)
	}

	defer middleware.ElapsedStat(ctx, "GetUnit")()
	pathID, err := db.getPathID(ctx, um.Path, um.ModulePath, um.Version)
	if err != nil {
		return nil, err
	}

	u := &internal.Unit{UnitMeta: *um}
	if fields&internal.WithReadme != 0 {
		var readme *internal.Readme
		if experiment.IsActive(ctx, internal.ExperimentUnitPage) {
			readme, err = db.getReadme(ctx, pathID)
		} else {
			readme, err = db.getModuleReadme(ctx, u.ModulePath, u.Version)
		}
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		u.Readme = readme
	}
	if fields&internal.WithDocumentation != 0 {
		doc, err := db.getDocumentation(ctx, pathID)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		u.Documentation = doc
	}
	if fields&internal.WithImports != 0 {
		imports, err := db.getImports(ctx, pathID)
		if err != nil {
			return nil, err
		}
		if len(imports) > 0 {
			u.Imports = imports
			u.NumImports = len(imports)
		}
	}
	if fields&internal.WithLicenses != 0 {
		lics, err := db.getLicenses(ctx, u.Path, u.ModulePath, pathID)
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

func (db *DB) getPathID(ctx context.Context, fullPath, modulePath, resolvedVersion string) (_ int, err error) {
	defer derrors.Wrap(&err, "getPathID(ctx, %q, %q, %q)", fullPath, modulePath, resolvedVersion)
	defer middleware.ElapsedStat(ctx, "getPathID")()
	var pathID int
	query := `
		SELECT p.id
		FROM paths p
		INNER JOIN modules m ON (p.module_id = m.id)
		WHERE
		    p.path = $1
		    AND m.module_path = $2
		    AND m.version = $3;`
	err = db.db.QueryRow(ctx, query, fullPath, modulePath, resolvedVersion).Scan(&pathID)
	switch err {
	case sql.ErrNoRows:
		return 0, derrors.NotFound
	case nil:
		return pathID, nil
	default:
		return 0, err
	}
}

// getDocumentation returns the documentation corresponding to pathID.
func (db *DB) getDocumentation(ctx context.Context, pathID int) (_ *internal.Documentation, err error) {
	defer derrors.Wrap(&err, "getDocumentation(ctx, %d)", pathID)
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
		    d.path_id=$1;`, pathID).Scan(
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
func (db *DB) getReadme(ctx context.Context, pathID int) (_ *internal.Readme, err error) {
	defer derrors.Wrap(&err, "getReadme(ctx, %d)", pathID)
	defer middleware.ElapsedStat(ctx, "getReadme")()
	var readme internal.Readme
	err = db.db.QueryRow(ctx, `
		SELECT file_path, contents
		FROM readmes
		WHERE path_id=$1;`, pathID).Scan(&readme.Filepath, &readme.Contents)
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
		INNER JOIN paths p
		ON p.module_id = m.id
		INNER JOIN readmes r
		ON p.id = r.path_id
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

// getImports returns the imports corresponding to pathID.
func (db *DB) getImports(ctx context.Context, pathID int) (_ []string, err error) {
	defer derrors.Wrap(&err, "getImports(ctx, %d)", pathID)
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
		WHERE path_id = $1`, collect, pathID); err != nil {
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
			p.path,
			p.name,
			p.redistributable,
			d.synopsis,
			p.license_types,
			p.license_paths
		FROM modules m
		INNER JOIN paths p
		ON p.module_id = m.id
		INNER JOIN documentation d
		ON d.path_id = p.id
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
				SELECT COUNT(path_id)
				FROM package_imports
				WHERE path_id = p.id
				GROUP BY path_id
				), 0) AS num_imports,
			COALESCE((
				SELECT imported_by_count
				FROM search_documents
				-- Only package_path is needed b/c it is the PK for
				-- search_documents.
				WHERE package_path = $1
				), 0) AS num_imported_by
		FROM paths p
		INNER JOIN modules m
		ON p.module_id = m.id
		LEFT JOIN documentation d
		ON d.path_id = p.id
		LEFT JOIN readmes r
		ON r.path_id = p.id
		WHERE
			p.path = $1
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
