// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

var (
	user       = getEnv("GO_DISCOVERY_DATABASE_TEST_USER", "postgres")
	password   = getEnv("GO_DISCOVERY_DATABASE_TEST_PASSWORD", "")
	host       = getEnv("GO_DISCOVERY_DATABASE_TEST_HOST", "localhost")
	testdbname = getEnv("GO_DISCOVERY_DATABASE_TEST_NAME", "discovery-database-test")
	testdb     = fmt.Sprintf("user=%s password=%s host=%s dbname=%s sslmode=disable", user, password, host, testdbname)
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// NowTruncated returns time.Now() truncated to Microsecond precision.
//
// This makes it easier to work with timestamps in PostgreSQL, which have
// Microsecond precision:
//   https://www.postgresql.org/docs/9.1/datatype-datetime.html
func NowTruncated() time.Time {
	return time.Now().Truncate(time.Microsecond)
}

// SetupCleanDB is used to test functions that execute Postgres queries. It
// should only ever be used for testing. It makes a connection to a test
// Postgres database and truncates all the tables in the database after the
// test is complete.
func SetupCleanDB(t *testing.T) (func(t *testing.T), *DB) {
	t.Helper()
	db, err := Open(testdb)
	if err != nil {
		t.Fatalf("Open(%q), error: %v", testdb, err)
	}
	cleanup := func(t *testing.T) {
		// truncates series and any tables that uses it as a foreign key.
		// This includes: modules, versions, documents, packages, and dependencies.
		db.Exec(`TRUNCATE series CASCADE;`)
		db.Exec(`TRUNCATE version_logs;`) // truncates version_logs
	}
	return cleanup, db
}
