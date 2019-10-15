// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/xerrors"
)

// GetPackage returns the first package from the database that has path and
// version.
//
// The returned error may be checked with
// xerrors.Is(err, derrors.InvalidArgument) to determine if it was caused by an
// invalid path or version.
func (db *DB) GetPackage(ctx context.Context, pkgPath, version string) (_ *internal.VersionedPackage, err error) {
	defer derrors.Wrap(&err, "DB.GetPackage(ctx, %q, %q)", pkgPath, version)
	if pkgPath == "" || version == "" {
		return nil, xerrors.Errorf("neither path nor version can be empty: %w", derrors.InvalidArgument)
	}
	return db.getPackage(ctx, pkgPath, version, "")
}

// GetPackageInModuleVersion returns a package from the database with pkgPath,
// modulePath and version.
//
// The returned error may be checked with
// xerrors.Is(err, derrors.InvalidArgument) to determine if it was caused by an
// invalid path or version.
func (db *DB) GetPackageInModuleVersion(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.VersionedPackage, err error) {
	defer derrors.Wrap(&err, "DB.GetPackageInModuleVersion(ctx, %q, %q, %q)", pkgPath, modulePath, version)
	if pkgPath == "" || modulePath == "" || version == "" {
		return nil, xerrors.Errorf("none of pkgPath, modulePath, or version can be empty: %w", derrors.InvalidArgument)
	}
	if version == internal.LatestVersion {
		return nil, xerrors.Errorf("version must be a specific semantic version: %w", derrors.InvalidArgument)
	}
	return db.getPackage(ctx, pkgPath, version, modulePath)
}

func (db *DB) getPackage(ctx context.Context, pkgPath, version, modulePath string) (_ *internal.VersionedPackage, err error) {
	args := []interface{}{pkgPath}
	query := `
		SELECT
			p.path,
			p.name,
			p.synopsis,
			p.v1_path,
			p.license_types,
			p.license_paths,
			p.documentation,
			p.goos,
			p.goarch,
			v.version,
			v.commit_time,
			v.readme_file_path,
			v.readme_contents,
			v.module_path,
			v.version_type,
			v.vcs_type,
		    v.source_info
		FROM
			versions v
		INNER JOIN
			packages p
		ON
			p.module_path = v.module_path
			AND v.version = p.version`

	if version == internal.LatestVersion {
		query += `
			WHERE
				p.path = $1
			ORDER BY
				-- Order the versions by release then prerelease.
				-- The default version should be the first release
				-- version available, if one exists.
				CASE WHEN v.prerelease = '~' THEN 0 ELSE 1 END,
				v.major DESC,
				v.minor DESC,
				v.patch DESC,
				v.prerelease DESC
			LIMIT 1;`
	} else if modulePath == "" {
		query += `
			WHERE
				p.path = $1
				AND p.version = $2
			ORDER BY
				p.module_path DESC
			LIMIT 1;`
		args = append(args, version)
	} else {
		query += `
			WHERE
				p.path = $1
				AND p.version = $2
				AND p.module_path = $3`
		args = append(args, version, modulePath)
	}

	var (
		pkg                        internal.VersionedPackage
		licenseTypes, licensePaths []string
	)
	row := db.queryRow(ctx, query, args...)
	err = row.Scan(&pkg.Path, &pkg.Name, &pkg.Synopsis,
		&pkg.V1Path, pq.Array(&licenseTypes), pq.Array(&licensePaths),
		&pkg.DocumentationHTML, nullIsEmpty(&pkg.GOOS), nullIsEmpty(&pkg.GOARCH), &pkg.Version,
		&pkg.CommitTime, &pkg.ReadmeFilePath, &pkg.ReadmeContents, &pkg.ModulePath,
		&pkg.VersionType, nullIsEmpty(&pkg.VCSType), sourceInfoScanner{&pkg.SourceInfo})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, xerrors.Errorf("package %s@%s: %w", pkgPath, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	pkg.RepositoryURL = pkg.SourceInfo.RepoURL()
	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}
	pkg.Licenses = lics
	return &pkg, nil
}
