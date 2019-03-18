// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"golang.org/x/discovery/internal"
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
	var commitTime, createdAt, updatedAt time.Time
	var synopsis, license, readme string
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

// InsertVersion inserts a Version into the database along with any
// necessary series and modules. Insertion fails and returns an error
// if the Version's primary key already exists in the database.
// Inserting a Version connected to a series or module that already
// exists in the database will not update the existing series or
// module.
func (db *DB) InsertVersion(version *internal.Version) error {
	if version == nil {
		return status.Errorf(codes.InvalidArgument, "postgres: cannot insert nil version")
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
			return err
		}

		if _, err := tx.Exec(
			`INSERT INTO modules(path, series_path)
			VALUES($1,$2)
			ON CONFLICT DO NOTHING`,
			version.Module.Path, version.Module.Series.Path); err != nil {
			return err
		}

		if _, err := tx.Exec(
			`INSERT INTO versions(module_path, version, synopsis, commit_time, license, readme)
			VALUES($1,$2,$3,$4,$5,$6)`,
			version.Module.Path,
			version.Version,
			version.Synopsis,
			version.CommitTime,
			version.License,
			version.ReadMe,
		); err != nil {
			return err
		}

		return nil
	})
}
