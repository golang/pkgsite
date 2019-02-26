// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"reflect"
	"testing"
	"time"

	"golang.org/x/discovery/internal"
)

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
		Module:       module,
		Version:      "v1.0.0",
		Synopsis:     "This is a synopsis",
		License:      "licensename",
		ReadMe:       "readme",
		CommitTime:   time.Now(),
		Packages:     []*internal.Package{},
		Dependencies: []*internal.Version{},
		Dependents:   []*internal.Version{},
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
			teardownTestCase, db := SetupCleanDB(t)
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
	teardownTestCase, db := SetupCleanDB(t)
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
