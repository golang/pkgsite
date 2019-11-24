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
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/xerrors"
)

// GetPackage returns the a package from the database with the corresponding
// pkgPath, modulePath and version.
//
// If version = internal.LatestVersion, the package corresponding to
// the latest matching module version will be fetched.
//
// If more than one module tie for a given dirPath and version pair, and
// modulePath = internal.UnknownModulePath, the package in the module with the
// longest module path will be fetched.
// For example, if there are
// two rows in the packages table:
// (1) path = "github.com/hashicorp/vault/api"
//     module_path = "github.com/hashicorp/vault"
// AND
// (2) path = "github.com/hashicorp/vault/api"
//     module_path = "github.com/hashicorp/vault/api"
// The latter will be returned.
//
// The returned error may be checked with
// xerrors.Is(err, derrors.InvalidArgument) to determine if it was caused by an
// invalid pkgPath, modulePath or version.
//
// The returned error may be checked with
// xerrors.Is(err, derrors.InvalidArgument) to determine if it was caused by an
// invalid path or version.
func (db *DB) GetPackage(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.VersionedPackage, err error) {
	defer derrors.Wrap(&err, "DB.GetPackage(ctx, %q, %q)", pkgPath, version)
	if pkgPath == "" || modulePath == "" || version == "" {
		return nil, xerrors.Errorf("none of pkgPath, modulePath, or version can be empty: %w", derrors.InvalidArgument)
	}

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
		    v.source_info
		FROM
			versions v
		INNER JOIN
			packages p
		ON
			p.module_path = v.module_path
			AND v.version = p.version`

	if modulePath == internal.UnknownModulePath || modulePath == stdlib.ModulePath {
		if version == internal.LatestVersion {
			// Only pkgPath is specified, so get the latest version of the
			// package found in any module.
			query += `
			WHERE
				p.path = $1
			ORDER BY
				-- Order the versions by release then prerelease.
				-- The default version should be the first release
				-- version available, if one exists.
				v.version_type = 'release' DESC,
				v.sort_version DESC,
				v.module_path DESC
			LIMIT 1;`
		} else {
			// pkgPath and version are specified, so get that package version
			// from any module.  If it exists in multiple modules, return the
			// one with the longest path.
			query += `
			WHERE
				p.path = $1
				AND p.version = $2
			ORDER BY
				p.module_path DESC
			LIMIT 1;`
			args = append(args, version)
		}
	} else if version == internal.LatestVersion {
		// pkgPath and modulePath are specified, so get the latest version of
		// the package in the specified module.
		query += `
			WHERE
				p.path = $1
				AND p.module_path = $2
			ORDER BY
				-- Order the versions by release then prerelease.
				-- The default version should be the first release
				-- version available, if one exists.
				v.version_type = 'release' DESC,
				v.sort_version DESC
			LIMIT 1;`
		args = append(args, modulePath)
	} else {
		// pkgPath, modulePath and version were all specified. Only one
		// directory should ever match this query.
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
	row := db.db.QueryRow(ctx, query, args...)
	err = row.Scan(&pkg.Path, &pkg.Name, &pkg.Synopsis,
		&pkg.V1Path, pq.Array(&licenseTypes), pq.Array(&licensePaths),
		database.NullIsEmpty(&pkg.DocumentationHTML), &pkg.GOOS, &pkg.GOARCH, &pkg.Version,
		&pkg.CommitTime, database.NullIsEmpty(&pkg.ReadmeFilePath), database.NullIsEmpty(&pkg.ReadmeContents),
		&pkg.ModulePath, &pkg.VersionType, jsonbScanner{&pkg.SourceInfo})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, xerrors.Errorf("package %s@%s: %w", pkgPath, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}
	pkg.Licenses = lics
	return &pkg, nil
}
