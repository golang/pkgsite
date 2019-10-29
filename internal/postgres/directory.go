// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/xerrors"
)

// GetDirectory returns the directory corresponding to the provided dirPath,
// modulePath, and version. The directory will contain all packages for that
// version, in sorted order by package path.
//
// If version = internal.LatestVersion, the directory corresponding to the
// latest matching module version will be fetched.
//
// If more than one module tie for a given dirPath and version pair, and
// modulePath = internal.UnknownModulePath, the directory for the module with
// the  longest module path will be fetched.
// For example, if there are
// two rows in the packages table:
// (1) path = "github.com/hashicorp/vault/api"
//     module_path = "github.com/hashicorp/vault"
// AND
// (2) path = "github.com/hashicorp/vault/api"
//     module_path = "github.com/hashicorp/vault/api"
// Only directories in the latter module will be returned.
//
// Packages will be returned for a given dirPath if: (1) the package path has a
// prefix of dirPath (2) the dirPath has a prefix matching the package's
// module_path
//
// For example, if the package "golang.org/x/tools/go/packages" in module
// "golang.org/x/tools" is in the database, it will match on:
// golang.org/x/tools
// golang.org/x/tools/go
// golang.org/x/tools/go/packages
//
// It will not match on:
// golang.org/x/tools/g
func (db *DB) GetDirectory(ctx context.Context, dirPath, modulePath, version string) (_ *internal.Directory, err error) {
	defer derrors.Wrap(&err, "DB.GetDirectory(ctx, %q, %q, %q)", dirPath, modulePath, version)

	if dirPath == "" || modulePath == "" || version == "" {
		return nil, xerrors.Errorf("none of pkgPath, modulePath, or version can be empty: %w", derrors.InvalidArgument)
	}

	query := `
		WITH parent_directories AS (
			SELECT *
			FROM packages
			WHERE
				tsv_parent_directories @@ ($1 || ':*')::tsquery
				AND (
					CASE WHEN module_path != 'std'
					THEN char_length(module_path) <= char_length($1)
					ELSE TRUE
					END
				)
		)
		SELECT
			p.path,
			p.name,
			p.synopsis,
			p.v1_path,
			p.documentation,
			p.license_types,
			p.license_paths,
			p.goos,
			p.goarch,
			v.version,
			v.module_path,
			v.readme_file_path,
			v.readme_contents,
			v.commit_time,
			v.version_type,
			v.source_info
		FROM
			parent_directories p`

	var args = []interface{}{dirPath}
	if version == internal.LatestVersion {
		query += `
		INNER JOIN (
			SELECT *
			FROM versions v
			WHERE v.module_path IN (
				SELECT module_path
				FROM parent_directories
			)
			ORDER BY
				-- Order the versions by release then prerelease.
				-- The default version should be the first release
				-- version available, if one exists.
				CASE WHEN
					prerelease = '~' THEN 0 ELSE 1 END,
					major DESC,
					minor DESC,
					patch DESC,
					prerelease DESC,
					module_path DESC
			LIMIT 1
		) v
		ON
			p.module_path = v.module_path
			AND p.version = v.version;`
	} else if modulePath == internal.UnknownModulePath {
		query += `
		INNER JOIN (
			SELECT *
			FROM versions
			WHERE module_path IN (
				SELECT module_path FROM parent_directories
			)
			AND version = $2
			ORDER BY module_path DESC
			LIMIT 1
		) v
		ON
			p.module_path = v.module_path
			AND p.version = v.version;`
		args = append(args, version)
	} else {
		query += `
		INNER JOIN (
			SELECT *
			FROM versions
			WHERE version = $2
			AND module_path = $3
		) v
		ON
			p.module_path = v.module_path
			AND p.version = v.version;`
		args = append(args, version, modulePath)
	}

	var (
		packages []*internal.Package
		vi       internal.VersionInfo
	)
	collect := func(rows *sql.Rows) error {
		var (
			pkg                        internal.Package
			licenseTypes, licensePaths []string
		)
		if err := rows.Scan(
			&pkg.Path,
			&pkg.Name,
			&pkg.Synopsis,
			&pkg.V1Path,
			&pkg.DocumentationHTML,
			pq.Array(&licenseTypes),
			pq.Array(&licensePaths),
			&pkg.GOOS,
			&pkg.GOARCH,
			&vi.Version,
			&vi.ModulePath,
			nullIsEmpty(&vi.ReadmeFilePath),
			&vi.ReadmeContents,
			&vi.CommitTime,
			&vi.VersionType,
			sourceInfoScanner{&vi.SourceInfo}); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return err
		}
		pkg.Licenses = lics
		packages = append(packages, &pkg)
		return nil
	}
	if err := db.runQuery(ctx, query, collect, args...); err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, xerrors.Errorf("packages in directory not found: %w", derrors.NotFound)
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Path < packages[j].Path
	})
	return &internal.Directory{
		Path:        dirPath,
		VersionInfo: vi,
		Packages:    packages,
	}, nil
}
