// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"

	// imported to register the postgres migration driver
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// imported to register the file source migration driver
	_ "github.com/golang-migrate/migrate/v4/source/file"
	// imported to register the postgres database driver
	_ "github.com/lib/pq"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// dbConnURI generates a postgres connection string in URI format.  This is
// necessary as migrate expects a URI.
func dbConnURI(dbName string) string {
	var (
		user     = getEnv("GO_DISCOVERY_DATABASE_TEST_USER", "postgres")
		password = getEnv("GO_DISCOVERY_DATABASE_TEST_PASSWORD", "")
		host     = getEnv("GO_DISCOVERY_DATABASE_TEST_HOST", "localhost")
	)
	cs := fmt.Sprintf("postgres://%s/%s?sslmode=disable&user=%s&password=%s",
		host, dbName, url.QueryEscape(user), url.QueryEscape(password))
	return cs
}

// NowTruncated returns time.Now() truncated to Microsecond precision.
//
// This makes it easier to work with timestamps in PostgreSQL, which have
// Microsecond precision:
//   https://www.postgresql.org/docs/9.1/datatype-datetime.html
func NowTruncated() time.Time {
	return time.Now().Truncate(time.Microsecond)
}

// multiErr can be used to combine one or more errors into a single error.
type multiErr []error

func (m multiErr) Error() string {
	var sb strings.Builder
	for _, err := range m {
		sep := ""
		if sb.Len() > 0 {
			sep = "|"
		}
		if err != nil {
			sb.WriteString(sep + err.Error())
		}
	}
	return sb.String()
}

// createDBIfNotExists checks whether the given dbName is an existing database,
// and creates one if not.
func createDBIfNotExists(dbName string) (outerErr error) {
	pg, err := sql.Open("postgres", dbConnURI(""))
	if err != nil {
		return err
	}
	defer func() {
		if err := pg.Close(); err != nil {
			outerErr = multiErr{outerErr, err}
		}
	}()

	rows, err := pg.Query("SELECT 1 from pg_database WHERE datname = $1 LIMIT 1", dbName)
	if err != nil {
		return err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		if _, err := pg.Exec(fmt.Sprintf("CREATE DATABASE %q;", dbName)); err != nil {
			return err
		}
	}
	return nil
}

func migrationsSource() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("unable to determine path to migrations directory")
	}
	dirpath := filepath.ToSlash(filepath.Dir(filename))
	migrationDir := path.Clean(path.Join(dirpath, "../../migrations"))
	return "file://" + migrationDir, nil
}

// SetupTestDB creates a test database named dbName if it does not already
// exist, and migrates it to the latest schema from the migrations directory.
func SetupTestDB(dbName string) (*DB, error) {
	if err := createDBIfNotExists(dbName); err != nil {
		return nil, fmt.Errorf("ensureDBExists(%q): %v", dbName, err)
	}
	dbURI := dbConnURI(dbName)
	source, err := migrationsSource()
	if err != nil {
		return nil, fmt.Errorf("migrationsSource(): %v", err)
	}
	m, err := migrate.New(source, dbURI)
	if err != nil {
		log.Fatalf("migrate.New(%q, [db=%q]): %v", source, dbName, err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		serr, dberr := m.Close()
		return nil, multiErr{fmt.Errorf("m.Up(): %v", err), serr, dberr}
	}
	m.Close()
	return Open(dbURI)
}

// ResetTestDB truncates all data from the given test DB.  It should be called
// after every test that mutates the database.
func ResetTestDB(db *DB, t *testing.T) {
	t.Helper()
	if err := db.Transact(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`TRUNCATE series CASCADE`); err != nil {
			return err
		}
		if _, err := tx.Exec(`TRUNCATE version_logs`); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("error resetting test DB: %v", err)
	}
}

// RunDBTests is a wrapper that runs the given testing suite in a test database
// named dbName.  The given *DB reference will be set to the instantiated test

func RunDBTests(dbName string, m *testing.M, testDB **DB) {
	db, err := SetupTestDB(dbName)
	if err != nil {
		log.Fatal(err)
	}
	*testDB = db
	code := m.Run()
	if err := db.Close(); err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}
