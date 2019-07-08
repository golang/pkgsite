// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"golang.org/x/discovery/internal/testhelper"

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

// connectAndExecute connects to the postgres database specified by uri and
// executes dbFunc, then cleans up the database connection.
func connectAndExecute(uri string, dbFunc func(*sql.DB) error) (outerErr error) {
	pg, err := sql.Open("postgres", uri)
	if err != nil {
		return err
	}
	defer func() {
		if err := pg.Close(); err != nil {
			outerErr = multiErr{outerErr, err}
		}
	}()
	return dbFunc(pg)
}

// createDBIfNotExists checks whether the given dbName is an existing database,
// and creates one if not.
func createDBIfNotExists(dbName string) error {
	return connectAndExecute(dbConnURI(""), func(pg *sql.DB) error {
		rows, err := pg.Query("SELECT 1 from pg_database WHERE datname = $1 LIMIT 1", dbName)
		if err != nil {
			return err
		}
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return err
			}
			log.Printf("Test database %q does not exist, creating.", dbName)
			if _, err := pg.Exec(fmt.Sprintf("CREATE DATABASE %q;", dbName)); err != nil {
				return fmt.Errorf("error creating %q: %v", dbName, err)
			}
		}
		return nil
	})
}

// recreateDB drops and recreates the database named dbName.
func recreateDB(dbName string) error {
	return connectAndExecute(dbConnURI(""), func(pg *sql.DB) error {
		if _, err := pg.Exec(fmt.Sprintf("DROP DATABASE %q;", dbName)); err != nil {
			return fmt.Errorf("error dropping %q: %v", dbName, err)
		}
		if _, err := pg.Exec(fmt.Sprintf("CREATE DATABASE %q;", dbName)); err != nil {
			return fmt.Errorf("error creating %q: %v", dbName, err)
		}
		return nil
	})
}

// migrationsSource returns a uri pointing to the migrations directory.  It
// returns an error if unable to determine this path.
func migrationsSource() string {
	migrationsDir := testhelper.TestDataPath("../../migrations")
	return "file://" + filepath.ToSlash(migrationsDir)
}

// tryToMigrate attempts to migrate the database named dbName to the latest
// migration. If this operation fails in the migration step, it returns
// isMigrationError=true to signal that the database should be recreated.
func tryToMigrate(dbName string) (isMigrationError bool, outerErr error) {
	dbURI := dbConnURI(dbName)
	source := migrationsSource()
	m, err := migrate.New(source, dbURI)
	if err != nil {
		return false, fmt.Errorf("migrate.New(): %v", err)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			outerErr = multiErr{outerErr, srcErr, dbErr}
		}
	}()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return true, fmt.Errorf("m.Up(): %v", err)
	}
	return false, nil
}

// SetupTestDB creates a test database named dbName if it does not already
// exist, and migrates it to the latest schema from the migrations directory.
func SetupTestDB(dbName string) (*DB, error) {
	if err := createDBIfNotExists(dbName); err != nil {
		return nil, fmt.Errorf("ensureDBExists(%q): %v", dbName, err)
	}
	if isMigrationError, err := tryToMigrate(dbName); err != nil {
		if isMigrationError {
			// failed during migration stage, recreate and try again
			log.Printf("Migration failed for %s: %v, recreating database.", dbName, err)
			if err := recreateDB(dbName); err != nil {
				return nil, fmt.Errorf("recreateDB(%q): %v", dbName, err)
			}
			_, err = tryToMigrate(dbName)
		}
		if err != nil {
			return nil, fmt.Errorf("unfixable error migrating database: %v", err)
		}
	}
	return Open("postgres", dbConnURI(dbName))
}

// ResetTestDB truncates all data from the given test DB.  It should be called
// after every test that mutates the database.
func ResetTestDB(db *DB, t *testing.T) {
	t.Helper()
	if err := db.Transact(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`TRUNCATE versions CASCADE;`); err != nil {
			return err
		}
		if _, err := tx.Exec(`TRUNCATE module_version_states;`); err != nil {
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
