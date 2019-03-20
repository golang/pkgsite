// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPostgres_ReadAndWriteVersionAndPackages(t *testing.T) {
	var (
		now    = time.Now()
		series = &internal.Series{
			Path:    "myseries",
			Modules: []*internal.Module{},
		}
		module = &internal.Module{
			Path:     "valid_module_name",
			Series:   series,
			Versions: []*internal.Version{},
		}
		testVersion = &internal.Version{
			Module:     module,
			Version:    "v1.0.0",
			License:    "licensename",
			ReadMe:     "readme",
			CommitTime: now,
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Synopsis: "This is a package synopsis",
					Path:     "path/to/foo",
				},
			},
		}
	)

	testCases := []struct {
		name, module, version, pkgpath string
		versionData                    *internal.Version
		wantWriteErrCode               codes.Code
		wantReadErr                    bool
	}{
		{
			name:             "nil_version_write_error",
			module:           "valid_module_name",
			version:          "v1.0.0",
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name:        "valid_test",
			module:      "valid_module_name",
			version:     "v1.0.0",
			pkgpath:     "path/to/foo",
			versionData: testVersion,
		},
		{
			name:        "nonexistent_version_test",
			module:      "valid_module_name",
			version:     "v1.2.3",
			versionData: testVersion,
			wantReadErr: true,
		},
		{
			name:        "nonexistent_module_test",
			module:      "nonexistent_module_name",
			version:     "v1.0.0",
			pkgpath:     "path/to/foo",
			versionData: testVersion,
			wantReadErr: true,
		},
		{
			name: "missing_module",
			versionData: &internal.Version{
				Version:    "v1.0.0",
				Synopsis:   "This is a synopsis",
				License:    "licensename",
				CommitTime: now,
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name: "missing_module_name",
			versionData: &internal.Version{
				Module:     &internal.Module{},
				Version:    "v1.0.0",
				Synopsis:   "This is a synopsis",
				License:    "licensename",
				CommitTime: now,
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name: "missing_version",
			versionData: &internal.Version{
				Module:     module,
				Synopsis:   "This is a synopsis",
				License:    "licensename",
				CommitTime: now,
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name: "empty_commit_time",
			versionData: &internal.Version{
				Module:   module,
				Version:  "v1.0.0",
				Synopsis: "This is a synopsis",
			},
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			if err := db.InsertVersion(tc.versionData); status.Code(err) != tc.wantWriteErrCode {
				t.Errorf("db.InsertVersion(%+v) error: %v, want write error: %v", tc.versionData, err, tc.wantWriteErrCode)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := db.InsertVersion(tc.versionData); status.Code(err) != tc.wantWriteErrCode {
				t.Errorf("db.InsertVersion(%+v) second insert error: %v, want write error: %v", tc.versionData, err, tc.wantWriteErrCode)
			}

			got, err := db.GetVersion(tc.module, tc.version)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("db.GetVersion(%q, %q) error: %v, want read error: %t", tc.module, tc.version, err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("db.GetVersion(%q, %q) = %v, want %v",
					tc.module, tc.version, got, tc.versionData)
			}

			if !tc.wantReadErr && reflect.DeepEqual(*got, *tc.versionData) {
				t.Errorf("db.GetVersion(%q, %q) = %v, want %v",
					tc.module, tc.version, got, tc.versionData)
			}

			gotPkg, err := db.GetPackage(tc.pkgpath, tc.version)
			if tc.versionData == nil || tc.versionData.Packages == nil || tc.pkgpath == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("db.GetPackage(%q, %q) = %v, want %v", tc.pkgpath, tc.version, err, sql.ErrNoRows)
				}
				return
			}

			wantPkg := tc.versionData.Packages[0]
			if err != nil {
				t.Fatalf("db.GetPackage(%q, %q) = %v, want %v", tc.pkgpath, tc.version, gotPkg, wantPkg)
			}

			if gotPkg.Version.Version != tc.versionData.Version {
				t.Errorf("db.GetPackage(%q, %q) version.version = %v, want %v", tc.pkgpath, tc.version, gotPkg.Version.Version, tc.versionData.Version)
			}
			if gotPkg.Version.License != tc.versionData.License {
				t.Errorf("db.GetPackage(%q, %q) version.license = %v, want %v", tc.pkgpath, tc.version, gotPkg.Version.License, tc.versionData.License)

			}

			gotPkg.Version = nil
			if diff := cmp.Diff(*gotPkg, *wantPkg); diff != "" {
				t.Errorf("db.GetPackage(%q, %q) Package mismatch (-want +got):\n%s", tc.pkgpath, tc.version, diff)
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
			ModulePath: "testModule",
			Version:    "v.1.0.0",
			CreatedAt:  now.Add(-10 * time.Minute),
			Source:     internal.VersionSourceProxyIndex,
		},
		&internal.VersionLog{
			ModulePath: "testModule",
			Version:    "v.1.1.0",
			CreatedAt:  now,
			Source:     internal.VersionSourceProxyIndex,
		},
		&internal.VersionLog{
			ModulePath: "testModule/v2",
			Version:    "v.2.0.0",
			CreatedAt:  now,
			Source:     internal.VersionSourceProxyIndex,
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
