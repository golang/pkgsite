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
	"golang.org/x/discovery/internal/database"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/stdlib"
)

// GetDirectory returns the directory corresponding to the provided dirPath,
// modulePath, and version. The directory will contain all packages for that
// version, in sorted order by package path.
//
// If version = internal.LatestVersion, the directory corresponding to the
// latest matching module version will be fetched.
//
// fields is a set of fields to read (see internal.FieldSet and related
// definitions). If a field is not in fields, it will not be read from the DB
// and its value will be one of the XXXFieldMissing constants. Only certain
// large fields are treated specially in this way.
//
// If more than one module ties for a given dirPath and version pair, and
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
func (db *DB) GetDirectory(ctx context.Context, dirPath, modulePath, version string, fields internal.FieldSet) (_ *internal.Directory, err error) {
	defer derrors.Wrap(&err, "DB.GetDirectory(ctx, %q, %q, %q)", dirPath, modulePath, version)

	if dirPath == "" || modulePath == "" || version == "" {
		return nil, fmt.Errorf("none of pkgPath, modulePath, or version can be empty: %w", derrors.InvalidArgument)
	}

	var (
		query string
		args  []interface{}
	)
	if modulePath == internal.UnknownModulePath || modulePath == stdlib.ModulePath {
		query, args = directoryQueryWithoutModulePath(dirPath, version, fields)
	} else {
		query, args = directoryQueryWithModulePath(dirPath, modulePath, version, fields)
	}

	var (
		packages []*internal.Package
		vi       = internal.VersionInfo{ReadmeContents: internal.StringFieldMissing}
	)
	collect := func(rows *sql.Rows) error {
		var (
			pkg          = internal.Package{DocumentationHTML: internal.StringFieldMissing}
			licenseTypes []string
			licensePaths []string
		)
		scanArgs := []interface{}{
			&pkg.Path,
			&pkg.Name,
			&pkg.Synopsis,
			&pkg.V1Path,
		}
		if fields&internal.WithDocumentationHTML != 0 {
			scanArgs = append(scanArgs, database.NullIsEmpty(&pkg.DocumentationHTML))
		}
		scanArgs = append(scanArgs,
			pq.Array(&licenseTypes),
			pq.Array(&licensePaths),
			&pkg.IsRedistributable,
			&pkg.GOOS,
			&pkg.GOARCH,
			&vi.Version,
			&vi.ModulePath,
			database.NullIsEmpty(&vi.ReadmeFilePath))
		if fields&internal.WithReadmeContents != 0 {
			scanArgs = append(scanArgs, database.NullIsEmpty(&vi.ReadmeContents))
		}
		var hasGoMod sql.NullBool
		scanArgs = append(scanArgs,
			&vi.CommitTime,
			&vi.VersionType,
			jsonbScanner{&vi.SourceInfo},
			&vi.IsRedistributable,
			&hasGoMod)
		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		setHasGoMod(&vi, hasGoMod)
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return err
		}
		pkg.Licenses = lics
		packages = append(packages, &pkg)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, args...); err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("packages in directory not found: %w", derrors.NotFound)
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

func directoryColumns(fields internal.FieldSet) string {
	var doc, readme string
	if fields&internal.WithDocumentationHTML != 0 {
		doc = "p.documentation,"
	}
	if fields&internal.WithReadmeContents != 0 {
		readme = "v.readme_contents,"
	}
	return `
			p.path,
			p.name,
			p.synopsis,
			p.v1_path,
			` + doc + `
			p.license_types,
			p.license_paths,
			p.redistributable,
			p.goos,
			p.goarch,
			p.version,
			p.module_path,
			v.readme_file_path,
			` + readme + `
			v.commit_time,
			v.version_type,
			v.source_info,
			v.redistributable,
			v.has_go_mod`
}

const orderByLatest = `
			ORDER BY
				-- Order the versions by release then prerelease.
				-- The default version should be the first release
				-- version available, if one exists.
				version_type = 'release' DESC,
				sort_version DESC,
				module_path DESC`

// directoryQueryWithoutModulePath returns the query and args needed to fetch a
// directory when no module path is provided.
func directoryQueryWithoutModulePath(dirPath, version string, fields internal.FieldSet) (string, []interface{}) {
	if version == internal.LatestVersion {
		// internal packages are filtered out from the search_documents table.
		// However, for other packages, fetching from search_documents is
		// significantly faster than fetching from packages.
		var table string
		if !isInternalPackage(dirPath) {
			table = "search_documents"
		} else {
			table = "packages"
		}

		// Only dirPath is specified, so get the latest version of the
		// package found in any module that contains that directory.
		//
		// This might not necessarily be the latest module version that
		// matches the directory path. For example,
		// github.com/hashicorp/vault@v1.2.3 does not contain
		// github.com/hashicorp/vault/api, but
		// github.com/hashicorp/vault/api@v1.1.5 does.
		return fmt.Sprintf(`
			SELECT %s
			FROM
				packages p
			INNER JOIN (
				SELECT *
				FROM
					versions
				WHERE
					(module_path, version) IN (
						SELECT module_path, version
						FROM %s
						WHERE tsv_parent_directories @@ $1::tsquery
						GROUP BY 1, 2
					)
				%s
				LIMIT 1
			) v
			ON
				p.module_path = v.module_path
				AND p.version = v.version
			WHERE tsv_parent_directories @@ $1::tsquery;`,
			directoryColumns(fields), table, orderByLatest), []interface{}{dirPath}
	}

	// dirPath and version are specified, so get that directory version
	// from any module.  If it exists in multiple modules, return the one
	// with the longest path.
	return fmt.Sprintf(`
		WITH potential_packages AS (
			SELECT *
			FROM packages
			WHERE tsv_parent_directories @@ $1::tsquery
		),
		module_version AS (
			SELECT v.*
			FROM versions v
			INNER JOIN potential_packages p
			ON
				p.module_path = v.module_path
				AND p.version = v.version
			WHERE
				p.version = $2
			ORDER BY
				module_path DESC
			LIMIT 1
		)
		SELECT %s
		FROM potential_packages p
		INNER JOIN module_version v
		ON
			p.module_path = v.module_path
			AND p.version = v.version;`, directoryColumns(fields)), []interface{}{dirPath, version}
}

// directoryQueryWithoutModulePath returns the query and args needed to fetch a
// directory when a module path is provided.
func directoryQueryWithModulePath(dirPath, modulePath, version string, fields internal.FieldSet) (string, []interface{}) {
	if version == internal.LatestVersion {
		// dirPath and modulePath are specified, so get the latest version of
		// the package in the specified module.
		return fmt.Sprintf(`
			SELECT %s
			FROM packages p
			INNER JOIN (
				SELECT *
				FROM versions
				WHERE
					module_path = $2
					AND version IN (
						SELECT version
						FROM packages
						WHERE
							tsv_parent_directories @@ $1::tsquery
							AND module_path=$2
					)
				%s
				LIMIT 1
			) v
			ON
				p.module_path = v.module_path
				AND p.version = v.version
			WHERE
				p.module_path = $2
				AND tsv_parent_directories @@ $1::tsquery;`,
			directoryColumns(fields), orderByLatest), []interface{}{dirPath, modulePath}
	}

	// dirPath, modulePath and version were all specified. Only one
	// directory should ever match this query.
	return fmt.Sprintf(`
			SELECT %s
			FROM
				packages p
			INNER JOIN
				versions v
			ON
				p.module_path = v.module_path
				AND p.version = v.version
			WHERE
				tsv_parent_directories @@ $1::tsquery
				AND p.module_path = $2
				AND p.version = $3;`, directoryColumns(fields)), []interface{}{dirPath, modulePath, version}
}
