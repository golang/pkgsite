// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"database/sql"
	"testing"
	"time"
)

func TestSetPoolSettings(t *testing.T) {
	// We use an empty DSN which the driver might reject, but sql.Open does not
	// actually connect or ping.
	sdb, err := sql.Open("postgres", "postgres://localhost/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()

	db := New(sdb, "test")

	t.Run("valid settings", func(t *testing.T) {
		maxOpen := 42
		maxIdle := 13
		maxLifetime := 10 * time.Minute
		maxIdleTime := 5 * time.Minute

		db.SetPoolSettings(maxOpen, maxIdle, maxLifetime, maxIdleTime)

		stats := db.Underlying().Stats()
		if stats.MaxOpenConnections != maxOpen {
			t.Errorf("got %d, want %d", stats.MaxOpenConnections, maxOpen)
		}
	})

	t.Run("maxIdle > maxOpen", func(t *testing.T) {
		maxOpen := 10
		maxIdle := 20
		maxLifetime := 10 * time.Minute
		maxIdleTime := 5 * time.Minute

		// This should log a warning and cap maxIdle to maxOpen.
		db.SetPoolSettings(maxOpen, maxIdle, maxLifetime, maxIdleTime)

		// We can't easily check internal sql.DB settings, but we verify it doesn't crash.
	})
}
