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
	"time"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/license"
)

// GetPackage returns the first package from the database that has path and
// version.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it was caused by an invalid path or version.
func (db *DB) GetPackage(ctx context.Context, path string, version string) (*internal.VersionedPackage, error) {
	if path == "" || version == "" {
		return nil, derrors.InvalidArgument("path and version cannot be empty")
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

	row := db.QueryRowContext(ctx, query, path, version)
	if err := row.Scan(&commitTime, pq.Array(&licenseTypes),
		pq.Array(&licensePaths), &readmeFilePath, &readmeContents, &modulePath,
		&name, &synopsis, &v1path, &versionType, &documentation, &repositoryURL, &vcsType, &homepageURL); err != nil {
		if err == sql.ErrNoRows {
			return nil, derrors.NotFound(fmt.Sprintf("package %s@%s not found", path, version))
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, fmt.Errorf("zipLicenseMetadata(%v, %v): %v", licenseTypes, licensePaths, err)
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

// GetLatestPackage returns the package from the database with the latest version.
// If multiple packages share the same path then the package that the database
// chooses is returned.
func (db *DB) GetLatestPackage(ctx context.Context, path string) (*internal.VersionedPackage, error) {
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
			v.module_path,
			v.major DESC,
			v.minor DESC,
			v.patch DESC,
			v.prerelease DESC
		LIMIT 1;`

	row := db.QueryRowContext(ctx, query, path)
	if err := row.Scan(&modulePath, pq.Array(&licenseTypes), pq.Array(&licensePaths), &version, &commitTime, &name, &synopsis, &v1path, &readmeFilePath, &readmeContents, &documentation, &repositoryURL, &vcsType, &homepageURL, &versionType); err != nil {
		if err == sql.ErrNoRows {
			return nil, derrors.NotFound(fmt.Sprintf("package %s@%s not found", path, version))
		}
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
	if err != nil {
		return nil, fmt.Errorf("zipLicenseMetadata(%v, %v): %v", licenseTypes, licensePaths, err)
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

// GetVersionForPackage returns the module version corresponding to path and
// version. *internal.Version will contain all packages for that version, in
// sorted order by package path.
func (db *DB) GetVersionForPackage(ctx context.Context, path, version string) (*internal.Version, error) {
	query := `SELECT
		p.path,
		p.module_path,
		p.name,
		p.synopsis,
		p.v1_path,
		p.license_types,
		p.license_paths,
		v.readme_file_path,
		v.readme_contents,
		v.commit_time,
		v.version_type,
		p.documentation,
		v.repository_url,
		v.vcs_type,
		v.homepage_url
	FROM
		packages p
	INNER JOIN
		versions v
	ON
		v.module_path = p.module_path
		AND v.version = p.version
	WHERE
		(p.module_path, p.version) IN (
			SELECT module_path, version
			FROM packages
			WHERE path = $1 AND version = $2
		)
	ORDER BY path;`

	var (
		pkgPath, modulePath, pkgName, synopsis, v1path, readmeFilePath, versionType string
		repositoryURL, vcsType, homepageURL                                         sql.NullString
		readmeContents, documentation                                               []byte
		commitTime                                                                  time.Time
		licenseTypes, licensePaths                                                  []string
	)

	rows, err := db.QueryContext(ctx, query, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %s, %q, %q): %v", query, path, version, err)
	}
	defer rows.Close()

	v := &internal.Version{}
	v.Version = version
	for rows.Next() {
		if err := rows.Scan(&pkgPath, &modulePath, &pkgName, &synopsis, &v1path,
			pq.Array(&licenseTypes), pq.Array(&licensePaths), &readmeFilePath,
			&readmeContents, &commitTime, &versionType, &documentation, &repositoryURL, &vcsType, &homepageURL); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		lics, err := zipLicenseMetadata(licenseTypes, licensePaths)
		if err != nil {
			return nil, fmt.Errorf("zipLicenseMetadata(%v, %v): %v", licenseTypes, licensePaths, err)
		}
		v.ModulePath = modulePath
		v.ReadmeFilePath = readmeFilePath
		v.ReadmeContents = readmeContents
		v.CommitTime = commitTime
		v.VersionType = internal.VersionType(versionType)
		v.RepositoryURL = repositoryURL.String
		v.VCSType = vcsType.String
		v.HomepageURL = homepageURL.String
		v.Packages = append(v.Packages, &internal.Package{
			Path:              pkgPath,
			Name:              pkgName,
			Synopsis:          synopsis,
			Licenses:          lics,
			V1Path:            v1path,
			DocumentationHTML: documentation,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(): %v", err)
	}

	return v, nil
}

// GetTaggedVersionsForPackageSeries returns a list of tagged versions sorted
// in descending order by major, minor and patch number and then lexicographically
// in descending order by prerelease. This list includes tagged versions of
// packages that have the same v1path.
func (db *DB) GetTaggedVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error) {
	return getVersions(ctx, db, path, []internal.VersionType{internal.VersionTypeRelease, internal.VersionTypePrerelease})
}

// GetPseudoVersionsForPackageSeries returns the 10 most recent from a list of
// pseudo-versions sorted in descending order by major, minor and patch number
// and then lexicographically in descending order by prerelease. This list includes
// pseudo-versions of packages that have the same v1path.
func (db *DB) GetPseudoVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error) {
	return getVersions(ctx, db, path, []internal.VersionType{internal.VersionTypePseudo})
}

// getVersions returns a list of versions sorted numerically
// in descending order by major, minor and patch number and then
// lexicographically in descending order by prerelease. The version types
// included in the list are specified by a list of VersionTypes. The results
// include the type of versions of packages that are part of the same series
// and have the same package v1path.
func getVersions(ctx context.Context, db *DB, path string, versionTypes []internal.VersionType) ([]*internal.VersionInfo, error) {
	var (
		modulePath, synopsis, version, v1path string
		commitTime                            time.Time
		versionHistory                        []*internal.VersionInfo
	)

	baseQuery := `SELECT
			p.module_path,
			p.version,
			p.v1_path,
			v.commit_time,
			p.synopsis
		FROM
			packages p
		INNER JOIN
			versions v
		ON
			p.module_path = v.module_path
			AND p.version = v.version
		WHERE
			p.v1_path IN (
				SELECT v1_path
				FROM packages
				WHERE path=$1
			)
			AND (%s)
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

	var (
		vtQuery []string
		params  = []interface{}{path}
	)
	for i, vt := range versionTypes {
		// v.version_type can be just version_type once
		// packages.version_type is dropped.
		vtQuery = append(vtQuery, fmt.Sprintf(`v.version_type = $%d`, i+2))
		params = append(params, vt.String())
	}

	query := fmt.Sprintf(baseQuery, strings.Join(vtQuery, " OR "), queryEnd)

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q): %v", query, path, err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&modulePath, &version, &v1path, &commitTime, &synopsis); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}

		versionHistory = append(versionHistory, &internal.VersionInfo{
			ModulePath: modulePath,
			Version:    version,
			CommitTime: commitTime,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(): %v", err)
	}

	return versionHistory, nil
}

// GetImports fetches and returns all of the imports for the package with path
// and version. If multiple packages have the same path and version, all of
// the imports will be returned.
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) GetImports(ctx context.Context, path, version string) ([]string, error) {
	if path == "" || version == "" {
		return nil, derrors.InvalidArgument("path and version cannot be empty")
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

	rows, err := db.QueryContext(ctx, query, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q, %q): %v", query, path, version, err)
	}
	defer rows.Close()

	var imports []string
	for rows.Next() {
		if err := rows.Scan(&toPath); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		imports = append(imports, toPath)
	}
	return imports, nil
}

// GetImportedBy fetches and returns all of the packages that import the
// package with path.
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) GetImportedBy(ctx context.Context, path string) ([]string, error) {
	if path == "" {
		return nil, derrors.InvalidArgument("path cannot be empty")
	}

	var fromPath string
	query := `
		SELECT
			DISTINCT ON (from_path) from_path
		FROM
			imports
		WHERE
			to_path = $1
		ORDER BY
			from_path;`

	rows, err := db.QueryContext(ctx, query, path)
	if err != nil {
		return nil, fmt.Errorf("db.Query(%q, %q) returned error: %v", query, path, err)
	}
	defer rows.Close()

	var importedby []string
	for rows.Next() {
		if err := rows.Scan(&fromPath); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		importedby = append(importedby, fromPath)
	}
	return importedby, nil
}

// GetLicenses returns all licenses associated with the given package path and
// version.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) GetLicenses(ctx context.Context, pkgPath, modulePath, version string) ([]*license.License, error) {
	if pkgPath == "" || version == "" {
		return nil, derrors.InvalidArgument("pkgPath and version cannot be empty")
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
			AND p.license_file_path = l.file_path
		ORDER BY l.file_path;`

	var (
		licenseTypes []string
		licensePath  string
		contents     []byte
	)
	rows, err := db.QueryContext(ctx, query, pkgPath, modulePath, version)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q): %v", query, pkgPath, err)
	}
	defer rows.Close()

	var licenses []*license.License
	for rows.Next() {
		if err := rows.Scan(pq.Array(&licenseTypes), &licensePath, &contents); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		licenses = append(licenses, &license.License{
			Metadata: license.Metadata{
				Types:    licenseTypes,
				FilePath: licensePath,
			},
			Contents: contents,
		})
	}
	sort.Slice(licenses, func(i, j int) bool {
		return compareLicenses(licenses[i].Metadata, licenses[j].Metadata)
	})
	return licenses, nil
}

// zipLicenseMetadata constructs license.Metadata from the given license types
// and paths, by zipping and then sorting.
func zipLicenseMetadata(licenseTypes []string, licensePaths []string) ([]*license.Metadata, error) {
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
		return compareLicenses(*mds[i], *mds[j])
	})
	return mds, nil
}

// compareLicenses reports whether i < j according to our license sorting
// semantics.
func compareLicenses(i, j license.Metadata) bool {
	if len(strings.Split(i.FilePath, "/")) > len(strings.Split(j.FilePath, "/")) {
		return true
	}
	return i.FilePath < j.FilePath
}

// GetVersion fetches a Version from the database with the primary key
// (module_path, version).
func (db *DB) GetVersion(ctx context.Context, modulePath string, version string) (*internal.VersionInfo, error) {
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
	row := db.QueryRowContext(ctx, query, modulePath, version)
	if err := row.Scan(&commitTime, &readmeFilePath, &readmeContents, &versionType, &repositoryURL, &vcsType, &homepageURL); err != nil {
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
