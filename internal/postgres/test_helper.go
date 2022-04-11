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
	"strings"
	"testing"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/sample"

	// imported to register the postgres migration driver
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// imported to register the file source migration driver
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// recreateDB drops and recreates the database named dbName.
func recreateDB(dbName string) error {
	err := database.DropDB(dbName)
	if err != nil {
		return err
	}

	return database.CreateDB(dbName)
}

// SetupTestDB creates a test database named dbName if it does not already
// exist, and migrates it to the latest schema from the migrations directory.
func SetupTestDB(dbName string) (_ *DB, err error) {
	defer derrors.Wrap(&err, "SetupTestDB(%q)", dbName)

	if err := database.CreateDBIfNotExists(dbName); err != nil {
		return nil, fmt.Errorf("CreateDBIfNotExists(%q): %w", dbName, err)
	}
	if isMigrationError, err := database.TryToMigrate(dbName); err != nil {
		if isMigrationError {
			// failed during migration stage, recreate and try again
			log.Printf("Migration failed for %s: %v, recreating database.", dbName, err)
			if err := recreateDB(dbName); err != nil {
				return nil, fmt.Errorf("recreateDB(%q): %v", dbName, err)
			}
			_, err = database.TryToMigrate(dbName)
		}
		if err != nil {
			return nil, fmt.Errorf("unfixable error migrating database: %v.\nConsider running ./devtools/drop_test_dbs.sh", err)
		}
	}
	db, err := database.Open("pgx", database.DBConnURI(dbName), "test")
	if err != nil {
		return nil, err
	}
	return New(db), nil
}

// ResetTestDB truncates all data from the given test DB.  It should be called
// after every test that mutates the database.
func ResetTestDB(db *DB, t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := database.ResetDB(ctx, db.db); err != nil {
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
//	func Test(t *testing.T) {
//	    db, release := acquire(t)
//	    defer release()
//	}
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
// It also updates the latest-version information for m.
func MustInsertModule(ctx context.Context, t *testing.T, db *DB, m *internal.Module) {
	mustInsertModule(ctx, t, db, m, "", true)
}

func MustInsertModuleGoMod(ctx context.Context, t *testing.T, db *DB, m *internal.Module, goMod string) {
	mustInsertModule(ctx, t, db, m, goMod, true)
}

func MustInsertModuleNotLatest(ctx context.Context, t *testing.T, db *DB, m *internal.Module) {
	mustInsertModule(ctx, t, db, m, "", false)
}

func mustInsertModule(ctx context.Context, t *testing.T, db *DB, m *internal.Module, goMod string, latest bool) {
	t.Helper()
	var lmv *internal.LatestModuleVersions
	if goMod == "-" {
		if err := db.UpdateLatestModuleVersionsStatus(ctx, m.ModulePath, 404); err != nil {
			t.Fatal(err)
		}
	} else if latest {
		lmv = addLatest(ctx, t, db, m.ModulePath, m.Version, goMod)
	}
	if _, err := db.InsertModule(ctx, m, lmv); err != nil {
		t.Fatal(err)
	}
}

func addLatest(ctx context.Context, t *testing.T, db *DB, modulePath, version, modFile string) *internal.LatestModuleVersions {
	if !strings.HasPrefix(strings.TrimSpace(modFile), "module") {
		modFile = "module " + modulePath + "\n" + modFile
	}
	info, err := internal.NewLatestModuleVersions(modulePath, version, version, "", []byte(modFile))
	if err != nil {
		t.Fatal(err)
	}
	lmv, err := db.UpdateLatestModuleVersions(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	return lmv
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
