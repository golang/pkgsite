// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/xerrors"
)

// GetPackage returns the first package from the database that has path and
// version.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it was caused by an invalid path or version.
func (db *DB) GetPackage(ctx context.Context, path string, version string) (_ *internal.VersionedPackage, err error) {
	defer derrors.Wrap(&err, "DB.GetPackage(ctx, %q, %q)", path, version)
	// TODO(b/140558033): fold the logic in getLatestPackage in here.
	if version == internal.LatestVersion {
		return db.getLatestPackage(ctx, path)
	}

	if path == "" || version == "" {
		return nil, xerrors.Errorf("neither path nor version can be empty: %w", derrors.InvalidArgument)
	}

	var (
		name, synopsis, modulePath, v1path, readmeFilePath, versionType string
		repositoryURL, vcsType, homepageURL                             sql.NullString
		commitTime                                                      time.Time
		readmeContents, documentation                                   []byte
		licenseTypes, licensePaths                                      []string
	)
	query := `
		SELECT
			v.commit_time,
			p.license_types,
			p.license_paths,
			v.readme_file_path,
			v.readme_contents,
			v.module_path,
			p.name,
			p.synopsis,
			p.v1_path,
			v.version_type,
			p.documentation,
			v.repository_url,
			v.vcs_type,
			v.homepage_url
		FROM
			versions v
		INNER JOIN
			packages p
		ON
			p.module_path = v.module_path
			AND v.version = p.version
		WHERE
			p.path = $1
			AND p.version = $2
		LIMIT 1;`

	row := db.queryRow(ctx, query, path, version)
	if err := row.Scan(&commitTime, pq.Array(&licenseTypes),
		pq.Array(&licensePaths), &readmeFilePath, &readmeContents, &modulePath,
		&name, &synopsis, &v1path, &versionType, &documentation, &repositoryURL, &vcsType, &homepageURL); err != nil {
		if err == sql.ErrNoRows {
			return nil, xerrors.Errorf("package %s@%s: %w", path, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}

	return &internal.VersionedPackage{
		Package: internal.Package{
			Name:              name,
			Path:              path,
			Synopsis:          synopsis,
			Licenses:          lics,
			V1Path:            v1path,
			DocumentationHTML: documentation,
		},
		VersionInfo: internal.VersionInfo{
			ModulePath:     modulePath,
			Version:        version,
			CommitTime:     commitTime,
			ReadmeFilePath: readmeFilePath,
			ReadmeContents: readmeContents,
			VersionType:    internal.VersionType(versionType),
			VCSType:        vcsType.String,
			RepositoryURL:  repositoryURL.String,
			HomepageURL:    homepageURL.String,
		},
	}, nil
}

// getLatestPackage returns the package from the database with the latest version.
// If multiple packages share the same path then the package that the database
// chooses is returned.
func (db *DB) getLatestPackage(ctx context.Context, path string) (_ *internal.VersionedPackage, err error) {
	defer derrors.Wrap(&err, "DB.GetLatestPackage(ctx, %q)", path)

	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	var (
		modulePath, name, synopsis, version, v1path, readmeFilePath, versionType string
		repositoryURL, vcsType, homepageURL                                      sql.NullString
		commitTime                                                               time.Time
		licenseTypes, licensePaths                                               []string
		readmeContents, documentation                                            []byte
	)
	query := `
		SELECT
			p.module_path,
			p.license_types,
			p.license_paths,
			v.version,
			v.commit_time,
			p.name,
			p.synopsis,
			p.v1_path,
			v.readme_file_path,
			v.readme_contents,
			p.documentation,
			v.repository_url,
			v.vcs_type,
			v.homepage_url,
			v.version_type
		FROM
			versions v
		INNER JOIN
			packages p
		ON
			p.module_path = v.module_path
			AND v.version = p.version
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

	row := db.queryRow(ctx, query, path)
	if err := row.Scan(&modulePath, pq.Array(&licenseTypes), pq.Array(&licensePaths), &version, &commitTime, &name, &synopsis, &v1path, &readmeFilePath, &readmeContents, &documentation, &repositoryURL, &vcsType, &homepageURL, &versionType); err != nil {
		if err == sql.ErrNoRows {
			return nil, derrors.NotFound
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, err
	}

	return &internal.VersionedPackage{
		Package: internal.Package{
			Name:              name,
			Path:              path,
			Synopsis:          synopsis,
			Licenses:          lics,
			V1Path:            v1path,
			DocumentationHTML: documentation,
		},
		VersionInfo: internal.VersionInfo{
			ModulePath:     modulePath,
			Version:        version,
			CommitTime:     commitTime,
			ReadmeFilePath: readmeFilePath,
			ReadmeContents: readmeContents,
			VCSType:        vcsType.String,
			RepositoryURL:  repositoryURL.String,
			HomepageURL:    homepageURL.String,
			VersionType:    internal.VersionType(versionType),
		},
	}, nil
}

// GetPackagesInVersion returns packages contained in the module version
// specified by modulePath and version. The returned packages will be sorted
// their package path.
func (db *DB) GetPackagesInVersion(ctx context.Context, modulePath, version string) (_ []*internal.Package, err error) {
	query := `SELECT
		path,
		name,
		synopsis,
		v1_path,
		license_types,
		license_paths,
		documentation
	FROM
		packages
	WHERE
		module_path = $1
		AND version = $2
	ORDER BY path;`

	var packages []*internal.Package
	collect := func(rows *sql.Rows) error {
		var (
			p                          internal.Package
			licenseTypes, licensePaths []string
		)
		if err := rows.Scan(&p.Path, &p.Name, &p.Synopsis, &p.V1Path, pq.Array(&licenseTypes),
			pq.Array(&licensePaths), &p.DocumentationHTML); err != nil {
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

	if err := db.runQuery(ctx, query, collect, modulePath, version); err != nil {
		return nil, xerrors.Errorf("DB.GetPackagesInVersion(ctx, %q, %q): %w", err)
	}
	return packages, nil
}

// GetTaggedVersionsForPackageSeries returns a list of tagged versions sorted
// in descending order by major, minor and patch number and then lexicographically
// in descending order by prerelease. This list includes tagged versions of
// packages that have the same v1path.
func (db *DB) GetTaggedVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error) {
	return getPackageVersions(ctx, db, path, []internal.VersionType{internal.VersionTypeRelease, internal.VersionTypePrerelease})
}

// GetPseudoVersionsForPackageSeries returns the 10 most recent from a list of
// pseudo-versions sorted in descending order by major, minor and patch number
// and then lexicographically in descending order by prerelease. This list includes
// pseudo-versions of packages that have the same v1path.
func (db *DB) GetPseudoVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error) {
	return getPackageVersions(ctx, db, path, []internal.VersionType{internal.VersionTypePseudo})
}

// getPackageVersions returns a list of versions sorted numerically
// in descending order by major, minor and patch number and then
// lexicographically in descending order by prerelease. The version types
// included in the list are specified by a list of VersionTypes. The results
// include the type of versions of packages that are part of the same series
// and have the same package v1path.
func getPackageVersions(ctx context.Context, db *DB, path string, versionTypes []internal.VersionType) (_ []*internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "DB.getPackageVersions(ctx, db, %q, %v)", path, versionTypes)

	baseQuery := `SELECT
			p.module_path,
			p.version,
			v.commit_time
		FROM
			packages p
		INNER JOIN
			versions v
		ON
			p.module_path = v.module_path
			AND p.version = v.version
		WHERE
			p.v1_path IN (
				SELECT DISTINCT v1_path
				FROM packages
				WHERE path=$1
			)
			AND version_type in (%s)
		ORDER BY
			v.module_path DESC,
			v.major DESC,
			v.minor DESC,
			v.patch DESC,
			v.prerelease DESC %s`
	queryEnd := `;`
	if len(versionTypes) == 0 {
		return nil, fmt.Errorf("error: must specify at least one version type")
	} else if len(versionTypes) == 1 && versionTypes[0] == internal.VersionTypePseudo {
		queryEnd = `LIMIT 10;`
	}
	query := fmt.Sprintf(baseQuery, versionTypeExpr(versionTypes), queryEnd)

	rows, err := db.query(ctx, query, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versionHistory []*internal.VersionInfo
	for rows.Next() {
		var vi internal.VersionInfo
		if err := rows.Scan(&vi.ModulePath, &vi.Version, &vi.CommitTime); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		versionHistory = append(versionHistory, &vi)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(): %v", err)
	}

	return versionHistory, nil
}

// versionTypeExpr returns a comma-separated list of version types,
// for use in a clause like "WHERE version_type IN (%s)"
func versionTypeExpr(vts []internal.VersionType) string {
	var vs []string
	for _, vt := range vts {
		vs = append(vs, fmt.Sprintf("'%s'", vt.String()))
	}
	return strings.Join(vs, ", ")
}

// GetTaggedVersionsForModule returns a list of tagged versions sorted
// in descending order by major, minor and patch number and then lexicographically
// in descending order by prerelease.
func (db *DB) GetTaggedVersionsForModule(ctx context.Context, modulePath string) ([]*internal.VersionInfo, error) {
	return getModuleVersions(ctx, db, modulePath, []internal.VersionType{internal.VersionTypeRelease, internal.VersionTypePrerelease})
}

// GetPseudoVersionsForModule returns the 10 most recent from a list of
// pseudo-versions sorted in descending order by major, minor and patch number
// and then lexicographically in descending order by prerelease.
func (db *DB) GetPseudoVersionsForModule(ctx context.Context, modulePath string) ([]*internal.VersionInfo, error) {
	return getModuleVersions(ctx, db, modulePath, []internal.VersionType{internal.VersionTypePseudo})
}

// getModuleVersions returns a list of versions sorted numerically
// in descending order by major, minor and patch number and then
// lexicographically in descending order by prerelease. The version types
// included in the list are specified by a list of VersionTypes.
func getModuleVersions(ctx context.Context, db *DB, modulePath string, versionTypes []internal.VersionType) (_ []*internal.VersionInfo, err error) {
	// TODO(b/139530312): get information for parent modules.
	defer derrors.Wrap(&err, "getModuleVersions(ctx, db, %q, %v)", modulePath, versionTypes)

	baseQuery := `
	SELECT
		module_path, version, commit_time
    FROM
		versions
	WHERE
		series_path = $1
	    AND version_type in (%s)
	ORDER BY
		major DESC,
		minor DESC,
		patch DESC,
		prerelease DESC %s`

	queryEnd := `;`
	if len(versionTypes) == 0 {
		return nil, fmt.Errorf("error: must specify at least one version type")
	} else if len(versionTypes) == 1 && versionTypes[0] == internal.VersionTypePseudo {
		queryEnd = `LIMIT 10;`
	}
	query := fmt.Sprintf(baseQuery, versionTypeExpr(versionTypes), queryEnd)
	var vinfos []*internal.VersionInfo
	collect := func(rows *sql.Rows) error {
		var vi internal.VersionInfo
		if err := rows.Scan(&vi.ModulePath, &vi.Version, &vi.CommitTime); err != nil {
			return err
		}
		vinfos = append(vinfos, &vi)
		return nil
	}
	if err := db.runQuery(ctx, query, collect, internal.SeriesPathForModule(modulePath)); err != nil {
		return nil, err
	}
	return vinfos, nil
}

// GetImports fetches and returns all of the imports for the package with path
// and version. If multiple packages have the same path and version, all of
// the imports will be returned.
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) GetImports(ctx context.Context, path, version string) (paths []string, err error) {
	defer derrors.Wrap(&err, "DB.GetImports(ctx, %q, %q)", path, version)

	if path == "" || version == "" {
		return nil, xerrors.Errorf("neither path nor version can be empty: %w", derrors.InvalidArgument)
	}

	var toPath string
	query := `
		SELECT
			to_path
		FROM
			imports
		WHERE
			from_path = $1
			AND from_version = $2
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
	if err := db.runQuery(ctx, query, collect, path, version); err != nil {
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
func (db *DB) GetImportedBy(ctx context.Context, path, modulePath string, limit int) (paths []string, err error) {
	defer derrors.Wrap(&err, "GetImportedBy(ctx, %q, %q)", path, modulePath)
	if path == "" {
		return nil, xerrors.Errorf("path cannot be empty: %w", derrors.InvalidArgument)
	}
	query := `
		SELECT
			DISTINCT from_path
		FROM
			imports
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
	if err := db.runQuery(ctx, query, collect, path, modulePath, limit); err != nil {
		return nil, err
	}
	return importedby, nil
}

// GetModuleLicenses returns all licenses associated with the given module path and
// version. These are the top-level licenses in the module zip file.
// It returns an InvalidArgument error if the module path or version is invalid.
func (db *DB) GetModuleLicenses(ctx context.Context, modulePath, version string) (_ []*license.License, err error) {
	defer derrors.Wrap(&err, "GetModuleLicenses(ctx, %q, %q)", modulePath, version)

	if modulePath == "" || version == "" {
		return nil, xerrors.Errorf("neither modulePath nor version can be empty: %w", derrors.InvalidArgument)
	}
	query := `
	SELECT
		types, file_path, contents
	FROM
		licenses
	WHERE
		module_path = $1 AND version = $2 AND position('/' in file_path) = 0
    `
	rows, err := db.query(ctx, query, modulePath, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectLicenses(rows)
}

// GetPackageLicenses returns all licenses associated with the given package path and
// version.
// It returns an InvalidArgument error if the module path or version is invalid.
func (db *DB) GetPackageLicenses(ctx context.Context, pkgPath, modulePath, version string) (_ []*license.License, err error) {
	defer derrors.Wrap(&err, "GetPackageLicenses(ctx, %q, %q, %q)", pkgPath, modulePath, version)

	if pkgPath == "" || version == "" {
		return nil, xerrors.Errorf("neither pkgPath nor version can be empty: %w", derrors.InvalidArgument)
	}
	query := `
		SELECT
			l.types,
			l.file_path,
			l.contents
		FROM
			licenses l
		INNER JOIN (
			SELECT DISTINCT ON (license_file_path)
				module_path,
				version,
				unnest(license_paths) AS license_file_path
			FROM
				packages
			WHERE
				path = $1
				AND module_path = $2
				AND version = $3
		) p
		ON
			p.module_path = l.module_path
			AND p.version = l.version
			AND p.license_file_path = l.file_path;`

	rows, err := db.query(ctx, query, pkgPath, modulePath, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectLicenses(rows)
}

// collectLicenses converts the sql rows to a list of licenses. The columns
// must be types, file_path and contents, in that order.
func collectLicenses(rows *sql.Rows) ([]*license.License, error) {
	mustHaveColumns(rows, "types", "file_path", "contents")
	var licenses []*license.License
	for rows.Next() {
		var (
			lic          = &license.License{Metadata: &license.Metadata{}}
			licenseTypes []string
		)
		if err := rows.Scan(pq.Array(&licenseTypes), &lic.FilePath, &lic.Contents); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		lic.Types = licenseTypes
		licenses = append(licenses, lic)
	}
	sort.Slice(licenses, func(i, j int) bool {
		return compareLicenses(licenses[i].Metadata, licenses[j].Metadata)
	})
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return licenses, nil
}

// mustHaveColumns panics if the columns of rows does not match wantColumns.
func mustHaveColumns(rows *sql.Rows, wantColumns ...string) {
	gotColumns, err := rows.Columns()
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(gotColumns, wantColumns) {
		panic(fmt.Sprintf("got columns %v, want $%v", gotColumns, wantColumns))
	}
}

// zipLicenseMetadata constructs license.Metadata from the given license types
// and paths, by zipping and then sorting.
func zipLicenseMetadata(licenseTypes []string, licensePaths []string) (_ []*license.Metadata, err error) {
	defer derrors.Wrap(&err, "zipLicenseMetadata(%v, %v)", licenseTypes, licensePaths)

	if len(licenseTypes) != len(licensePaths) {
		return nil, fmt.Errorf("BUG: got %d license types and %d license paths", len(licenseTypes), len(licensePaths))
	}
	byPath := make(map[string]*license.Metadata)
	var mds []*license.Metadata
	for i, p := range licensePaths {
		md, ok := byPath[p]
		if !ok {
			md = &license.Metadata{FilePath: p}
			mds = append(mds, md)
		}
		// By convention, we insert a license path with empty corresponding license
		// type if we are unable to detect *any* licenses in the file. This ensures
		// that we mark this package as non-redistributable.
		if licenseTypes[i] != "" {
			md.Types = append(md.Types, licenseTypes[i])
		}
	}
	sort.Slice(mds, func(i, j int) bool {
		return compareLicenses(mds[i], mds[j])
	})
	return mds, nil
}

// compareLicenses reports whether i < j according to our license sorting
// semantics.
func compareLicenses(i, j *license.Metadata) bool {
	if len(strings.Split(i.FilePath, "/")) > len(strings.Split(j.FilePath, "/")) {
		return true
	}
	return i.FilePath < j.FilePath
}

// GetVersionInfo fetches a Version from the database with the primary key
// (module_path, version).
func (db *DB) GetVersionInfo(ctx context.Context, modulePath string, version string) (_ *internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "GetVersionInfo(ctx, %q, %q)", modulePath, version)
	// TODO(b/140558033): fold the logic of getLatestVersionInfo in here.
	if version == internal.LatestVersion {
		return db.getLatestVersionInfo(ctx, modulePath)
	}

	var (
		repositoryURL, vcsType, homepageURL sql.NullString
		readmeFilePath, versionType         string
		commitTime                          time.Time
		readmeContents                      []byte
	)

	query := `
		SELECT
			v.commit_time,
			v.readme_file_path,
			v.readme_contents,
			v.version_type,
			v.repository_url,
			v.vcs_type,
			v.homepage_url
		FROM
			versions v
		WHERE module_path = $1 and version = $2;`
	row := db.queryRow(ctx, query, modulePath, version)
	if err := row.Scan(&commitTime, &readmeFilePath, &readmeContents, &versionType, &repositoryURL, &vcsType, &homepageURL); err != nil {
		if err == sql.ErrNoRows {
			return nil, xerrors.Errorf("module version %s@%s: %w", modulePath, version, derrors.NotFound)
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	return &internal.VersionInfo{
		ModulePath:     modulePath,
		Version:        version,
		CommitTime:     commitTime,
		ReadmeFilePath: readmeFilePath,
		ReadmeContents: readmeContents,
		VersionType:    internal.VersionType(versionType),
		VCSType:        vcsType.String,
		RepositoryURL:  repositoryURL.String,
		HomepageURL:    homepageURL.String,
	}, nil
}

// getLatestVersionInfo fetches a Version from the database with given
// modulePath at the latest version.
func (db *DB) getLatestVersionInfo(ctx context.Context, modulePath string) (_ *internal.VersionInfo, err error) {
	defer derrors.Wrap(&err, "GetLatestVersionInfo(ctx, %q)", modulePath)

	if modulePath == "" {
		return nil, errors.New("modulePath cannot be empty")
	}

	query := `
		SELECT
			commit_time,
			readme_file_path,
			readme_contents,
			version,
			version_type,
			repository_url,
			vcs_type,
			homepage_url,
			prerelease
		FROM
			versions
		WHERE module_path = $1
		ORDER BY
			-- Order the versions by release then prerelease.
			-- The default version should be the first release
			-- version available, if one exists.
			CASE WHEN prerelease = '~' THEN 0 ELSE 1 END,
			major DESC,
			minor DESC,
			patch DESC,
			prerelease DESC
		LIMIT 1;`
	row := db.queryRow(ctx, query, modulePath)
	var (
		vi                                  = &internal.VersionInfo{ModulePath: modulePath}
		repositoryURL, vcsType, homepageURL sql.NullString
	)

	var pr string
	err = row.Scan(&vi.CommitTime, &vi.ReadmeFilePath, &vi.ReadmeContents, &vi.Version, &vi.VersionType,
		&repositoryURL, &vcsType, &homepageURL, &pr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, derrors.NotFound
		}
		return nil, err
	}
	vi.VCSType = vcsType.String
	vi.RepositoryURL = repositoryURL.String
	vi.HomepageURL = homepageURL.String
	return vi, nil
}
