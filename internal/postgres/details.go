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
)

// GetNestedModules returns the latest major version of all nested modules
// given a modulePath path prefix.
func (db *DB) GetNestedModules(ctx context.Context, modulePath string) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetNestedModules(ctx, %v)", modulePath)

	query := `
		SELECT DISTINCT ON (series_path)
			m.module_path,
			m.version,
			m.commit_time,
			m.redistributable,
			m.has_go_mod,
			m.source_info
		FROM
			modules m
		WHERE
			m.module_path LIKE $1 || '/%'
		ORDER BY
			m.series_path,
			m.incompatible,
			m.version_type = 'release' DESC,
			m.sort_version DESC;
	`

	var modules []*internal.ModuleInfo
	collect := func(rows *sql.Rows) error {
		mi, err := scanModuleInfo(rows.Scan)
		if err != nil {
			return fmt.Errorf("rows.Scan(): %v", err)
		}
		isExcluded, err := db.IsExcluded(ctx, mi.ModulePath)
		if err != nil {
			return err
		}
		if !isExcluded {
			modules = append(modules, mi)
		}
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, modulePath); err != nil {
		return nil, err
	}

	return modules, nil
}

// LegacyGetPackagesInModule returns packages contained in the module version
// specified by modulePath and version. The returned packages will be sorted
// by their package path.
func (db *DB) LegacyGetPackagesInModule(ctx context.Context, modulePath, resolvedVersion string) (_ []*internal.LegacyPackage, err error) {
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

	if err := db.db.RunQuery(ctx, query, collect, modulePath, resolvedVersion); err != nil {
		return nil, fmt.Errorf("DB.LegacyGetPackagesInModule(ctx, %q, %q): %w", modulePath, resolvedVersion, err)
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

// LegacyGetImports fetches and returns all of the imports for the package with
// pkgPath, modulePath and version.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) LegacyGetImports(ctx context.Context, pkgPath, modulePath, resolvedVersion string) (paths []string, err error) {
	defer derrors.Wrap(&err, "DB.LegacyGetImports(ctx, %q, %q, %q)", pkgPath, modulePath, resolvedVersion)

	if pkgPath == "" || resolvedVersion == "" || modulePath == "" {
		return nil, fmt.Errorf("pkgPath, modulePath and version must all be non-empty: %w", derrors.InvalidArgument)
	}

	query := `
		SELECT to_path
		FROM imports
		WHERE
			from_path = $1
			AND from_version = $2
			AND from_module_path = $3
		ORDER BY
			to_path;`
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
	if err := db.db.RunQuery(ctx, query, collect, pkgPath, resolvedVersion, modulePath); err != nil {
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
func (db *DB) GetModuleInfo(ctx context.Context, modulePath, resolvedVersion string) (_ *internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "GetModuleInfo(ctx, %q, %q)", modulePath, resolvedVersion)

	query := `
		SELECT
			module_path,
			version,
			commit_time,
			redistributable,
			has_go_mod,
			source_info
		FROM
			modules
		WHERE
			module_path = $1
			AND version = $2;`

	row := db.db.QueryRow(ctx, query, modulePath, resolvedVersion)
	mi, err := scanModuleInfo(row.Scan)
	if err == sql.ErrNoRows {
		return nil, derrors.NotFound
	}
	if err != nil {
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	return mi, nil
}

// LegacyGetModuleInfo fetches a module version from the database with the primary key
// (module_path, version).
func (db *DB) LegacyGetModuleInfo(ctx context.Context, modulePath string, requestedVersion string) (_ *internal.LegacyModuleInfo, err error) {
	defer derrors.Wrap(&err, "LegacyGetModuleInfo(ctx, %q, %q)", modulePath, requestedVersion)

	query := `
		SELECT
			module_path,
			version,
			commit_time,
			readme_file_path,
			readme_contents,
			source_info,
			redistributable,
			has_go_mod
		FROM
			modules m`

	args := []interface{}{modulePath}
	if requestedVersion == internal.LatestVersion {
		query += fmt.Sprintf(`
			WHERE m.module_path = $1 %s LIMIT 1;`, orderByLatest)
	} else {
		query += `
			WHERE m.module_path = $1 AND m.version = $2;`
		args = append(args, requestedVersion)
	}

	var mi internal.LegacyModuleInfo
	row := db.db.QueryRow(ctx, query, args...)
	if err := row.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime,
		database.NullIsEmpty(&mi.LegacyReadmeFilePath), database.NullIsEmpty(&mi.LegacyReadmeContents),
		jsonbScanner{&mi.SourceInfo}, &mi.IsRedistributable, &mi.HasGoMod); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("module version %s@%s: %w", modulePath, requestedVersion, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	if db.bypassLicenseCheck {
		mi.IsRedistributable = true
	} else {
		mi.RemoveNonRedistributableData()
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

// scanModuleInfo constructs an *internal.ModuleInfo from the given scanner.
func scanModuleInfo(scan func(dest ...interface{}) error) (*internal.ModuleInfo, error) {
	var mi internal.ModuleInfo
	if err := scan(&mi.ModulePath, &mi.Version, &mi.CommitTime,
		&mi.IsRedistributable, &mi.HasGoMod, jsonbScanner{&mi.SourceInfo}); err != nil {
		return nil, err
	}
	return &mi, nil
}

// convertDocumentation takes a string that was read from the database and
// converts it to a safehtml.HTML.
//
// It rewrites documentation links by stripping the /pkg path prefix. It
// preserves the safety of its argument. That is, if docHTML is safe
// from XSS attacks, so is replaceDocumentationLinks(docHTML).
func convertDocumentation(docHTML string) safehtml.HTML {
	doc := removePkgPrefix(docHTML)
	// We trust the data in our database and the transformation done by removePkgPrefix.
	return uncheckedconversions.HTMLFromStringKnownToSatisfyTypeContract(doc)
}

// packageLinkRegexp matches cross-package identifier links that have been
// generated by the dochtml package. At the time this hack was added, these
// links are all constructed to have either the form
//   <a href="/pkg/[path]">[name]</a>
// or the form
//   <a href="/pkg/[path]#identifier">[name]</a>
//
// The packageLinkRegexp mutates these links as follows:
//   - remove the now unnecessary '/pkg' path prefix
var packageLinkRegexp = regexp.MustCompile(`(<a href="/)pkg/([^?#"]+)((?:#[^"]*)?">.*?</a>)`)

// removePkgPrefix removes the /pkg path prefix from links in docHTML.
// See documentation for packageLinkRegexp for explanation and
// TestRemovePkgPrefix for examples. It preserves the safety of its argument.
// That is, if docHTML is safe from XSS attacks, so is
// removePkgPrefix(docHTML).
func removePkgPrefix(docHTML string) string {
	return packageLinkRegexp.ReplaceAllString(docHTML, `$1$2$3`)
}
