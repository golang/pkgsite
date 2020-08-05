// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"golang.org/x/pkgsite/internal/database"
)

type DB struct {
	db                 *database.DB
	bypassLicenseCheck bool
}

// New returns a new postgres DB.
func New(db *database.DB) *DB {
	return &DB{db, false}
}

// NewBypassingLicenseCheck returns a new postgres DB that bypasses license
// checks on insertion. That means all data will be inserted for
// non-redistributable modules and packages.
func NewBypassingLicenseCheck(db *database.DB) *DB {
	return &DB{db, true}
}

// Close closes a DB.
func (db *DB) Close() error {
	return db.db.Close()
}

// Underlying returns the *database.DB inside db.
func (db *DB) Underlying() *database.DB {
	return db.db
}
