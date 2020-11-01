// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package postgres provides functionality for reading and writing to
// the postgres database.
package postgres

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/pkgsite/internal/database"
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
	p.Poll(ctx) // Initialize the state.
	p.Start(ctx, time.Minute)
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

// unitIDColumn returns the name of the column that is the foreign key to the units table.
// This is temporary, until we rename all the "path_id" columns to "unit_id".
func (db *DB) unitIDColumn(ctx context.Context, table string) string {
	// Make a trivial query.
	rows, err := db.db.Query(ctx, fmt.Sprintf(`SELECT * FROM %s LIMIT 1`, table))
	if err != nil {
		log.Errorf(ctx, "unitIDColumn: %v", err)
		return "path_id"
	}
	defer rows.Close()
	_ = rows.Next()
	cols, err := rows.Columns()
	if err != nil {
		log.Errorf(ctx, "unitIDColumn: rows.Columns: %v", err)
		return "path_id"
	}
	for _, c := range cols {
		if c == "unit_id" {
			return c
		}
	}
	return "path_id"
}
