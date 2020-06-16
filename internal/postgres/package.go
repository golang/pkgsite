// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/stdlib"
)

// LegacyGetPackage returns the a package from the database with the corresponding
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
// errors.Is(err, derrors.InvalidArgument) to determine if it was caused by an
// invalid pkgPath, modulePath or version.
//
// The returned error may be checked with
// errors.Is(err, derrors.InvalidArgument) to determine if it was caused by an
// invalid path or version.
func (db *DB) LegacyGetPackage(ctx context.Context, pkgPath, modulePath, version string) (_ *internal.LegacyVersionedPackage, err error) {
	defer derrors.Wrap(&err, "DB.LegacyGetPackage(ctx, %q, %q)", pkgPath, version)
	if pkgPath == "" || modulePath == "" || version == "" {
		return nil, fmt.Errorf("none of pkgPath, modulePath, or version can be empty: %w", derrors.InvalidArgument)
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
			p.redistributable,
			p.documentation,
			p.goos,
			p.goarch,
			m.version,
			m.commit_time,
			m.readme_file_path,
			m.readme_contents,
			m.module_path,
			m.version_type,
		    m.source_info,
			m.redistributable,
			m.has_go_mod
		FROM
			modules m
		INNER JOIN
			packages p
		ON
			p.module_path = m.module_path
			AND m.version = p.version`

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
				m.version_type = 'release' DESC,
				m.sort_version DESC,
				m.module_path DESC
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
				m.version_type = 'release' DESC,
				m.sort_version DESC
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
		pkg                        internal.LegacyVersionedPackage
		licenseTypes, licensePaths []string
		hasGoMod                   sql.NullBool
	)
	row := db.db.QueryRow(ctx, query, args...)
	err = row.Scan(&pkg.Path, &pkg.Name, &pkg.Synopsis,
		&pkg.V1Path, pq.Array(&licenseTypes), pq.Array(&licensePaths), &pkg.LegacyPackage.IsRedistributable,
		database.NullIsEmpty(&pkg.DocumentationHTML), &pkg.GOOS, &pkg.GOARCH, &pkg.Version,
		&pkg.CommitTime, database.NullIsEmpty(&pkg.LegacyReadmeFilePath), database.NullIsEmpty(&pkg.LegacyReadmeContents),
		&pkg.ModulePath, &pkg.VersionType, jsonbScanner{&pkg.SourceInfo}, &pkg.LegacyModuleInfo.IsRedistributable,
		&hasGoMod)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("package %s@%s: %w", pkgPath, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	setHasGoMod(&pkg.ModuleInfo, hasGoMod)
	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}
	pkg.Licenses = lics
	return &pkg, nil
}
