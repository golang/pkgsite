// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"github.com/google/safehtml"
	"github.com/google/safehtml/uncheckedconversions"
	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
)

// LegacyGetPackagesInModule returns packages contained in the module version
// specified by modulePath and version. The returned packages will be sorted
// by their package path.
func (db *DB) LegacyGetPackagesInModule(ctx context.Context, modulePath, version string) (_ []*internal.LegacyPackage, err error) {
	query := `SELECT
		path,
		name,
		synopsis,
		v1_path,
		license_types,
		license_paths,
		redistributable,
		documentation,
		goos,
		goarch
	FROM
		packages
	WHERE
		module_path = $1
		AND version = $2
	ORDER BY path;`

	var packages []*internal.LegacyPackage
	collect := func(rows *sql.Rows) error {
		var (
			p                          internal.LegacyPackage
			licenseTypes, licensePaths []string
			docHTML                    string
		)
		if err := rows.Scan(&p.Path, &p.Name, &p.Synopsis, &p.V1Path, pq.Array(&licenseTypes),
			pq.Array(&licensePaths), &p.IsRedistributable, database.NullIsEmpty(&docHTML),
			&p.GOOS, &p.GOARCH); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return err
		}
		p.Licenses = lics
		p.DocumentationHTML = convertDocumentation(docHTML)
		packages = append(packages, &p)
		return nil
	}

	if err := db.db.RunQuery(ctx, query, collect, modulePath, version); err != nil {
		return nil, fmt.Errorf("DB.LegacyGetPackagesInModule(ctx, %q, %q): %w", modulePath, version, err)
	}
	return packages, nil
}

// GetImports fetches and returns all of the imports for the package with
// pkgPath, modulePath and version.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) GetImports(ctx context.Context, pkgPath, modulePath, version string) (paths []string, err error) {
	defer derrors.Wrap(&err, "DB.GetImports(ctx, %q, %q, %q)", pkgPath, modulePath, version)

	if pkgPath == "" || version == "" || modulePath == "" {
		return nil, fmt.Errorf("pkgPath, modulePath and version must all be non-empty: %w", derrors.InvalidArgument)
	}

	var query string
	if experiment.IsActive(ctx, internal.ExperimentUsePackageImports) {
		query = `
		SELECT to_path
		FROM package_imports i
		INNER JOIN paths p
		ON p.id = i.path_id
		INNER JOIN modules m
		ON m.id = p.module_id
		WHERE
			p.path = $1
			AND m.version = $2
			AND m.module_path = $3
		ORDER BY
			to_path;`
	} else {
		query = `
		SELECT to_path
		FROM imports
		WHERE
			from_path = $1
			AND from_version = $2
			AND from_module_path = $3
		ORDER BY
			to_path;`
	}

	var (
		toPath  string
		imports []string
	)
	collect := func(rows *sql.Rows) error {
		if err := rows.Scan(&toPath); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		imports = append(imports, toPath)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, pkgPath, version, modulePath); err != nil {
		return nil, err
	}
	return imports, nil
}

// GetImportedBy fetches and returns all of the packages that import the
// package with path.
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
//
// Instead of supporting pagination, this query runs with a limit.
func (db *DB) GetImportedBy(ctx context.Context, pkgPath, modulePath string, limit int) (paths []string, err error) {
	defer derrors.Wrap(&err, "GetImportedBy(ctx, %q, %q)", pkgPath, modulePath)
	if pkgPath == "" {
		return nil, fmt.Errorf("pkgPath cannot be empty: %w", derrors.InvalidArgument)
	}
	query := `
		SELECT
			DISTINCT from_path
		FROM
			imports_unique
		WHERE
			to_path = $1
		AND
			from_module_path <> $2
		ORDER BY
			from_path
		LIMIT $3`

	var importedby []string
	collect := func(rows *sql.Rows) error {
		var fromPath string
		if err := rows.Scan(&fromPath); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		importedby = append(importedby, fromPath)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, pkgPath, modulePath, limit); err != nil {
		return nil, err
	}
	return importedby, nil
}

// GetModuleInfo fetches a module version from the database with the primary key
// (module_path, version).
func (db *DB) GetModuleInfo(ctx context.Context, modulePath, version string) (_ *internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetModuleInfo(ctx, %q, %q)", modulePath, version)

	query := `
		SELECT
			module_path,
			version,
			commit_time,
			version_type,
			source_info,
			redistributable,
			has_go_mod
		FROM
			modules
		WHERE
			module_path = $1
			AND version = $2;`

	var mi internal.ModuleInfo
	row := db.db.QueryRow(ctx, query, modulePath, version)
	if err := row.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime, &mi.VersionType,
		jsonbScanner{&mi.SourceInfo}, &mi.IsRedistributable, &mi.HasGoMod); err != nil {
		if err == sql.ErrNoRows {
			return nil, derrors.NotFound
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	return &mi, nil
}

// LegacyGetModuleInfo fetches a module version from the database with the primary key
// (module_path, version).
func (db *DB) LegacyGetModuleInfo(ctx context.Context, modulePath string, version string) (_ *internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetModuleInfo(ctx, %q, %q)", modulePath, version)

	query := `
		SELECT
			module_path,
			version,
			commit_time,
			readme_file_path,
			readme_contents,
			version_type,
			source_info,
			redistributable,
			has_go_mod
		FROM
			modules`

	args := []interface{}{modulePath}
	if version == internal.LatestVersion {
		query += `
			WHERE module_path = $1
			ORDER BY
				-- Order the versions by release then prerelease.
				-- The default version should be the first release
				-- version available, if one exists.
				version_type = 'release' DESC,
				sort_version DESC
			LIMIT 1;`
	} else {
		query += `
			WHERE module_path = $1 AND version = $2;`
		args = append(args, version)
	}

	var mi internal.LegacyModuleInfo
	row := db.db.QueryRow(ctx, query, args...)
	if err := row.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime,
		database.NullIsEmpty(&mi.LegacyReadmeFilePath), database.NullIsEmpty(&mi.LegacyReadmeContents), &mi.VersionType,
		jsonbScanner{&mi.SourceInfo}, &mi.IsRedistributable, &mi.HasGoMod); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("module version %s@%s: %w", modulePath, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	return &mi, nil
}

// jsonbScanner scans a jsonb value into a Go value.
type jsonbScanner struct {
	ptr interface{} // a pointer to a Go struct or other JSON-serializable value
}

func (s jsonbScanner) Scan(value interface{}) (err error) {
	defer derrors.Wrap(&err, "jsonbScanner(%+v)", value)

	vptr := reflect.ValueOf(s.ptr)
	if value == nil {
		// *s.ptr = nil
		vptr.Elem().Set(reflect.Zero(vptr.Elem().Type()))
		return nil
	}
	jsonBytes, ok := value.([]byte)
	if !ok {
		return errors.New("not a []byte")
	}
	// v := &[type of *s.ptr]
	v := reflect.New(vptr.Elem().Type())
	if err := json.Unmarshal(jsonBytes, v.Interface()); err != nil {
		return err
	}

	// *s.ptr = *v
	vptr.Elem().Set(v.Elem())
	return nil
}

// convertDocumentation takes a string that was read from the database and
// converts it to a safehtml.HTML.
func convertDocumentation(doc string) safehtml.HTML {
	if addDocQueryParam {
		doc = hackUpDocumentation(doc)
	}
	// We trust the data in our database and the transformation done by hackUpDocumentation.
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(doc)
}

// addDocQueryParam controls whether to use a regexp replacement to append
// ?tab=doc to urls linking to package identifiers within the documentation.
const addDocQueryParam = true

// packageLinkRegexp matches cross-package identifier links that have been
// generated by the dochtml package. At the time this hack was added, these
// links are all constructed to have either the form
//   <a href="/pkg/[path]">[name]</a>
// or the form
//   <a href="/pkg/[path]#identifier">[name]</a>
//
// The packageLinkRegexp mutates these links as follows:
//   - remove the now unnecessary '/pkg' path prefix
//   - add an explicit ?tab=doc after the path.
var packageLinkRegexp = regexp.MustCompile(`(<a href="/)pkg/([^?#"]+)((?:#[^"]*)?">.*?</a>)`)

// hackUpDocumentation rewrites anchor hrefs to add a tab=doc query parameter.
// It preserves the safety of its argument. That is, if docHTML is safe
// from XSS attacks, so is hackUpDocumentation(docHTML).
func hackUpDocumentation(docHTML string) string {
	return packageLinkRegexp.ReplaceAllString(docHTML, `$1$2?tab=doc$3`)
}
