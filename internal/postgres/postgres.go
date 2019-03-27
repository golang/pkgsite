// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
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

// LatestProxyIndexUpdate reports the last time the Proxy Index Cron
// successfully fetched data from the Module Proxy Index.
func (db *DB) LatestProxyIndexUpdate() (time.Time, error) {
	query := `
		SELECT created_at
		FROM version_logs
		WHERE source=$1
		ORDER BY created_at DESC
		LIMIT 1`

	var createdAt time.Time
	row := db.QueryRow(query, internal.VersionSourceProxyIndex)
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
func (db *DB) InsertVersionLogs(logs []*internal.VersionLog) error {
	return db.Transact(func(tx *sql.Tx) error {
		for _, l := range logs {
			if _, err := tx.Exec(
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
// (path, version).
func (db *DB) GetVersion(path string, version string) (*internal.Version, error) {
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
	row := db.QueryRow(query, path, version)
	if err := row.Scan(&createdAt, &updatedAt, &synopsis, &commitTime, &license, &readme); err != nil {
		return nil, fmt.Errorf("row.Scan(%q, %q, %q, %q, %q, %q): %v",
			createdAt, updatedAt, synopsis, commitTime, license, readme, err)
	}
	return &internal.Version{
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Module: &internal.Module{
			Path: path,
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
func (db *DB) GetPackage(path string, version string) (*internal.Package, error) {
	if path == "" || version == "" {
		return nil, status.Errorf(codes.InvalidArgument, "postgres: path and version cannot be empty")
	}

	var commitTime, createdAt, updatedAt time.Time
	var name, synopsis, license string
	query := `
		SELECT
			v.created_at,
			v.updated_at,
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
			p.path = $1
			AND p.version = $2
		LIMIT 1;`

	row := db.QueryRow(query, path, version)
	if err := row.Scan(&createdAt, &updatedAt, &commitTime, &license, &name, &synopsis); err != nil {
		return nil, fmt.Errorf("row.Scan(%q, %q, %q, %q, %q, %q): %v",
			createdAt, updatedAt, commitTime, license, name, synopsis, err)
	}

	return &internal.Package{
		Name:     name,
		Path:     path,
		Synopsis: synopsis,
		Version: &internal.Version{
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Module: &internal.Module{
				Path: path,
			},
			Version:    version,
			Synopsis:   synopsis,
			CommitTime: commitTime,
			License:    license,
		},
	}, nil
}

// GetLatestPackage returns the package from the database with the latest version.
// If multiple packages share the same path then the package that the database
// chooses is returned.
func (db *DB) GetLatestPackage(path string) (*internal.Package, error) {
	if path == "" {
		return nil, status.Errorf(codes.InvalidArgument, "postgres: path cannot be empty")
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

	row := db.QueryRow(query, path)
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
func padPrerelease(p string) (string, error) {
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
func (db *DB) InsertVersion(version *internal.Version) error {
	if version == nil || !semver.IsValid(version.Version) || version.Module == nil {
		return status.Errorf(codes.InvalidArgument, "postgres: cannot insert nil or invalid version")
	}

	if err := module.CheckPath(version.Module.Path); err != nil {
		return status.Errorf(codes.InvalidArgument, "postgres: cannot insert version with invalid module path: %v", err)
	}

	if version.Module == nil || version.Module.Path == "" || version.Version == "" || version.CommitTime.IsZero() {
		var errReasons []string
		if version.Module == nil || version.Module.Path == "" {
			errReasons = append(errReasons, "no module path")
		}
		if version.Version == "" {
			errReasons = append(errReasons, "no specified version")
		}
		if version.CommitTime.IsZero() {
			errReasons = append(errReasons, "empty commit time")
		}
		return status.Errorf(codes.InvalidArgument,
			fmt.Sprintf("postgres: cannot insert version %v: %s", version, strings.Join(errReasons, ", ")))
	}

	return db.Transact(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`INSERT INTO series(path)
			VALUES($1)
			ON CONFLICT DO NOTHING`,
			version.Module.Series.Path); err != nil {
			return fmt.Errorf("error inserting series: %v", err)
		}

		if _, err := tx.Exec(
			`INSERT INTO modules(path, series_path)
			VALUES($1,$2)
			ON CONFLICT DO NOTHING`,
			version.Module.Path, version.Module.Series.Path); err != nil {
			return fmt.Errorf("error inserting module: %v", err)
		}

		versionSplit := strings.Split(version.Version, ".")
		major, _ := strconv.ParseInt(strings.TrimPrefix("v", versionSplit[0]), 0, 64)
		minor, _ := strconv.ParseInt(versionSplit[1], 0, 64)
		patch, _ := strconv.ParseInt(strings.Split(versionSplit[2], semver.Prerelease(version.Version))[0], 0, 64)
		prerelease, err := padPrerelease(semver.Prerelease(version.Version))
		if err != nil {
			return fmt.Errorf("error padding prerelease: %v", err)
		}

		if _, err := tx.Exec(
			`INSERT INTO versions(module_path, version, synopsis, commit_time, license, readme, major, minor, patch, prerelease)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`,
			version.Module.Path,
			version.Version,
			version.Synopsis,
			version.CommitTime,
			version.License,
			version.ReadMe,
			major,
			minor,
			patch,
			prerelease,
		); err != nil {
			return fmt.Errorf("error inserting version: %v", err)
		}

		stmt, err := tx.Prepare(
			`INSERT INTO packages (path, synopsis, name, version, module_path, version_type)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`)
		if err != nil {
			return fmt.Errorf("error preparing package stmt: %v", err)
		}

		for _, p := range version.Packages {
			if _, err = stmt.Exec(p.Path, p.Synopsis, p.Name, version.Version, version.Module.Path, version.VersionType.String()); err != nil {
				return fmt.Errorf("error inserting package: %v", err)
			}
		}

		return nil
	})
}

// GetLatestPackageForPaths returns a list of packages that have the latest version that
// corresponds to each path specified in the list of paths. The resulting list is
// sorted by package path lexicographically. So if multiple packages have the same
// path then the package whose module path comes first lexicographically will be
// returned.
func (db *DB) GetLatestPackageForPaths(paths []string) ([]*internal.Package, error) {
	var (
		packages                                           []*internal.Package
		commitTime, createdAt, updatedAt                   time.Time
		path, modulePath, name, synopsis, license, version string
	)

	query := `
		SELECT DISTINCT ON (p.path)
			v.created_at,
			v.updated_at,
			p.path,
			p.module_path,
			v.version,
			v.commit_time,
			v.license,
			p.name,
			p.synopsis
		FROM
			packages p
		INNER JOIN
			versions v
		ON
			v.module_path = p.module_path
			AND v.version = p.version
		WHERE
			p.path = $1
		ORDER BY
			p.path,
			p.module_path,
			v.major DESC,
			v.minor DESC,
			v.patch DESC,
			v.prerelease DESC;`

	anyPaths := fmt.Sprintf("ANY('%s'::text[])", strings.Join(paths, ", "))
	rows, err := db.Query(query, anyPaths)
	if err != nil {
		return nil, fmt.Errorf("db.Query(%q, %q) returned error: %v", query, anyPaths, err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Scan(&createdAt, &updatedAt, &path, &modulePath, &version, &commitTime, &license, &name, &synopsis); err != nil {
			return nil, fmt.Errorf("row.Scan(%q, %q, %q, %q, %q, %q, %q, %q, %q): %v",
				createdAt, updatedAt, path, modulePath, version, commitTime, license, name, synopsis, err)
		}

		packages = append(packages, &internal.Package{
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
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err() returned error %v", err)
	}

	return packages, nil
}
