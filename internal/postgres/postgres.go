// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
)

// DB wraps a sql.DB to provide an API for interacting with discovery data
// stored in Postgres.
type DB struct {
	*sql.DB
}

// Open creates a new DB for the given Postgres connection string.
func Open(dbinfo string) (*DB, error) {
	db, err := sql.Open("postgres", dbinfo)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

// Transact executes the given function in the context of a SQL transaction,
// rolling back the transaction if the function panics or returns an error.
func (db *DB) Transact(txFunc func(*sql.Tx) error) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("db.Begin(): %v", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			if err = tx.Commit(); err != nil {
				err = fmt.Errorf("tx.Commit(): %v", err)
			}
		}
	}()

	if err := txFunc(tx); err != nil {
		return fmt.Errorf("txFunc(tx): %v", err)
	}
	return nil
}

// prepareAndExec prepares a query statement and executes it insde the provided
// transaction.
func prepareAndExec(tx *sql.Tx, query string, stmtFunc func(*sql.Stmt) error) (err error) {
	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(%q): %v", query, err)
	}

	defer func() {
		if err = stmt.Close(); err != nil {
			err = fmt.Errorf("stmt.Close: %v", err)
		}
	}()
	return stmtFunc(stmt)
}

// buildInsertQuery builds an multi-value insert query, following the format:
// INSERT TO <table> (<columns>) VALUES
// (<placeholders-for-each-item-in-values>) If conflictNoAction is true, it
// append ON CONFLICT DO NOTHING to the end of the query.
func buildInsertQuery(table string, columns []string, values []interface{}, conflictNoAction bool) (string, error) {
	if remainder := len(values) % len(columns); remainder != 0 {
		return "", fmt.Errorf("modulus of len(values) and len(columns) must be 0: got %d", remainder)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s", table)
	fmt.Fprintf(&b, "(%s) VALUES", strings.Join(columns, ", "))

	var placeholders []string
	for i := 1; i <= len(values); i++ {
		// Construct the full query by adding placeholders for each
		// set of values that we want to insert.
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		if i%len(columns) != 0 {
			continue
		}

		// When the end of a set is reached, write it to the query
		// builder and reset placeholders.
		fmt.Fprintf(&b, "(%s)", strings.Join(placeholders, ", "))
		placeholders = []string{}

		// Do not add a comma delimiter after the last set of values.
		if i == len(values) {
			break
		}
		b.WriteString(", ")
	}

	if conflictNoAction {
		b.WriteString("ON CONFLICT DO NOTHING")
	}

	return b.String(), nil
}

// bulkInsert constructs and executes a multi-value insert statement. The
// query is constructed using the format: INSERT TO <table> (<columns>) VALUES
// (<placeholders-for-each-item-in-values>) If conflictNoAction is true, it
// append ON CONFLICT DO NOTHING to the end of the query. The query is executed
// using a PREPARE statement with the provided values.
func bulkInsert(ctx context.Context, tx *sql.Tx, table string, columns []string, values []interface{}, conflictNoAction bool) error {
	query, err := buildInsertQuery(table, columns, values, conflictNoAction)
	if err != nil {
		return fmt.Errorf("buildInsertQuery(%q, %v, values, %t): %v", table, columns, conflictNoAction, err)
	}

	if _, err := tx.ExecContext(ctx, query, values...); err != nil {
		return fmt.Errorf("tx.ExecContext(ctx, %q, values): %v", query, err)
	}
	return nil
}

// LatestProxyIndexUpdate reports the last time the Proxy Index Cron
// successfully fetched data from the Module Proxy Index.
func (db *DB) LatestProxyIndexUpdate(ctx context.Context) (time.Time, error) {
	query := `
		SELECT created_at
		FROM version_logs
		WHERE source=$1
		ORDER BY created_at DESC
		LIMIT 1`

	var createdAt time.Time
	row := db.QueryRowContext(ctx, query, internal.VersionSourceProxyIndex)
	switch err := row.Scan(&createdAt); err {
	case sql.ErrNoRows:
		return time.Time{}, nil
	case nil:
		return createdAt, nil
	default:
		return time.Time{}, err
	}
}

// InsertVersionLogs inserts a VersionLog into the database and
// insertion fails and returns an error if the VersionLog's primary
// key already exists in the database.
func (db *DB) InsertVersionLogs(ctx context.Context, logs []*internal.VersionLog) error {
	return db.Transact(func(tx *sql.Tx) error {
		for _, l := range logs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO version_logs(module_path, version, created_at, source, error)
				VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING;`,
				l.ModulePath, l.Version, l.CreatedAt, l.Source, l.Error,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func zipLicenseInfo(licenseTypes []string, licensePaths []string) ([]*internal.LicenseInfo, error) {
	if len(licenseTypes) != len(licensePaths) {
		return nil, fmt.Errorf("BUG: got %d license types and %d license paths", len(licenseTypes), len(licensePaths))
	}
	var licenseFiles []*internal.LicenseInfo
	for i, t := range licenseTypes {
		licenseFiles = append(licenseFiles, &internal.LicenseInfo{Type: t, FilePath: licensePaths[i]})
	}
	return licenseFiles, nil
}

// GetVersion fetches a Version from the database with the primary key
// (module_path, version).
func (db *DB) GetVersion(ctx context.Context, modulePath string, version string) (*internal.VersionInfo, error) {
	var (
		commitTime time.Time
		seriesPath string
		readme     []byte
	)

	query := `
		SELECT
			m.series_path,
			v.commit_time,
			v.readme
		FROM
			versions v
		INNER JOIN
			modules m
		ON
			m.path = v.module_path
		WHERE module_path = $1 and version = $2;`
	row := db.QueryRowContext(ctx, query, modulePath, version)
	if err := row.Scan(&seriesPath, &commitTime, &readme); err != nil {
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}
	return &internal.VersionInfo{
		SeriesPath: seriesPath,
		ModulePath: modulePath,
		Version:    version,
		CommitTime: commitTime,
		ReadMe:     readme,
	}, nil
}

// GetLicenses returns all licenses associated with the given package path and
// version.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) GetLicenses(ctx context.Context, pkgPath string, version string) ([]*internal.License, error) {
	if pkgPath == "" || version == "" {
		return nil, derrors.InvalidArgument("pkgPath and version cannot be empty")
	}
	query := `
		SELECT
			l.type,
			l.file_path,
			l.contents
		FROM
			licenses l
		INNER JOIN
			package_licenses pl
		ON
			pl.module_path = l.module_path
			AND pl.version = l.version
			AND pl.file_path = l.file_path
		INNER JOIN
			packages p
		ON
			p.module_path = pl.module_path
			AND p.version = pl.version
			AND p.path = pl.package_path
		WHERE
			p.path = $1
			AND p.version = $2
		ORDER BY l.file_path;`

	var (
		licenseType, licensePath string
		contents                 []byte
	)
	rows, err := db.QueryContext(ctx, query, pkgPath, version)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q): %v", query, pkgPath, err)
	}
	defer rows.Close()

	var licenses []*internal.License
	for rows.Next() {
		if err := rows.Scan(&licenseType, &licensePath, &contents); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		licenses = append(licenses, &internal.License{
			LicenseInfo: internal.LicenseInfo{
				Type:     licenseType,
				FilePath: licensePath,
			},
			Contents: contents,
		})
	}
	return licenses, nil
}

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
		commitTime                                     time.Time
		name, synopsis, seriesPath, modulePath, suffix string
		readme                                         []byte
		licenseTypes, licensePaths                     []string
	)
	query := `
		SELECT
			v.commit_time,
			p.license_types,
			p.license_paths,
			v.readme,
			m.series_path,
			v.module_path,
			p.name,
			p.synopsis,
			p.suffix
		FROM
			versions v
		INNER JOIN
			vw_licensed_packages p
		ON
			p.module_path = v.module_path
			AND v.version = p.version
		INNER JOIN
			modules m
		ON
		  m.path = v.module_path
		WHERE
			p.path = $1
			AND p.version = $2
		LIMIT 1;`

	row := db.QueryRowContext(ctx, query, path, version)
	if err := row.Scan(&commitTime, pq.Array(&licenseTypes),
		pq.Array(&licensePaths), &readme, &seriesPath, &modulePath, &name, &synopsis, &suffix); err != nil {
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	lics, err := zipLicenseInfo(licenseTypes, licensePaths)
	if err != nil {
		return nil, fmt.Errorf("zipLicenseInfo(%v, %v): %v", licenseTypes, licensePaths, err)
	}

	return &internal.VersionedPackage{
		Package: internal.Package{
			Name:     name,
			Path:     path,
			Synopsis: synopsis,
			Licenses: lics,
			Suffix:   suffix,
		},
		VersionInfo: internal.VersionInfo{
			SeriesPath: seriesPath,
			ModulePath: modulePath,
			Version:    version,
			CommitTime: commitTime,
			ReadMe:     readme,
		},
	}, nil
}

// getVersions returns a list of versions sorted numerically
// in descending order by major, minor and patch number and then
// lexicographically in descending order by prerelease. The version types
// included in the list are specified by a list of VersionTypes. The results
// include the type of versions of packages that are part of the same series
// and have the same package suffix as the package specified by the path.
func getVersions(ctx context.Context, db *DB, path string, versionTypes []internal.VersionType) ([]*internal.VersionInfo, error) {
	var (
		commitTime                                time.Time
		seriesPath, modulePath, synopsis, version string
		versionHistory                            []*internal.VersionInfo
	)

	baseQuery := `WITH package_series AS (
			SELECT
				m.series_path,
				p.path AS package_path,
				p.suffix AS package_suffix,
				p.module_path,
				v.version,
				v.commit_time,
				p.synopsis,
				v.major,
				v.minor,
				v.patch,
				v.prerelease,
				p.version_type
			FROM
				modules m
			INNER JOIN
				packages p
			ON
				p.module_path = m.path
			INNER JOIN
				versions v
			ON
				p.module_path = v.module_path
				AND p.version = v.version
		), filters AS (
			SELECT
				series_path,
				package_suffix
			FROM
				package_series
			WHERE
				package_path = $1
		)
		SELECT
			series_path,
			module_path,
			version,
			commit_time,
			synopsis
		FROM
			package_series
		WHERE
			series_path IN (SELECT series_path FROM filters)
			AND package_suffix IN (SELECT package_suffix FROM filters)
			AND (%s)
		ORDER BY
			module_path DESC,
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

	var (
		vtQuery []string
		params  []interface{}
	)
	params = append(params, path)
	for i, vt := range versionTypes {
		vtQuery = append(vtQuery, fmt.Sprintf(`version_type = $%d`, i+2))
		params = append(params, vt.String())
	}

	query := fmt.Sprintf(baseQuery, strings.Join(vtQuery, " OR "), queryEnd)

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q): %v", query, path, err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&seriesPath, &modulePath, &version, &commitTime, &synopsis); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}

		versionHistory = append(versionHistory, &internal.VersionInfo{
			SeriesPath: seriesPath,
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

// GetTaggedVersionsForPackageSeries returns a list of tagged versions sorted
// in descending order by major, minor and patch number and then lexicographically
// in descending order by prerelease. This list includes tagged versions of
// packages that are part of the same series and have the same package suffix.
func (db *DB) GetTaggedVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error) {
	return getVersions(ctx, db, path, []internal.VersionType{internal.VersionTypeRelease, internal.VersionTypePrerelease})
}

// GetPseudoVersionsForPackageSeries returns the 10 most recent from a list of
// pseudo-versions sorted in descending order by major, minor and patch number
// and then lexicographically in descending order by prerelease. This list includes
// pseudo-versions of packages that are part of the same series and have the same
// package suffix.
func (db *DB) GetPseudoVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.VersionInfo, error) {
	return getVersions(ctx, db, path, []internal.VersionType{internal.VersionTypePseudo})
}

// GetLatestPackage returns the package from the database with the latest version.
// If multiple packages share the same path then the package that the database
// chooses is returned.
func (db *DB) GetLatestPackage(ctx context.Context, path string) (*internal.VersionedPackage, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	var (
		commitTime, createdAt, updatedAt                        time.Time
		seriesPath, modulePath, name, synopsis, version, suffix string
		licenseTypes, licensePaths                              []string
	)
	query := `
		SELECT
			v.created_at,
			v.updated_at,
			m.series_path,
			p.module_path,
			p.license_types,
			p.license_paths,
			v.version,
			v.commit_time,
			p.name,
			p.synopsis,
			p.suffix
		FROM
			versions v
		INNER JOIN
			modules m
		ON
			v.module_path = m.path
		INNER JOIN
			vw_licensed_packages p
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
	if err := row.Scan(&createdAt, &updatedAt, &seriesPath, &modulePath, pq.Array(&licenseTypes), pq.Array(&licensePaths),
		&version, &commitTime, &name, &synopsis, &suffix); err != nil {
		return nil, fmt.Errorf("row.Scan(): %v", err)
	}

	lics, err := zipLicenseInfo(licenseTypes, licensePaths)
	if err != nil {
		return nil, fmt.Errorf("zipLicenseInfo(%v, %v): %v", licenseTypes, licensePaths, err)
	}

	return &internal.VersionedPackage{
		Package: internal.Package{
			Name:     name,
			Path:     path,
			Synopsis: synopsis,
			Licenses: lics,
			Suffix:   suffix,
		},
		VersionInfo: internal.VersionInfo{
			SeriesPath: seriesPath,
			ModulePath: modulePath,
			Version:    version,
			CommitTime: commitTime,
		},
	}, nil
}

// GetVersionForPackage returns the module version corresponding to path and
// version. *internal.Version will contain all packages for that version.
func (db *DB) GetVersionForPackage(ctx context.Context, path, version string) (*internal.Version, error) {
	query := `SELECT
		p.path,
		m.series_path,
		p.module_path,
		p.name,
		p.synopsis,
		p.suffix,
		p.license_types,
		p.license_paths,
		v.readme,
		v.commit_time
	FROM
		vw_licensed_packages p
	INNER JOIN
		versions v
	ON
		v.module_path = p.module_path
		AND v.version = p.version
	INNER JOIN
		modules m
	ON
		m.path = v.module_path
	WHERE
		p.version = $1
		AND p.module_path IN (
			SELECT module_path
			FROM packages
			WHERE path=$2
		)
	ORDER BY name, path;`

	var (
		pkgPath, seriesPath, modulePath, pkgName, synopsis, suffix string
		readme                                                     []byte
		commitTime                                                 time.Time
		licenseTypes, licensePaths                                 []string
	)

	rows, err := db.QueryContext(ctx, query, version, path)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %s, %q): %v", query, path, err)
	}
	defer rows.Close()

	v := &internal.Version{}
	v.Version = version
	for rows.Next() {
		if err := rows.Scan(&pkgPath, &seriesPath, &modulePath, &pkgName, &synopsis, &suffix,
			pq.Array(&licenseTypes), pq.Array(&licensePaths), &readme, &commitTime); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		lics, err := zipLicenseInfo(licenseTypes, licensePaths)
		if err != nil {
			return nil, fmt.Errorf("zipLicenseInfo(%v, %v): %v", licenseTypes, licensePaths, err)
		}
		v.SeriesPath = seriesPath
		v.ModulePath = modulePath
		v.ReadMe = readme
		v.CommitTime = commitTime
		v.Packages = append(v.Packages, &internal.Package{
			Path:     pkgPath,
			Name:     pkgName,
			Synopsis: synopsis,
			Licenses: lics,
			Suffix:   suffix,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(): %v", err)
	}

	return v, nil
}

// GetImports fetches and returns all of the imports for the package with path
// and version. If multiple packages have the same path and version, all of
// the imports will be returned.
// The returned error may be checked with derrors.IsInvalidArgument to
// determine if it resulted from an invalid package path or version.
func (db *DB) GetImports(ctx context.Context, path, version string) ([]*internal.Import, error) {
	if path == "" || version == "" {
		return nil, derrors.InvalidArgument("path and version cannot be empty")
	}

	var toPath, toName string
	query := `
		SELECT
			to_name,
			to_path
		FROM
			imports
		WHERE
			from_path = $1
			AND from_version = $2
		ORDER BY
			to_path,
			to_name;`

	rows, err := db.QueryContext(ctx, query, path, version)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q, %q): %v", query, path, version, err)
	}
	defer rows.Close()

	var imports []*internal.Import
	for rows.Next() {
		if err := rows.Scan(&toName, &toPath); err != nil {
			return nil, fmt.Errorf("row.Scan(): %v", err)
		}
		imports = append(imports, &internal.Import{
			Name: toName,
			Path: toPath,
		})
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

// prefixZeroes returns a string that is padded with zeroes on the
// left until the string is exactly 20 characters long. If the string
// is already 20 or more characters it is returned unchanged. 20
// characters being the length because the length of a date in the form
// yyyymmddhhmmss has 14 characters and that is longest number that
// is expected to be found in a prerelease number field.
func prefixZeroes(s string) (string, error) {
	if len(s) > 20 {
		return "", fmt.Errorf("prefixZeroes(%v): input string is more than 20 characters", s)
	}

	if len(s) == 20 {
		return s, nil
	}

	var padded []string

	for i := 0; i < 20-len(s); i++ {
		padded = append(padded, "0")
	}

	return strings.Join(append(padded, s), ""), nil
}

// isNum returns true if every character in a string is a number
// and returns false otherwise.
func isNum(v string) bool {
	i := 0
	for i < len(v) && '0' <= v[i] && v[i] <= '9' {
		i++
	}
	return len(v) > 0 && i == len(v)
}

// padPrerelease returns '~' if the given string is empty
// and otherwise pads all number fields with zeroes so that
// the resulting field is 20 characters and returns that
// string without the '-' prefix. The '~' is returned so that
// full releases will take greatest precedence when sorting
// in ASCII sort order. The given string may only contain
// lowercase letters, numbers, periods, hyphens or nothing.
func padPrerelease(v string) (string, error) {
	p := semver.Prerelease(v)
	if p == "" {
		return "~", nil
	}

	pre := strings.Split(strings.TrimPrefix(p, "-"), ".")
	var err error

	for i, segment := range pre {
		if isNum(segment) {
			pre[i], err = prefixZeroes(segment)
			if err != nil {
				return "", fmt.Errorf("padRelease(%v): number field %v is longer than 20 characters", p, segment)
			}
		}
	}

	return strings.Join(pre, "."), nil
}

// InsertVersion inserts a Version into the database along with any necessary
// series, modules and packages. If any of these rows already exist, they will
// not be updated. The version string is also parsed into major, minor, patch
// and prerelease used solely for sorting database queries by semantic version.
// The prerelease column will pad any number fields with zeroes on the left so
// all number fields in the prerelease column have 20 characters. If the
// version is malformed then insertion will fail.
//
// The returned error may be checked with derrors.IsInvalidArgument to
// determine whether it was caused by an invalid version or module.
func (db *DB) InsertVersion(ctx context.Context, version *internal.Version, licenses []*internal.License) error {
	if err := validateVersion(version); err != nil {
		return derrors.InvalidArgument(fmt.Sprintf("validateVersion: %v", err))
	}

	return db.Transact(func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO series(path)
			VALUES($1)
			ON CONFLICT DO NOTHING`,
			version.SeriesPath); err != nil {
			return fmt.Errorf("error inserting series: %v", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO modules(path, series_path)
			VALUES($1,$2)
			ON CONFLICT DO NOTHING`,
			version.ModulePath, version.SeriesPath); err != nil {
			return fmt.Errorf("error inserting module: %v", err)
		}

		majorint, err := major(version.Version)
		if err != nil {
			return fmt.Errorf("major(%q): %v", version.Version, err)
		}

		minorint, err := minor(version.Version)
		if err != nil {
			return fmt.Errorf("minor(%q): %v", version.Version, err)
		}

		patchint, err := patch(version.Version)
		if err != nil {
			return fmt.Errorf("patch(%q): %v", version.Version, err)
		}

		prerelease, err := padPrerelease(version.Version)
		if err != nil {
			return fmt.Errorf("padPrerelease(%q): %v", version.Version, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO versions(module_path, version, commit_time, readme, major, minor, patch, prerelease)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`,
			version.ModulePath,
			version.Version,
			version.CommitTime,
			version.ReadMe,
			majorint,
			minorint,
			patchint,
			prerelease,
		); err != nil {
			return fmt.Errorf("error inserting version: %v", err)
		}

		var licenseValues []interface{}
		for _, l := range licenses {
			licenseValues = append(licenseValues, version.ModulePath, version.Version, l.FilePath, l.Contents, l.Type)
		}
		if len(licenseValues) > 0 {
			licenseCols := []string{
				"module_path",
				"version",
				"file_path",
				"contents",
				"type",
			}
			table := "licenses"
			if err := bulkInsert(ctx, tx, table, licenseCols, licenseValues, true); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, [%d licenseValues]): %v", table, licenseCols, len(licenseValues), err)
			}
		}

		var pkgValues []interface{}
		var importValues []interface{}
		var pkgLicenseValues []interface{}
		for _, p := range version.Packages {
			pkgValues = append(pkgValues, p.Path, p.Synopsis, p.Name, version.Version, version.ModulePath, version.VersionType.String(), p.Suffix)

			for _, l := range p.Licenses {
				pkgLicenseValues = append(pkgLicenseValues, version.ModulePath, version.Version, l.FilePath, p.Path)
			}

			for _, i := range p.Imports {
				importValues = append(importValues, p.Path, version.ModulePath, version.Version, i.Path, i.Name)
			}
		}
		if len(pkgValues) > 0 {
			pkgCols := []string{
				"path",
				"synopsis",
				"name",
				"version",
				"module_path",
				"version_type",
				"suffix",
			}
			table := "packages"
			if err := bulkInsert(ctx, tx, table, pkgCols, pkgValues, true); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, %d pkgValues): %v", table, pkgCols, len(pkgValues), err)
			}
		}
		if len(pkgLicenseValues) > 0 {
			pkgLicenseCols := []string{
				"module_path",
				"version",
				"file_path",
				"package_path",
			}
			table := "package_licenses"
			if err := bulkInsert(ctx, tx, table, pkgLicenseCols, pkgLicenseValues, true); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, %d pkgLicenseValues): %v", table, pkgLicenseCols, len(pkgLicenseValues), err)
			}
		}

		if len(importValues) > 0 {
			importCols := []string{
				"from_path",
				"from_module_path",
				"from_version",
				"to_path",
				"to_name",
			}
			table := "imports"
			if err := bulkInsert(ctx, tx, table, importCols, importValues, true); err != nil {
				return fmt.Errorf("bulkInsert(ctx, tx, %q, %v, %d importValues): %v", table, importCols, len(importValues), err)
			}
		}
		return nil
	})
}

// major returns the major version integer value of the semantic version
// v.  For example, major("v2.1.0") == 2.
func major(v string) (int, error) {
	m := strings.TrimPrefix(semver.Major(v), "v")
	major, err := strconv.Atoi(m)
	if err != nil {
		return 0, fmt.Errorf("strconv.Atoi(%q): %v", m, err)
	}
	return major, nil
}

// minor returns the minor version integer value of the semantic version For
// example, minor("v2.1.0") == 1.
func minor(v string) (int, error) {
	m := strings.TrimPrefix(semver.MajorMinor(v), fmt.Sprintf("%s.", semver.Major(v)))
	minor, err := strconv.Atoi(m)
	if err != nil {
		return 0, fmt.Errorf("strconv.Atoi(%q): %v", m, err)
	}
	return minor, nil
}

// patch returns the patch version integer value of the semantic version For
// example, patch("v2.1.0+incompatible") == 0.
func patch(v string) (int, error) {
	s := strings.TrimPrefix(semver.Canonical(v), fmt.Sprintf("%s.", semver.MajorMinor(v)))
	p := strings.TrimSuffix(s, semver.Prerelease(v))
	patch, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("strconv.Atoi(%q): %v", p, err)
	}
	return patch, nil
}

// validateVersion checks that fields needed to insert a version into the
// database are present. Otherwise, it returns an error listing the reasons the
// version cannot be inserted.
func validateVersion(version *internal.Version) error {
	if version == nil {
		return fmt.Errorf("nil version")
	}

	var errReasons []string

	if version.SeriesPath == "" {
		errReasons = append(errReasons, "no series path")
	}
	if version.Version == "" {
		errReasons = append(errReasons, "no specified version")
	} else if !semver.IsValid(version.Version) {
		errReasons = append(errReasons, "invalid version")
	}

	if version.ModulePath == "" {
		errReasons = append(errReasons, "no module path")
	} else if err := module.CheckPath(version.ModulePath); err != nil {
		errReasons = append(errReasons, "invalid module path")
	}

	if version.CommitTime.IsZero() {
		errReasons = append(errReasons, "empty commit time")
	}

	if len(errReasons) == 0 {
		return nil
	}

	return fmt.Errorf("cannot insert version %q: %s", version.Version, strings.Join(errReasons, ", "))
}
