// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dbtest supports testing with a database.
package dbtest

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"golang.org/x/pkgsite/internal/derrors"
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

// DBConnURI generates a postgres connection string in URI format.  This is
// necessary as migrate expects a URI.
func DBConnURI(dbName string) string {
	var (
		user     = getEnv("GO_DISCOVERY_DATABASE_TEST_USER", "postgres")
		password = getEnv("GO_DISCOVERY_DATABASE_TEST_PASSWORD", "")
		host     = getEnv("GO_DISCOVERY_DATABASE_TEST_HOST", "localhost")
		port     = getEnv("GO_DISCOVERY_DATABASE_TEST_PORT", "5432")
	)
	cs := fmt.Sprintf("postgres://%s/%s?sslmode=disable&user=%s&password=%s&port=%s",
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

// CreateDBIfNotExists checks whether the given dbName is an existing database,
// and creates one if not.
func CreateDBIfNotExists(dbName string) error {
	return ConnectAndExecute(DBConnURI(""), func(pg *sql.DB) error {
		rows, err := pg.Query("SELECT 1 from pg_database WHERE datname = $1 LIMIT 1", dbName)
		if err != nil {
			return err
		}
		defer rows.Close()
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
