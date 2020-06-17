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
	"strings"

	"github.com/lib/pq"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/version"
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
		)
		if err := rows.Scan(&p.Path, &p.Name, &p.Synopsis, &p.V1Path, pq.Array(&licenseTypes),
			pq.Array(&licensePaths), &p.IsRedistributable, database.NullIsEmpty(&p.DocumentationHTML),
			&p.GOOS, &p.GOARCH); err != nil {
			return fmt.Errorf("row.Scan(): %v", err)
		}
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return err
		}
		p.Licenses = lics
		packages = append(packages, &p)
		return nil
	}

	if err := db.db.RunQuery(ctx, query, collect, modulePath, version); err != nil {
		return nil, fmt.Errorf("DB.LegacyGetPackagesInModule(ctx, %q, %q): %w", modulePath, version, err)
	}
	return packages, nil
}

// GetTaggedVersionsForPackageSeries returns a list of tagged versions sorted in
// descending semver order. This list includes tagged versions of packages that
// have the same v1path.
func (db *DB) GetTaggedVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*internal.ModuleInfo, error) {
	return getPackageVersions(ctx, db, pkgPath, []version.Type{version.TypeRelease, version.TypePrerelease})
}

// GetPseudoVersionsForPackageSeries returns the 10 most recent from a list of
// pseudo-versions sorted in descending semver order. This list includes
// pseudo-versions of packages that have the same v1path.
func (db *DB) GetPseudoVersionsForPackageSeries(ctx context.Context, pkgPath string) ([]*internal.ModuleInfo, error) {
	return getPackageVersions(ctx, db, pkgPath, []version.Type{version.TypePseudo})
}

// getPackageVersions returns a list of versions sorted in descending semver
// order. The version types included in the list are specified by a list of
// VersionTypes.
func getPackageVersions(ctx context.Context, db *DB, pkgPath string, versionTypes []version.Type) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "DB.getPackageVersions(ctx, db, %q, %v)", pkgPath, versionTypes)

	baseQuery := `
		SELECT
			p.module_path,
			p.version,
			m.commit_time
		FROM
			packages p
		INNER JOIN
			modules m
		ON
			p.module_path = m.module_path
			AND p.version = m.version
		WHERE
			p.v1_path = (
				SELECT v1_path
				FROM packages
				WHERE path = $1
				LIMIT 1
			)
			AND version_type in (%s)
		ORDER BY
			m.sort_version DESC %s`
	queryEnd := `;`
	if len(versionTypes) == 0 {
		return nil, fmt.Errorf("error: must specify at least one version type")
	} else if len(versionTypes) == 1 && versionTypes[0] == version.TypePseudo {
		queryEnd = `LIMIT 10;`
	}
	query := fmt.Sprintf(baseQuery, versionTypeExpr(versionTypes), queryEnd)

	rows, err := db.db.Query(ctx, query, pkgPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versionHistory []*internal.ModuleInfo
	for rows.Next() {
		var mi internal.ModuleInfo
		if err := rows.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		versionHistory = append(versionHistory, &mi)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(): %v", err)
	}

	return versionHistory, nil
}

// versionTypeExpr returns a comma-separated list of version types,
// for use in a clause like "WHERE version_type IN (%s)"
func versionTypeExpr(vts []version.Type) string {
	var vs []string
	for _, vt := range vts {
		vs = append(vs, fmt.Sprintf("'%s'", vt.String()))
	}
	return strings.Join(vs, ", ")
}

// GetTaggedVersionsForModule returns a list of tagged versions sorted in
// descending semver order.
func (db *DB) GetTaggedVersionsForModule(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	return getModuleVersions(ctx, db, modulePath, []version.Type{version.TypeRelease, version.TypePrerelease})
}

// GetPseudoVersionsForModule returns the 10 most recent from a list of
// pseudo-versions sorted in descending semver order.
func (db *DB) GetPseudoVersionsForModule(ctx context.Context, modulePath string) ([]*internal.ModuleInfo, error) {
	return getModuleVersions(ctx, db, modulePath, []version.Type{version.TypePseudo})
}

// getModuleVersions returns a list of versions sorted in descending semver
// order. The version types included in the list are specified by a list of
// VersionTypes.
func getModuleVersions(ctx context.Context, db *DB, modulePath string, versionTypes []version.Type) (_ []*internal.ModuleInfo, err error) {
	defer derrors.Wrap(&err, "getModuleVersions(ctx, db, %q, %v)", modulePath, versionTypes)

	baseQuery := `
	SELECT
		module_path, version, commit_time
    FROM
		modules
	WHERE
		series_path = $1
	    AND version_type in (%s)
	ORDER BY
		sort_version DESC %s`

	queryEnd := `;`
	if len(versionTypes) == 0 {
		return nil, fmt.Errorf("error: must specify at least one version type")
	} else if len(versionTypes) == 1 && versionTypes[0] == version.TypePseudo {
		queryEnd = `LIMIT 10;`
	}
	query := fmt.Sprintf(baseQuery, versionTypeExpr(versionTypes), queryEnd)
	var vinfos []*internal.ModuleInfo
	collect := func(rows *sql.Rows) error {
		var mi internal.ModuleInfo
		if err := rows.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime); err != nil {
			return err
		}
		vinfos = append(vinfos, &mi)
		return nil
	}
	if err := db.db.RunQuery(ctx, query, collect, internal.SeriesPathForModule(modulePath)); err != nil {
		return nil, err
	}
	return vinfos, nil
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

	var toPath string
	query := `
		SELECT to_path
		FROM imports
		WHERE
			from_path = $1
			AND from_version = $2
			AND from_module_path = $3
		ORDER BY
			to_path;`

	var imports []string
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

// LegacyGetModuleInfo fetches a Version from the database with the primary key
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

	var (
		mi       internal.LegacyModuleInfo
		hasGoMod sql.NullBool
	)
	row := db.db.QueryRow(ctx, query, args...)
	if err := row.Scan(&mi.ModulePath, &mi.Version, &mi.CommitTime,
		database.NullIsEmpty(&mi.LegacyReadmeFilePath), database.NullIsEmpty(&mi.LegacyReadmeContents), &mi.VersionType,
		jsonbScanner{&mi.SourceInfo}, &mi.IsRedistributable, &hasGoMod); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("module version %s@%s: %w", modulePath, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	setHasGoMod(&mi.ModuleInfo, hasGoMod)
	return &mi, nil
}

func setHasGoMod(mi *internal.ModuleInfo, nb sql.NullBool) {
	// The safe default value for HasGoMod is true, because search will penalize modules that don't have one.
	// This is temporary: when has_go_mod is fully populated, we'll make it NOT NULL.
	mi.HasGoMod = true
	if nb.Valid {
		mi.HasGoMod = nb.Bool
	}
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
