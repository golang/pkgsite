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
// module version, including the package that lives at fullPath, if present.
func (db *DB) GetPackagesInUnit(ctx context.Context, fullPath, modulePath, resolvedVersion string) (_ []*internal.PackageMeta, err error) {
	defer derrors.Wrap(&err, "DB.GetPackagesInUnit(ctx, %q, %q, %q)", fullPath, modulePath, resolvedVersion)

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
	pathID, err := db.getPathID(ctx, pi.Path, pi.ModulePath, pi.Version)
	if err != nil {
		return nil, err
	}

	u := &internal.Unit{PathInfo: *pi}
	if fields&internal.WithReadme != 0 {
		readme, err := db.getReadme(ctx, u.ModulePath, u.Version)
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
		if doc != nil {
			u.Package = &internal.Package{
				Path:          u.Path,
				Name:          u.Name,
				Documentation: doc,
			}
		}
	}
	if fields&internal.WithImports != 0 {
		imports, err := db.getImports(ctx, pathID)
		if err != nil {
			return nil, err
		}
		if len(imports) > 0 {
			u.Imports = imports
		}
	}
	if fields == internal.AllFields {
		if u.Name != "" {
			if u.Package == nil {
				u.Package = &internal.Package{
					Path: u.Path,
				}
			}
			u.Package.Name = u.Name
		}
	}
	if !db.bypassLicenseCheck {
		u.RemoveNonRedistributableData()
	}
	return u, nil
}

func (db *DB) getPathID(ctx context.Context, fullPath, modulePath, version string) (_ int, err error) {
	defer derrors.Wrap(&err, "getPathID(ctx, %q, %q, %q)", fullPath, modulePath, version)
	var pathID int
	query := `
		SELECT p.id
		FROM paths p
		INNER JOIN modules m ON (p.module_id = m.id)
		WHERE
		    p.path = $1
		    AND m.module_path = $2
		    AND m.version = $3;`
	err = db.db.QueryRow(ctx, query, fullPath, modulePath, version).Scan(&pathID)
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
