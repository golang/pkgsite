// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package postgres provides functionality for reading and writing to
// the postgres database.
package postgres

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/log"
	"golang.org/x/pkgsite/internal/poller"
)

type DB struct {
	db                 *database.DB
	bypassLicenseCheck bool
	expoller           *poller.Poller
	cancel             func()
}

// New returns a new postgres DB.
func New(db *database.DB) *DB {
	return newdb(db, false)
}

// NewBypassingLicenseCheck returns a new postgres DB that bypasses license
// checks. That means all data will be inserted and returned for
// non-redistributable modules, packages and directories.
func NewBypassingLicenseCheck(db *database.DB) *DB {
	return newdb(db, true)
}

// For testing.
var startPoller = true

func newdb(db *database.DB, bypass bool) *DB {
	p := poller.New(
		[]string(nil),
		func(ctx context.Context) (interface{}, error) {
			return getExcludedPrefixes(ctx, db)
		},
		func(err error) {
			log.Errorf(context.Background(), "getting excluded prefixes: %v", err)
		})
	ctx, cancel := context.WithCancel(context.Background())
	if startPoller {
		p.Poll(ctx) // Initialize the state.
		p.Start(ctx, time.Minute)
	}
	return &DB{
		db:                 db,
		bypassLicenseCheck: bypass,
		expoller:           p,
		cancel:             cancel,
	}
}

// Close closes a DB.
func (db *DB) Close() error {
	db.cancel()
	return db.db.Close()
}

// Underlying returns the *database.DB inside db.
func (db *DB) Underlying() *database.DB {
	return db.db
}

// StalenessTimestamp returns the index timestamp of the oldest
// module that is newer than the index timestamp of the youngest module we have
// processed. That is, let T be the maximum index timestamp of all processed
// modules. Then this function return the minimum index timestamp of unprocessed
// modules that is no less than T, or an error that wraps derrors.NotFound if
// there is none.
//
// The name of the function is imprecise: there may be an older unprocessed
// module, if one newer than it has been processed.
//
// We use this function to compute a metric that is a lower bound on the time
// it takes to process a module since it appeared in the index.
func (db *DB) StalenessTimestamp(ctx context.Context) (time.Time, error) {
	var ts time.Time
	err := db.db.QueryRow(ctx, `
		SELECT m.index_timestamp
		FROM module_version_states m
		CROSS JOIN (
			-- the index timestamp of the youngest processed module
			SELECT index_timestamp
			FROM module_version_states
			WHERE last_processed_at IS NOT NULL
			ORDER BY 1 DESC
			LIMIT 1
		) yp
		WHERE m.index_timestamp > yp.index_timestamp
		AND last_processed_at IS NULL
		ORDER BY m.index_timestamp ASC
		LIMIT 1
	`).Scan(&ts)
	switch err {
	case nil:
		return ts, nil
	case sql.ErrNoRows:
		return time.Time{}, derrors.NotFound
	default:
		return time.Time{}, err
	}
}

// NumUnprocessedModules returns the number of modules that need to be processed.
func (db *DB) NumUnprocessedModules(ctx context.Context) (total, new int, err error) {
	defer derrors.Wrap(&err, "NumUnprocessedModules()")

	err = db.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM module_version_states WHERE status = 0 OR status >= 500
	`).Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	err = db.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM module_version_states WHERE status = 0 OR status = 500
	`).Scan(&new)
	if err != nil {
		return 0, 0, err
	}
	return total, new, nil
}
