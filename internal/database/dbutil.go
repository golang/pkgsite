// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/testhelper"

	// imported to register the postgres migration driver
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"

	// imported to register the file source migration driver
	_ "github.com/golang-migrate/migrate/v4/source/file"
	// imported to register the postgres database driver
	_ "github.com/lib/pq"
)

// DBConnURI generates a postgres connection string in URI format.  This is
// necessary as migrate expects a URI.
func DBConnURI(dbName string) string {
	var (
		user     = config.GetEnv("GO_DISCOVERY_DATABASE_USER", "postgres")
		password = config.GetEnv("GO_DISCOVERY_DATABASE_PASSWORD", "")
		host     = config.GetEnv("GO_DISCOVERY_DATABASE_HOST", "localhost")
		port     = config.GetEnv("GO_DISCOVERY_DATABASE_PORT", "5432")
	)
	cs := fmt.Sprintf("postgres://%s/%s?sslmode=disable&user=%s&password=%s&port=%s&timezone=UTC",
		host, dbName, url.QueryEscape(user), url.QueryEscape(password), url.QueryEscape(port))
	return cs
}

// MultiErr can be used to combine one or more errors into a single error.
type MultiErr []error

func (m MultiErr) Error() string {
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

// ConnectAndExecute connects to the postgres database specified by uri and
// executes dbFunc, then cleans up the database connection.
// It returns an error that Is derrors.NotFound if no connection could be made.
func ConnectAndExecute(uri string, dbFunc func(*sql.DB) error) (outerErr error) {
	pg, err := sql.Open("postgres", uri)
	if err == nil {
		err = pg.Ping()
	}
	if err != nil {
		return fmt.Errorf("%w: %v", derrors.NotFound, err)
	}
	defer func() {
		if err := pg.Close(); err != nil {
			outerErr = MultiErr{outerErr, err}
		}
	}()
	return dbFunc(pg)
}

// CreateDB creates a new database dbName.
func CreateDB(dbName string) error {
	return ConnectAndExecute(DBConnURI(""), func(pg *sql.DB) error {
		if _, err := pg.Exec(fmt.Sprintf(`
			CREATE DATABASE %q
				TEMPLATE=template0
				LC_COLLATE='C'
				LC_CTYPE='C';`, dbName)); err != nil {
			return fmt.Errorf("error creating %q: %v", dbName, err)
		}

		return nil
	})
}

// DropDB drops the database named dbName.
func DropDB(dbName string) error {
	return ConnectAndExecute(DBConnURI(""), func(pg *sql.DB) error {
		if _, err := pg.Exec(fmt.Sprintf("DROP DATABASE %q;", dbName)); err != nil {
			return fmt.Errorf("error dropping %q: %v", dbName, err)
		}
		return nil
	})
}

// CreateDBIfNotExists checks whether the given dbName is an existing database,
// and creates one if not.
func CreateDBIfNotExists(dbName string) error {
	exists, err := checkIfDBExists(dbName)
	if err != nil || exists {
		return err
	}

	log.Printf("Database %q does not exist, creating.", dbName)
	return CreateDB(dbName)
}

// checkIfDBExists check if dbName exists.
func checkIfDBExists(dbName string) (bool, error) {
	var exists bool

	err := ConnectAndExecute(DBConnURI(""), func(pg *sql.DB) error {
		rows, err := pg.Query("SELECT 1 from pg_database WHERE datname = $1 LIMIT 1", dbName)
		if err != nil {
			return err
		}
		defer rows.Close()

		if rows.Next() {
			exists = true
			return nil
		}

		return rows.Err()
	})

	return exists, err
}

// TryToMigrate attempts to migrate the database named dbName to the latest
// migration. If this operation fails in the migration step, it returns
// isMigrationError=true to signal that the database should be recreated.
func TryToMigrate(dbName string) (isMigrationError bool, outerErr error) {
	dbURI := DBConnURI(dbName)
	source := migrationsSource()
	m, err := migrate.New(source, dbURI)
	if err != nil {
		return false, fmt.Errorf("migrate.New(): %v", err)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			outerErr = MultiErr{outerErr, srcErr, dbErr}
		}
	}()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return true, fmt.Errorf("m.Up(): %v", err)
	}
	return false, nil
}

// migrationsSource returns a uri pointing to the migrations directory.  It
// returns an error if unable to determine this path.
func migrationsSource() string {
	migrationsDir := testhelper.TestDataPath("../../migrations")
	return "file://" + filepath.ToSlash(migrationsDir)
}

// ResetDB truncates all data from the given test DB.  It should be called
// after every test that mutates the database.
func ResetDB(ctx context.Context, db *DB) error {
	if err := db.Transact(ctx, sql.LevelDefault, func(tx *DB) error {
		if _, err := tx.Exec(ctx, `
			TRUNCATE modules CASCADE;
			TRUNCATE search_documents;
			TRUNCATE version_map;
			TRUNCATE paths CASCADE;
			TRUNCATE symbol_names CASCADE;
			TRUNCATE imports_unique;
			TRUNCATE latest_module_versions;`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `TRUNCATE module_version_states CASCADE;`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `TRUNCATE excluded_prefixes;`); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error resetting test DB: %v", err)
	}
	return nil
}
