// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/discovery/internal"

	_ "github.com/lib/pq"
)

var (
	user     = getEnv("GO_DISCOVERY_DATABASE_TEST_USER", "postgres")
	password = getEnv("GO_DISCOVERY_DATABASE_TEST_PASSWORD", "")
	host     = getEnv("GO_DISCOVERY_DATABASE_TEST_HOST", "localhost")
	dbname   = getEnv("GO_DISCOVERY_DATABASE_TEST_NAME", "discovery-database")
	testdb   = fmt.Sprintf("user=%s host=%s dbname=%s sslmode=disable", user, host, testdbname)
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func setupTestCase(t *testing.T) (func(t *testing.T), *DB) {
	db, err := Open(testdb)
	if err != nil {
		t.Fatalf("Open(testdb) error: %v", err)
	}
	fn := func(t *testing.T) {
		db.Exec(`TRUNCATE version_logs;`) // truncates the version_logs table
	}
	return fn, db
}

func TestLatestProxyIndexUpdateReturnsNilWithNoRows(t *testing.T) {
	teardownTestCase, db := setupTestCase(t)
	defer teardownTestCase(t)

	dbTime, err := db.LatestProxyIndexUpdate()
	if err != nil {
		t.Errorf("db.LatestProxyIndexUpdate error: %v", err)
	}

	if !dbTime.IsZero() {
		t.Errorf("db.LatestProxyIndexUpdate() = %v, want %v", dbTime, time.Time{})
	}
}

func TestLatestProxyIndexUpdateReturnsLatestTimestamp(t *testing.T) {
	teardownTestCase, db := setupTestCase(t)
	defer teardownTestCase(t)

	now := time.Now().UTC()
	newVersions := []*internal.VersionLog{
		&internal.VersionLog{
			Name:      "testModule",
			Version:   "v.1.0.0",
			CreatedAt: now.Add(-10 * time.Minute),
			Source:    internal.VersionLogProxyIndex,
		},
		&internal.VersionLog{
			Name:      "testModule",
			Version:   "v.1.1.0",
			CreatedAt: now,
			Source:    internal.VersionLogProxyIndex,
		},
		&internal.VersionLog{
			Name:      "testModule/v2",
			Version:   "v.2.0.0",
			CreatedAt: now,
			Source:    internal.VersionLogProxyIndex,
		},
	}

	if err := db.InsertVersionLogs(newVersions); err != nil {
		t.Errorf("db.InsertVersionLogs(newVersions) error: %v", err)
	}

	dbTime, err := db.LatestProxyIndexUpdate()
	if err != nil {
		t.Errorf("db.LatestProxyIndexUpdate error: %v", err)
	}
	if !dbTime.Equal(now) {
		t.Errorf("db.LatestProxyIndexUpdate() = %v, want %v", dbTime, now)
	}
}

func TestInsertVersionLogs(t *testing.T) {
	teardownTestCase, db := setupTestCase(t)
	defer teardownTestCase(t)

	now := time.Now().UTC()
	newVersions := []*internal.VersionLog{
		&internal.VersionLog{
			Name:      "testModule",
			Version:   "v.1.0.0",
			CreatedAt: now.Add(-10 * time.Minute),
			Source:    internal.VersionLogProxyIndex,
		},
		&internal.VersionLog{
			Name:      "testModule",
			Version:   "v.1.1.0",
			CreatedAt: now,
			Source:    internal.VersionLogProxyIndex,
		},
		&internal.VersionLog{
			Name:      "testModule/v2",
			Version:   "v.2.0.0",
			CreatedAt: now,
			Source:    internal.VersionLogProxyIndex,
		},
	}

	if err := db.InsertVersionLogs(newVersions); err != nil {
		t.Errorf("db.InsertVersionLogs(newVersions) error: %v", err)
	}
}
