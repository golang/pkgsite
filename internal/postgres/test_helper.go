// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	err := dbtest.DropDB(dbName)
	if err != nil {
		return err
	}

	return dbtest.CreateDB(dbName)
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
		return nil, fmt.Errorf("CreateDBIfNotExists(%q): %w", dbName, err)
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
			return nil, fmt.Errorf("unfixable error migrating database: %v.\nConsider running ./devtools/drop_test_dbs.sh", err)
		}
	}
	driver := os.Getenv("GO_DISCOVERY_DATABASE_DRIVER")
	if driver == "" {
		driver = "postgres"
	}
	db, err := database.Open(driver, dbtest.DBConnURI(dbName), "test")
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
	if err := db.db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
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
		t.Fatalf("error resetting test DB: %v", err)
	}
	db.expoller.Poll(ctx) // clear excluded prefixes
}

// RunDBTests is a wrapper that runs the given testing suite in a test database
// named dbName.  The given *DB reference will be set to the instantiated test
// database.
func RunDBTests(dbName string, m *testing.M, testDB **DB) {
	database.QueryLoggingDisabled = true
	db, err := SetupTestDB(dbName)
	if err != nil {
		if errors.Is(err, derrors.NotFound) && os.Getenv("GO_DISCOVERY_TESTDB") != "true" {
			log.Printf("SKIPPING: could not connect to DB (see doc/postgres.md to set up): %v", err)
			return
		}
		log.Fatal(err)
	}
	*testDB = db
	code := m.Run()
	if err := db.Close(); err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}

// RunDBTestsInParallel sets up numDBs databases, then runs the tests. Before it runs them,
// it sets acquirep to a function that tests should use to acquire a database. The second
// return value of the function should be called in a defer statement to release the database.
// For example:
//
//    func Test(t *testing.T) {
//        db, release := acquire(t)
//        defer release()
func RunDBTestsInParallel(dbBaseName string, numDBs int, m *testing.M, acquirep *func(*testing.T) (*DB, func())) {
	start := time.Now()
	database.QueryLoggingDisabled = true
	dbs := make(chan *DB, numDBs)
	for i := 0; i < numDBs; i++ {
		db, err := SetupTestDB(fmt.Sprintf("%s-%d", dbBaseName, i))
		if err != nil {
			if errors.Is(err, derrors.NotFound) && os.Getenv("GO_DISCOVERY_TESTDB") != "true" {
				log.Printf("SKIPPING: could not connect to DB (see doc/postgres.md to set up): %v", err)
				return
			}
			log.Fatal(err)
		}
		dbs <- db
	}

	*acquirep = func(t *testing.T) (*DB, func()) {
		db := <-dbs
		release := func() {
			ResetTestDB(db, t)
			dbs <- db
		}
		return db, release
	}

	log.Printf("parallel test setup for %d DBs took %s", numDBs, time.Since(start))
	code := m.Run()
	if len(dbs) != cap(dbs) {
		log.Fatal("not all DBs were released")
	}
	for i := 0; i < numDBs; i++ {
		db := <-dbs
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}
	os.Exit(code)
}

// MustInsertModule inserts m into db, calling t.Fatal on error.
func MustInsertModule(ctx context.Context, t *testing.T, db *DB, m *internal.Module) {
	MustInsertModuleLMV(ctx, t, db, m, nil)
}

func MustInsertModuleLMV(ctx context.Context, t *testing.T, db *DB, m *internal.Module, lmv *internal.LatestModuleVersions) {
	t.Helper()
	if _, err := db.InsertModule(ctx, m, lmv); err != nil {
		t.Fatal(err)
	}
}

// InsertSampleDirectory tree inserts a set of packages for testing
// GetUnit and frontend.FetchDirectoryDetails.
func InsertSampleDirectoryTree(ctx context.Context, t *testing.T, testDB *DB) {
	t.Helper()

	for _, data := range []struct {
		modulePath, version string
		suffixes            []string
	}{
		{
			"std",
			"v1.13.4",
			[]string{
				"archive/tar",
				"archive/zip",
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
				"archive/tar",
				"archive/zip",
				"cmd/go",
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
		{
			"github.com/hashicorp/vault/api",
			"v1.1.2",
			[]string{""},
		},
		{
			"github.com/hashicorp/vault",
			"v1.1.2",
			[]string{
				"api",
				"builtin/audit/file",
				"builtin/audit/socket",
				"vault/replication",
				"vault/seal/transit",
			},
		},
		{
			"github.com/hashicorp/vault",
			"v1.2.3",
			[]string{
				"builtin/audit/file",
				"builtin/audit/socket",
				"internal/foo",
				"vault/replication",
				"vault/seal/transit",
			},
		},
		{
			"github.com/hashicorp/vault",
			"v1.0.3",
			[]string{
				"api",
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
	} {
		m := sample.Module(data.modulePath, data.version, data.suffixes...)
		MustInsertModule(ctx, t, testDB, m)
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
