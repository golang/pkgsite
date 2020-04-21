// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/dbtest"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"

	// imported to register the postgres migration driver
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// imported to register the file source migration driver
	_ "github.com/golang-migrate/migrate/v4/source/file"
	// imported to register the postgres database driver
	_ "github.com/lib/pq"
)

// recreateDB drops and recreates the database named dbName.
func recreateDB(dbName string) error {
	return dbtest.ConnectAndExecute(dbtest.DBConnURI(""), func(pg *sql.DB) error {
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
	dbURI := dbtest.DBConnURI(dbName)
	source := migrationsSource()
	m, err := migrate.New(source, dbURI)
	if err != nil {
		return false, fmt.Errorf("migrate.New(): %v", err)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			outerErr = dbtest.MultiErr{outerErr, srcErr, dbErr}
		}
	}()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return true, fmt.Errorf("m.Up(): %v", err)
	}
	return false, nil
}

// SetupTestDB creates a test database named dbName if it does not already
// exist, and migrates it to the latest schema from the migrations directory.
func SetupTestDB(dbName string) (_ *DB, err error) {
	defer derrors.Wrap(&err, "SetupTestDB(%q)", dbName)

	if err := dbtest.CreateDBIfNotExists(dbName); err != nil {
		return nil, fmt.Errorf("createDBIfNotExists(%q): %v", dbName, err)
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
			return nil, fmt.Errorf("unfixable error migrating database: %v.\nConsider running ./scripts/drop_test_dbs.sh", err)
		}
	}
	db, err := database.Open("postgres", dbtest.DBConnURI(dbName))
	if err != nil {
		return nil, err
	}
	return New(db), nil
}

// ResetTestDB truncates all data from the given test DB.  It should be called
// after every test that mutates the database.
func ResetTestDB(db *DB, t *testing.T) {
	ctx := context.Background()
	t.Helper()
	if err := db.db.Transact(ctx, func(tx *database.DB) error {
		if _, err := tx.Exec(ctx, `
			TRUNCATE modules CASCADE;
			TRUNCATE imports_unique;
			TRUNCATE experiments;`); err != nil {
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
		t.Fatalf("error resetting test DB: %v", err)
	}
}

// RunDBTests is a wrapper that runs the given testing suite in a test database
// named dbName.  The given *DB reference will be set to the instantiated test
// database.
func RunDBTests(dbName string, m *testing.M, testDB **DB) {
	database.QueryLoggingDisabled = true
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

// InsertSampleDirectory tree inserts a set of packages for testing
// GetDirectory and frontend.FetchDirectoryDetails.
func InsertSampleDirectoryTree(ctx context.Context, t *testing.T, testDB *DB) {
	t.Helper()

	for _, data := range []struct {
		modulePath, version string
		paths               []string
	}{
		{
			"std",
			"v1.13.4",
			[]string{
				"archive/zip",
				"archive/tar",
				"cmd/go",
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
		{
			"std",
			"v1.13.0",
			[]string{
				"archive/zip",
				"archive/tar",
				"cmd/go",
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
		{
			"github.com/hashicorp/vault/api",
			"v1.1.2",
			[]string{"github.com/hashicorp/vault/api"},
		},
		{
			"github.com/hashicorp/vault",
			"v1.1.2",
			[]string{
				"github.com/hashicorp/vault/api",
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
				"github.com/hashicorp/vault/vault/replication",
				"github.com/hashicorp/vault/vault/seal/transit",
			},
		},
		{
			"github.com/hashicorp/vault",
			"v1.2.3",
			[]string{
				"github.com/hashicorp/vault/internal/foo",
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
				"github.com/hashicorp/vault/vault/replication",
				"github.com/hashicorp/vault/vault/seal/transit",
			},
		},
		{
			"github.com/hashicorp/vault",
			"v1.0.3",
			[]string{
				"github.com/hashicorp/vault/api",
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
			},
		},
	} {
		var pkgs []*internal.Package
		for _, path := range data.paths {
			p := sample.Package()
			p.Path = path
			p.Imports = nil
			pkgs = append(pkgs, p)
		}

		m := sample.Module()
		m.ModulePath = data.modulePath
		m.Version = data.version
		m.Packages = pkgs
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

}

// GetFromSearchDocuments retrieves the module path and version for the given
// package path from the search_documents table. If the path is not in the table,
// the third return value is false.
func GetFromSearchDocuments(ctx context.Context, t *testing.T, db *DB, packagePath string) (modulePath, version string, found bool) {
	row := db.db.QueryRow(ctx, `
			SELECT module_path, version
			FROM search_documents
			WHERE package_path = $1`,
		packagePath)
	err := row.Scan(&modulePath, &version)
	switch err {
	case sql.ErrNoRows:
		return "", "", false
	case nil:
		return modulePath, version, true
	default:
		t.Fatal(err)
	}
	return
}
