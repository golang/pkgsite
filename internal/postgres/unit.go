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
	"golang.org/x/pkgsite/internal/stdlib"
)

// GetPackagesInUnit returns all of the packages in a unit from a
// module version, including the package that lives at dirPath, if present.
func (db *DB) GetPackagesInUnit(ctx context.Context, dirPath, modulePath, resolvedVersion string) (_ []*internal.PackageMeta, err error) {
	defer derrors.Wrap(&err, "DB.GetPackagesInUnit(ctx, %q, %q, %q)", dirPath, modulePath, resolvedVersion)

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
		if dirPath == stdlib.ModulePath || pkg.Path == dirPath || strings.HasPrefix(pkg.Path, dirPath+"/") {
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
	if len(packages) == 0 {
		return nil, fmt.Errorf("unit does not contain any packages: %w", derrors.NotFound)
	}
	if !db.bypassLicenseCheck {
		for _, p := range packages {
			p.RemoveNonRedistributableData()
		}
	}
	return packages, nil
}

// GetUnit returns a unit from the database, along with all of the
// data associated with that unit.
// TODO(golang/go#39629): remove pID.
func (db *DB) GetUnit(ctx context.Context, pi *internal.PathInfo, fields internal.FieldSet) (_ *internal.Unit, err error) {
	defer derrors.Wrap(&err, "GetUnit(ctx, %q, %q, %q)", pi.Path, pi.ModulePath, pi.Version)
	pathID, isRedistributable, err := db.getPathIDAndIsRedistributable(ctx, pi.Path, pi.ModulePath, pi.Version)
	if err != nil {
		return nil, err
	}

	dir := &internal.Unit{
		DirectoryMeta: internal.DirectoryMeta{
			Path:              pi.Path,
			PathID:            pathID,
			IsRedistributable: isRedistributable,
			ModuleInfo: internal.ModuleInfo{
				ModulePath: pi.ModulePath,
				Version:    pi.Version,
			},
		},
	}
	if fields&internal.WithReadme != 0 {
		readme, err := db.getReadme(ctx, dir.ModulePath, dir.Version)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		dir.Readme = readme
	}
	if fields&internal.WithDocumentation != 0 {
		doc, err := db.getDocumentation(ctx, dir.PathID)
		if err != nil && !errors.Is(err, derrors.NotFound) {
			return nil, err
		}
		if doc != nil {
			dir.Package = &internal.Package{
				Path:          dir.Path,
				Documentation: doc,
			}
		}
	}
	if fields&internal.WithImports != 0 {
		imports, err := db.getImports(ctx, dir.PathID)
		if err != nil {
			return nil, err
		}
		if len(imports) > 0 {
			dir.Imports = imports
		}
	}
	if fields == internal.AllFields {
		dmeta, err := db.GetDirectoryMeta(ctx, pi.Path, pi.ModulePath, pi.Version)
		if err != nil {
			return nil, err
		}
		dir.DirectoryMeta = *dmeta
		if dir.Name != "" {
			if dir.Package == nil {
				dir.Package = &internal.Package{Path: dir.Path}
			}
			dir.Package.Name = dmeta.Name
		}
	}
	if !db.bypassLicenseCheck {
		dir.RemoveNonRedistributableData()
	}
	return dir, nil
}

func (db *DB) getPathIDAndIsRedistributable(ctx context.Context, fullPath, modulePath, version string) (_ int, _ bool, err error) {
	defer derrors.Wrap(&err, "getPathID(ctx, %q, %q, %q)", fullPath, modulePath, version)
	var (
		pathID            int
		isRedistributable bool
	)
	query := `
		SELECT p.id, p.redistributable
		FROM paths p
		INNER JOIN modules m ON (p.module_id = m.id)
		WHERE
		    p.path = $1
		    AND m.module_path = $2
		    AND m.version = $3;`
	err = db.db.QueryRow(ctx, query, fullPath, modulePath, version).Scan(&pathID, &isRedistributable)
	switch err {
	case sql.ErrNoRows:
		return 0, false, derrors.NotFound
	case nil:
		return pathID, isRedistributable, nil
	default:
		return 0, false, err
	}
}

// GetDirectoryMeta information about a directory from the database.
func (db *DB) GetDirectoryMeta(ctx context.Context, path, modulePath, version string) (_ *internal.DirectoryMeta, err error) {
	defer derrors.Wrap(&err, "GetDirectoryMeta(ctx, %q, %q, %q)", path, modulePath, version)
	query := `
		SELECT
			m.module_path,
			m.version,
			m.commit_time,
			m.version_type,
			m.redistributable,
			m.has_go_mod,
			m.source_info,
			p.id,
			p.path,
			p.name,
			p.v1_path,
			p.redistributable,
			p.license_types,
			p.license_paths
		FROM modules m
		INNER JOIN paths p
		ON p.module_id = m.id
		WHERE
			p.path = $1
			AND m.module_path = $2
			AND m.version = $3;`
	var (
		mi                         internal.ModuleInfo
		dir                        internal.DirectoryMeta
		licenseTypes, licensePaths []string
	)
	row := db.db.QueryRow(ctx, query, path, modulePath, version)
	if err := row.Scan(
		&mi.ModulePath,
		&mi.Version,
		&mi.CommitTime,
		&mi.VersionType,
		&mi.IsRedistributable,
		&mi.HasGoMod,
		jsonbScanner{&mi.SourceInfo},
		&dir.PathID,
		&dir.Path,
		database.NullIsEmpty(&dir.Name),
		&dir.V1Path,
		&dir.IsRedistributable,
		pq.Array(&licenseTypes),
		pq.Array(&licensePaths),
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("unit %s@%s: %w", path, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}
	dir.ModuleInfo = mi
	dir.Licenses = lics
	return &dir, err
}

// getDocumentation returns the documentation corresponding to pathID.
func (db *DB) getDocumentation(ctx context.Context, pathID int) (_ *internal.Documentation, err error) {
	defer derrors.Wrap(&err, "getDocumentation(ctx, %d)", pathID)
	var (
		doc     internal.Documentation
		docHTML string
	)
	err = db.db.QueryRow(ctx, `
		SELECT
			d.goos,
			d.goarch,
			d.synopsis,
			d.html
		FROM documentation d
		WHERE
		    d.path_id=$1;`, pathID).Scan(
		database.NullIsEmpty(&doc.GOOS),
		database.NullIsEmpty(&doc.GOARCH),
		database.NullIsEmpty(&doc.Synopsis),
		database.NullIsEmpty(&docHTML),
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
func (db *DB) getReadme(ctx context.Context, modulePath, version string) (_ *internal.Readme, err error) {
	defer derrors.Wrap(&err, "getReadme(ctx, %q, %q)", modulePath, version)
	// TODO(golang/go#38513): update to query on PathID and query the readmes
	// table directly once we start displaying READMEs for directories instead
	// of the top-level module.
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
			AND m.module_path=p.path`, modulePath, version).Scan(&readme.Filepath, &readme.Contents)
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
