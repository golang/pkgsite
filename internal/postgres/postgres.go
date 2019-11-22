// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"

	"golang.org/x/discovery/internal/database"
)

type DB struct {
	db *database.DB
}

// New returns a new postgres DB.
func New(db *database.DB) *DB {
	return &DB{db}
}

// Close closes a DB.
func (db *DB) Close() error {
	return db.db.Close()
}

// Underlying returns the *database.DB inside db.
func (db *DB) Underlying() *database.DB {
	return db.db
}

// TODO(jba): remove.
// GetSQLDB returns the underlying SQL database for the postgres instance. This
// allows the ETL to perform streaming operations on the database.
func (db *DB) GetSQLDB() *sql.DB {
	return db.db.Underlying()
}
