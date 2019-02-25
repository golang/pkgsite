package postgres

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/discovery/internal"
)

var (
	user       = getEnv("GO_DISCOVERY_DATABASE_TEST_USER", "postgres")
	password   = getEnv("GO_DISCOVERY_DATABASE_TEST_PASSWORD", "")
	host       = getEnv("GO_DISCOVERY_DATABASE_TEST_HOST", "localhost")
	testdbname = getEnv("GO_DISCOVERY_DATABASE_TEST_NAME", "discovery-database-test")
	testdb     = fmt.Sprintf("user=%s host=%s dbname=%s sslmode=disable", user, host, testdbname)
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func setupCleanDB(t *testing.T) (func(t *testing.T), *DB) {
	t.Helper()
	db, err := Open(testdb)
	if err != nil {
		t.Fatalf("Open(%q), error: %v", testdb, err)
	}
	cleanup := func(t *testing.T) {
		db.Exec(`TRUNCATE version_logs;`)     // truncates version_logs
		db.Exec(`TRUNCATE versions CASCADE;`) // truncates versions and any tables that use versions as a foreign key.
	}
	return cleanup, db
}

func TestPostgres_ReadAndWriteVersion(t *testing.T) {
	var series = &internal.Series{
		Name:    "myseries",
		Modules: []*internal.Module{},
	}

	var module = &internal.Module{
		Name:     "valid_module_name",
		Series:   series,
		Versions: []*internal.Version{},
	}

	var testVersion = &internal.Version{
		Module:          module,
		Version:         "v1.0.0",
		Synopsis:        "This is a synopsis",
		LicenseName:     "licensename",
		LicenseContents: "licensecontents",
		ReadMe:          "readme",
		CommitTime:      time.Now(),
		Packages:        []*internal.Package{},
		Dependencies:    []*internal.Version{},
		Dependents:      []*internal.Version{},
	}

	testCases := []struct {
		name, moduleName, version string
		versionData               *internal.Version
		wantReadErr, wantWriteErr bool
	}{
		{
			name:         "nil_version_write_error",
			moduleName:   "valid_module_name",
			version:      "v1.0.0",
			wantReadErr:  true,
			wantWriteErr: true,
		},
		{
			name:        "valid_test",
			moduleName:  "valid_module_name",
			version:     "v1.0.0",
			versionData: testVersion,
		},
		{
			name:        "nonexistent_version_test",
			moduleName:  "valid_module_name",
			version:     "v1.2.3",
			versionData: testVersion,
			wantReadErr: true,
		},
		{
			name:        "nonexistent_module_test",
			moduleName:  "nonexistent_module_name",
			version:     "v1.0.0",
			versionData: testVersion,
			wantReadErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := setupCleanDB(t)
			defer teardownTestCase(t)

			if err := db.InsertVersion(tc.versionData); tc.wantWriteErr != (err != nil) {
				t.Errorf("db.InsertVersion(%+v) error: %v, want write error: %t", tc.versionData, err, tc.wantWriteErr)
			}

			// Test that insertion of duplicate primary key fails when the first insert worked
			if err := db.InsertVersion(tc.versionData); err == nil {
				t.Errorf("db.InsertVersion(%+v) on duplicate version did not produce error", testVersion)
			}

			got, err := db.GetVersion(tc.moduleName, tc.version)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("db.GetVersion(%q, %q) error: %v, want read error: %t", tc.moduleName, tc.version, err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("db.GetVersion(%q, %q) = %v, want %v",
					tc.moduleName, tc.version, got, tc.versionData)
			}

			if !tc.wantReadErr && reflect.DeepEqual(*got, *tc.versionData) {
				t.Errorf("db.GetVersion(%q, %q) = %v, want %v",
					tc.moduleName, tc.version, got, tc.versionData)
			}
		})
	}
}

func TestPostgress_InsertVersionLogs(t *testing.T) {
	teardownTestCase, db := setupCleanDB(t)
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
