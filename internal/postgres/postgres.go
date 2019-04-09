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

	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/thirdparty/module"
	"golang.org/x/discovery/internal/thirdparty/semver"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DB struct {
	*sql.DB
}

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
		return
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	return txFunc(tx)
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

// GetVersion fetches a Version from the database with the primary key
// (module_path, version).
func (db *DB) GetVersion(ctx context.Context, modulePath string, version string) (*internal.Version, error) {
	var (
		commitTime, createdAt, updatedAt time.Time
		synopsis, license                string
		readme                           []byte
	)

	query := `
		SELECT
			created_at,
			updated_at,
			synopsis,
			commit_time,
			license,
			readme
		FROM versions
		WHERE module_path = $1 and version = $2;`
	row := db.QueryRowContext(ctx, query, modulePath, version)
	if err := row.Scan(&createdAt, &updatedAt, &synopsis, &commitTime, &license, &readme); err != nil {
		return nil, fmt.Errorf("row.Scan(%q, %q, %q, %q, %q, %q): %v",
			createdAt, updatedAt, synopsis, commitTime, license, readme, err)
	}
	return &internal.Version{
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Module: &internal.Module{
			Path: modulePath,
		},
		Version:    version,
		Synopsis:   synopsis,
		CommitTime: commitTime,
		License:    license,
		ReadMe:     readme,
	}, nil
}

// GetPackage returns the first package from the database that has path and
// version.
func (db *DB) GetPackage(ctx context.Context, path string, version string) (*internal.Package, error) {
	if path == "" || version == "" {
		return nil, status.Errorf(codes.InvalidArgument, "postgres: path and version cannot be empty")
	}

	var (
		commitTime, createdAt, updatedAt     time.Time
		name, synopsis, license, module_path string
		readme                               []byte
	)
	query := `
		SELECT
			v.created_at,
			v.updated_at,
			v.commit_time,
			v.license,
			v.readme,
			v.module_path,
			p.name,
			p.synopsis
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
	if err := row.Scan(&createdAt, &updatedAt, &commitTime, &license, &readme, &module_path, &name, &synopsis); err != nil {
		return nil, fmt.Errorf("row.Scan(%q, %q, %q, %q, %q, %q, %q, %q): %v",
			createdAt, updatedAt, commitTime, license, readme, module_path, name, synopsis, err)
	}

	return &internal.Package{
		Name:     name,
		Path:     path,
		Synopsis: synopsis,
		Version: &internal.Version{
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Module: &internal.Module{
				Path: module_path,
			},
			Version:    version,
			Synopsis:   synopsis,
			CommitTime: commitTime,
			License:    license,
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
func getVersions(ctx context.Context, db *DB, path string, versionTypes []internal.VersionType) ([]*internal.Version, error) {
	var (
		commitTime                                               time.Time
		pkgPath, modulePath, pkgName, synopsis, license, version string
		versionHistory                                           []*internal.Version
	)

	baseQuery :=
		`WITH package_series AS (
			SELECT
				m.series_path,
				p.path AS package_path,
				p.suffix AS package_suffix,
				p.module_path,
				p.name,
				v.version,
				v.commit_time,
				v.license,
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
			package_path,
			module_path,
			name,
			version,
			commit_time,
			license,
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
		return nil, fmt.Errorf("db.QueryContext(ctx, %q, %q) returned error: %v", query, path, err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&pkgPath, &modulePath, &pkgName, &version, &commitTime, &license, &synopsis); err != nil {
			return nil, fmt.Errorf("row.Scan( %q, %q, %q, %q, %q, %q, %q): %v",
				pkgPath, modulePath, pkgName, version, commitTime, license, synopsis, err)
		}

		versionHistory = append(versionHistory, &internal.Version{
			Module: &internal.Module{
				Path: modulePath,
			},
			Version:    version,
			Synopsis:   synopsis,
			CommitTime: commitTime,
			License:    license,
			Packages: []*internal.Package{
				&internal.Package{
					Path: pkgPath,
					Name: pkgName,
				},
			},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err() returned error %v", err)
	}

	return versionHistory, nil
}

// GetTaggedVersionsForPackageSeries returns a list of tagged versions sorted
// in descending order by major, minor and patch number and then lexicographically
// in descending order by prerelease. This list includes tagged versions of
// packages that are part of the same series and have the same package suffix.
func (db *DB) GetTaggedVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.Version, error) {
	return getVersions(ctx, db, path, []internal.VersionType{internal.VersionTypeRelease, internal.VersionTypePrerelease})
}

// GetPseudoVersionsForPackageSeries returns the 10 most recent from a list of
// pseudo-versions sorted in descending order by major, minor and patch number
// and then lexicographically in descending order by prerelease. This list includes
// pseudo-versions of packages that are part of the same series and have the same
// package suffix.
func (db *DB) GetPseudoVersionsForPackageSeries(ctx context.Context, path string) ([]*internal.Version, error) {
	return getVersions(ctx, db, path, []internal.VersionType{internal.VersionTypePseudo})
}

// GetLatestPackage returns the package from the database with the latest version.
// If multiple packages share the same path then the package that the database
// chooses is returned.
func (db *DB) GetLatestPackage(ctx context.Context, path string) (*internal.Package, error) {
	if path == "" {
		return nil, fmt.Errorf("postgres: path cannot be empty")
	}

	var (
		commitTime, createdAt, updatedAt             time.Time
		modulePath, name, synopsis, license, version string
	)
	query := `
		SELECT
			v.created_at,
			v.updated_at,
			p.module_path,
			v.version,
			v.commit_time,
			v.license,
			p.name,
			p.synopsis
		FROM
			versions v
		INNER JOIN
			packages p
		ON
			p.module_path = v.module_path
			AND v.version = p.version
		WHERE
			path = $1
		ORDER BY
			v.module_path,
			v.major DESC,
			v.minor DESC,
			v.patch DESC,
			v.prerelease DESC
		LIMIT 1;`

	row := db.QueryRowContext(ctx, query, path)
	if err := row.Scan(&createdAt, &updatedAt, &modulePath, &version, &commitTime, &license, &name, &synopsis); err != nil {
		return nil, fmt.Errorf("row.Scan(%q, %q, %q, %q, %q, %q, %q, %q): %v",
			createdAt, updatedAt, modulePath, version, commitTime, license, name, synopsis, err)
	}

	return &internal.Package{
		Name:     name,
		Path:     path,
		Synopsis: synopsis,
		Version: &internal.Version{
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Module: &internal.Module{
				Path: modulePath,
			},
			Version:    version,
			CommitTime: commitTime,
			License:    license,
		},
	}, nil
}

// GetVersionForPackage returns the module version corresponding to path and
// version. *internal.Version will contain all packages for that version.
func (db *DB) GetVersionForPackage(ctx context.Context, path, version string) (*internal.Version, error) {
	query := `SELECT
		p.path,
		p.module_path,
		p.name,
		p.synopsis,
		v.readme,
		v.license,
		v.commit_time
	FROM
		packages p
	INNER JOIN
		versions v
	ON
		v.module_path = p.module_path
		AND v.version = p.version
	WHERE
		p.version = $1
		AND p.module_path IN (
			SELECT module_path
			FROM packages
			WHERE path=$2
		)
	ORDER BY name, path;`

	var (
		pkgPath, modulePath, pkgName, synopsis, license string
		readme                                          []byte
		commitTime                                      time.Time
	)

	rows, err := db.QueryContext(ctx, query, version, path)
	if err != nil {
		return nil, fmt.Errorf("db.QueryContext(ctx, %s, %q) returned error: %v", query, path, err)
	}
	defer rows.Close()

	v := &internal.Version{
		Module:  &internal.Module{},
		Version: version,
	}
	for rows.Next() {
		if err := rows.Scan(&pkgPath, &modulePath, &pkgName, &synopsis, &readme, &license, &commitTime); err != nil {
			return nil, fmt.Errorf("row.Scan( %q, %q, %q, %q): %v",
				pkgPath, modulePath, pkgName, synopsis, err)
		}
		v.Module.Path = modulePath
		v.ReadMe = readme
		v.License = license
		v.CommitTime = commitTime
		v.Packages = append(v.Packages, &internal.Package{
			Path:     pkgPath,
			Name:     pkgName,
			Synopsis: synopsis,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err() returned error %v", err)
	}

	return v, nil

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
// The prerelease column will pad any number fields with zeroes on the left
// so all number fields in the prerelease column have 20 characters. If the
// version is malformed then insertion will fail.
func (db *DB) InsertVersion(ctx context.Context, version *internal.Version) error {
	if err := validateVersion(version); err != nil {
		return status.Errorf(codes.InvalidArgument, fmt.Sprintf("validateVersion: %v", err))
	}

	return db.Transact(func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO series(path)
			VALUES($1)
			ON CONFLICT DO NOTHING`,
			version.Module.Series.Path); err != nil {
			return fmt.Errorf("error inserting series: %v", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO modules(path, series_path)
			VALUES($1,$2)
			ON CONFLICT DO NOTHING`,
			version.Module.Path, version.Module.Series.Path); err != nil {
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
			`INSERT INTO versions(module_path, version, synopsis, commit_time, license, readme, major, minor, patch, prerelease)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
			version.Module.Path,
			version.Version,
			version.Synopsis,
			version.CommitTime,
			version.License,
			version.ReadMe,
			majorint,
			minorint,
			patchint,
			prerelease,
		); err != nil {
			return fmt.Errorf("error inserting version: %v", err)
		}

		stmt, err := tx.Prepare(
			`INSERT INTO packages (path, synopsis, name, version, module_path, version_type, suffix)
			VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`)
		if err != nil {
			return fmt.Errorf("error preparing package stmt: %v", err)
		}
		defer stmt.Close()

		for _, p := range version.Packages {
			if _, err = stmt.ExecContext(ctx, p.Path, p.Synopsis, p.Name, version.Version, version.Module.Path, version.VersionType.String(), p.Suffix); err != nil {
				return fmt.Errorf("error inserting package: %v", err)
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

	if version.Version == "" {
		errReasons = append(errReasons, "no specified version")
	} else if !semver.IsValid(version.Version) {
		errReasons = append(errReasons, "invalid version")
	}

	if version.Module == nil || version.Module.Path == "" {
		errReasons = append(errReasons, "no module path")
	} else if err := module.CheckPath(version.Module.Path); err != nil {
		errReasons = append(errReasons, "invalid module path")
	} else if version.Module.Series == nil || version.Module.Series.Path == "" {
		errReasons = append(errReasons, "no series path")
	}

	if version.CommitTime.IsZero() {
		errReasons = append(errReasons, "empty commit time")
	}

	if len(errReasons) == 0 {
		return nil
	}

	return fmt.Errorf("cannot insert module %+v at version %q: %s", version.Module, version.Version, strings.Join(errReasons, ", "))
}
