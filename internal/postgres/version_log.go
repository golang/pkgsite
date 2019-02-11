// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"time"

	"golang.org/x/discovery/internal"
)

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
