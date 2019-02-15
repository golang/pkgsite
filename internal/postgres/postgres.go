// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"errors"
	"time"

	"golang.org/x/discovery/internal"
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
	row := db.QueryRow(query, internal.VersionLogProxyIndex)
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
				`INSERT INTO version_logs(name, version, created_at, source, error)
				VALUES ($1, $2, $3, $4, $5);`,
				l.Name, l.Version, l.CreatedAt, l.Source, l.Error,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetVersion fetches a Version from the database with the primary key
// (name, version).
func (db *DB) GetVersion(name string, version string) (*internal.Version, error) {
	var synopsis string
	var commitTime, createdAt, updatedAt time.Time
	var license string

	query := `
		SELECT
			created_at,
			updated_at,
			synopsis,
			commit_time,
			license
		FROM versions
		WHERE name = $1 and version = $2;`
	row := db.QueryRow(query, name, version)

	if err := row.Scan(&createdAt, &updatedAt, &synopsis, &commitTime, &license); err != nil {
		return nil, err
	}
	return &internal.Version{
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Module: &internal.Module{
			Name: name,
		},
		Version:    version,
		Synopsis:   synopsis,
		CommitTime: commitTime,
		License: &internal.License{
			Type: license,
		},
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
		return errors.New("postgres: cannot insert nil version")
	}

	return db.Transact(func(tx *sql.Tx) error {
		if _, err := tx.Exec(
			`INSERT INTO series(name)
			VALUES($1)
			ON CONFLICT DO NOTHING`,
			version.Module.Series.Name); err != nil {
			return err
		}

		if _, err := tx.Exec(
			`INSERT INTO modules(name, series_name)
			VALUES($1,$2)
			ON CONFLICT DO NOTHING`,
			version.Module.Name, version.Module.Series.Name); err != nil {
			return err
		}

		// TODO(ckimblebrown, julieqiu): Update Versions schema and insert readmes,
		// licenses, dependencies, and packages (the rest of the fields in the
		// internal.Version struct)
		if _, err := tx.Exec(
			`INSERT INTO versions(name, version, synopsis, commit_time, license, deleted)
			VALUES($1,$2,$3,$4,$5,$6)`,
			version.Module.Name,
			version.Version,
			version.Synopsis,
			version.CommitTime,
			version.License.Type,
			false,
		); err != nil {
			return err
		}

		return nil
	})
}
