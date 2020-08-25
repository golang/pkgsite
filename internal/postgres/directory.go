// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/stdlib"
)

// GetPackagesInDirectory returns all of the packages in a directory from a
// module version, including the package that lives at dirPath, if present.
func (db *DB) GetPackagesInDirectory(ctx context.Context, dirPath, modulePath, resolvedVersion string) (_ []*internal.PackageMeta, err error) {
	defer derrors.Wrap(&err, "DB.GetPackagesInDirectory(ctx, %q, %q, %q)", dirPath, modulePath, resolvedVersion)

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
		return nil, fmt.Errorf("directory does not contain any packages: %w", derrors.NotFound)
	}
	if !db.bypassLicenseCheck {
		for _, p := range packages {
			p.RemoveNonRedistributableData()
		}
	}
	return packages, nil
}

// GetDirectory returns a directory from the database, along with all of the
// data associated with that directory, including the package, imports, readme,
// documentation, and licenses.
func (db *DB) GetDirectory(ctx context.Context, path, modulePath, version string) (_ *internal.Directory, err error) {
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
			p.license_paths,
			d.goos,
			d.goarch,
			d.synopsis,
			d.html
		FROM modules m
		INNER JOIN paths p
		ON p.module_id = m.id
		LEFT JOIN documentation d
		ON d.path_id = p.id
		WHERE
			p.path = $1
			AND m.module_path = $2
			AND m.version = $3;`
	var (
		mi                         internal.ModuleInfo
		dir                        internal.Directory
		doc                        internal.Documentation
		docHTML                    string
		pkg                        internal.Package
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
		database.NullIsEmpty(&pkg.Name),
		&dir.V1Path,
		&dir.IsRedistributable,
		pq.Array(&licenseTypes),
		pq.Array(&licensePaths),
		database.NullIsEmpty(&doc.GOOS),
		database.NullIsEmpty(&doc.GOARCH),
		database.NullIsEmpty(&doc.Synopsis),
		database.NullIsEmpty(&docHTML),
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("directory %s@%s: %w", path, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}
	dir.Licenses = lics
	if pkg.Name != "" {
		dir.Name = pkg.Name
		dir.Package = &pkg
		pkg.Path = dir.Path
		doc.HTML = convertDocumentation(docHTML)
		pkg.Documentation = &doc
		collect := func(rows *sql.Rows) error {
			var path string
			if err := rows.Scan(&path); err != nil {
				return fmt.Errorf("row.Scan(): %v", err)
			}
			pkg.Imports = append(pkg.Imports, path)
			return nil
		}
		if err := db.db.RunQuery(ctx, `
		SELECT to_path
		FROM package_imports
		WHERE path_id = $1`, collect, dir.PathID); err != nil {
			return nil, err
		}
	}

	// TODO(golang/go#38513): remove and query the readmes table directly once
	// we start displaying READMEs for directories instead of the top-level
	// module.
	var readme internal.Readme
	row = db.db.QueryRow(ctx, `
		SELECT file_path, contents
		FROM modules m
		INNER JOIN paths p
		ON p.module_id = m.id
		INNER JOIN readmes r
		ON p.id = r.path_id
		WHERE
		    module_path=$1
			AND m.version=$2
			AND m.module_path=p.path`, modulePath, version)
	if err := row.Scan(&readme.Filepath, &readme.Contents); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if readme.Filepath != "" {
		dir.Readme = &readme
	}
	dir.ModuleInfo = mi
	if !db.bypassLicenseCheck {
		dir.RemoveNonRedistributableData()
	}
	return &dir, nil
}

// LegacyGetDirectory returns the directory corresponding to the provided dirPath,
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
func (db *DB) LegacyGetDirectory(ctx context.Context, dirPath, modulePath, version string, fields internal.FieldSet) (_ *internal.LegacyDirectory, err error) {
	defer derrors.Wrap(&err, "DB.LegacyGetDirectory(ctx, %q, %q, %q)", dirPath, modulePath, version)

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
		packages []*internal.LegacyPackage
		mi       = internal.LegacyModuleInfo{LegacyReadmeContents: internal.StringFieldMissing}
	)
	collect := func(rows *sql.Rows) error {
		var (
			pkg          internal.LegacyPackage
			licenseTypes []string
			licensePaths []string
			docHTML      string = internal.StringFieldMissing
		)
		scanArgs := []interface{}{
			&pkg.Path,
			&pkg.Name,
			&pkg.Synopsis,
			&pkg.V1Path,
		}
		if fields&internal.WithDocumentationHTML != 0 {
			scanArgs = append(scanArgs, database.NullIsEmpty(&docHTML))
		}
		scanArgs = append(scanArgs,
			pq.Array(&licenseTypes),
			pq.Array(&licensePaths),
			&pkg.IsRedistributable,
			&pkg.GOOS,
			&pkg.GOARCH,
			&mi.Version,
			&mi.ModulePath,
			database.NullIsEmpty(&mi.LegacyReadmeFilePath))
		if fields&internal.WithReadmeContents != 0 {
			scanArgs = append(scanArgs, database.NullIsEmpty(&mi.LegacyReadmeContents))
		}
		scanArgs = append(scanArgs,
			&mi.CommitTime,
			&mi.VersionType,
			jsonbScanner{&mi.SourceInfo},
			&mi.IsRedistributable,
			&mi.HasGoMod)
		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return err
		}
		pkg.Licenses = lics
		pkg.DocumentationHTML = convertDocumentation(docHTML)
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
	ld := &internal.LegacyDirectory{
		Path:             dirPath,
		LegacyModuleInfo: mi,
		Packages:         packages,
	}
	if !db.bypassLicenseCheck {
		ld.RemoveNonRedistributableData()
	}
	return ld, nil
}

func directoryColumns(fields internal.FieldSet) string {
	var doc, readme string
	if fields&internal.WithDocumentationHTML != 0 {
		doc = "p.documentation,"
	}
	if fields&internal.WithReadmeContents != 0 {
		readme = "m.readme_contents,"
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
			m.readme_file_path,
			` + readme + `
			m.commit_time,
			m.version_type,
			m.source_info,
			m.redistributable,
			m.has_go_mod`
}

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
					modules m
				WHERE
					(m.module_path, m.version) IN (
						SELECT module_path, version
						FROM %s
						WHERE tsv_parent_directories @@ $1::tsquery
						GROUP BY 1, 2
					)
				%s
				LIMIT 1
			) m
			ON
				p.module_path = m.module_path
				AND p.version = m.version
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
			SELECT m.*
			FROM modules m
			INNER JOIN potential_packages p
			ON
				p.module_path = m.module_path
				AND p.version = m.version
			WHERE
				p.version = $2
			ORDER BY
				module_path DESC
			LIMIT 1
		)
		SELECT %s
		FROM potential_packages p
		INNER JOIN module_version m
		ON
			p.module_path = m.module_path
			AND p.version = m.version;`, directoryColumns(fields)), []interface{}{dirPath, version}
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
				FROM modules m
				WHERE
					m.module_path = $2
					AND m.version IN (
						SELECT version
						FROM packages
						WHERE
							tsv_parent_directories @@ $1::tsquery
							AND module_path=$2
					)
				%s
				LIMIT 1
			) m
			ON
				p.module_path = m.module_path
				AND p.version = m.version
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
				modules m
			ON
				p.module_path = m.module_path
				AND p.version = m.version
			WHERE
				tsv_parent_directories @@ $1::tsquery
				AND p.module_path = $2
				AND p.version = $3;`, directoryColumns(fields)), []interface{}{dirPath, modulePath, version}
}
