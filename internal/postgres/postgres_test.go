// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const testTimeout = 5 * time.Second

var (
	sampleLicenseInfos = []*internal.LicenseInfo{
		{Type: "licensename", FilePath: "bar/LICENSE"},
	}
	sampleLicenses = []*internal.License{
		{LicenseInfo: *sampleLicenseInfos[0], Contents: []byte("Lorem Ipsum")},
	}
)

func TestBulkInsert(t *testing.T) {
	table := "test_bulk_insert"
	for _, tc := range []struct {
		name             string
		columns          []string
		values           []interface{}
		conflictNoAction bool
		wantErr          bool
		wantCount        int
	}{
		{

			name:      "test-one-row",
			columns:   []string{"colA"},
			values:    []interface{}{"valueA"},
			wantCount: 1,
		},
		{

			name:      "test-multiple-rows",
			columns:   []string{"colA"},
			values:    []interface{}{"valueA1", "valueA2", "valueA3"},
			wantCount: 3,
		},
		{

			name:    "test-invalid-column-name",
			columns: []string{"invalid_col"},
			values:  []interface{}{"valueA"},
			wantErr: true,
		},
		{

			name:    "test-mismatch-num-cols-and-vals",
			columns: []string{"colA", "colB"},
			values:  []interface{}{"valueA1", "valueB1", "valueA2"},
			wantErr: true,
		},
		{

			name:             "test-conflict-no-action-true",
			columns:          []string{"colA"},
			values:           []interface{}{"valueA", "valueA"},
			conflictNoAction: true,
			wantCount:        1,
		},
		{

			name:             "test-conflict-no-action-false",
			columns:          []string{"colA"},
			values:           []interface{}{"valueA", "valueA"},
			conflictNoAction: false,
			wantErr:          true,
		},
		{

			// This should execute the statement
			// INSERT INTO series (path) VALUES ('''); TRUNCATE series CASCADE;)');
			// which will insert a row with path value:
			// '); TRUNCATE series CASCADE;)
			// Rather than the statement
			// INSERT INTO series (path) VALUES (''); TRUNCATE series CASCADE;));
			// which would truncate most tables in the database.
			name:             "test-sql-injection",
			columns:          []string{"colA"},
			values:           []interface{}{fmt.Sprintf("''); TRUNCATE %s CASCADE;))", table)},
			conflictNoAction: true,
			wantCount:        1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			createQuery := fmt.Sprintf(`CREATE TABLE %s (
					colA TEXT NOT NULL,
					colB TEXT,
					PRIMARY KEY (colA)
				);`, table)
			if _, err := db.ExecContext(ctx, createQuery); err != nil {
				t.Fatalf("db.ExecContext(ctx, %q): %v", createQuery, err)
			}
			defer func() {
				dropTableQuery := fmt.Sprintf("DROP TABLE %s;", table)
				if _, err := db.ExecContext(ctx, dropTableQuery); err != nil {
					t.Fatalf("db.ExecContext(ctx, %q): %v", dropTableQuery, err)
				}
			}()

			if err := db.Transact(func(tx *sql.Tx) error {
				return bulkInsert(ctx, tx, table, tc.columns, tc.values, tc.conflictNoAction)
			}); tc.wantErr && err == nil || !tc.wantErr && err != nil {
				t.Errorf("db.Transact: %v | wantErr = %t", err, tc.wantErr)
			}

			if tc.wantCount != 0 {
				var count int
				query := "SELECT COUNT(*) FROM " + table
				row := db.QueryRow(query)
				err := row.Scan(&count)
				if err != nil {
					t.Fatalf("db.QueryRow(%q): %v", query, err)
				}
				if count != tc.wantCount {
					t.Errorf("db.QueryRow(%q) = %d; want = %d", query, count, tc.wantCount)
				}
			}
		})
	}

}

func TestPostgres_ReadAndWriteVersionAndPackages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	var (
		now    = NowTruncated()
		series = &internal.Series{
			Path:    "myseries",
			Modules: []*internal.Module{},
		}
		module = &internal.Module{
			Path:     "github.com/valid_module_name",
			Series:   series,
			Versions: []*internal.Version{},
		}
		testVersion = &internal.Version{
			Module:     module,
			Version:    "v1.0.0",
			ReadMe:     []byte("readme"),
			CommitTime: now,
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Synopsis: "This is a package synopsis",
					Path:     "path.to/foo",
				},
			},
			VersionType: internal.VersionTypeRelease,
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
			module:           "github.com/valid_module_name",
			version:          "v1.0.0",
			wantWriteErrCode: codes.InvalidArgument,
			wantReadErr:      true,
		},
		{
			name:        "valid_test",
			module:      "github.com/valid_module_name",
			version:     "v1.0.0",
			pkgpath:     "path.to/foo",
			versionData: testVersion,
		},
		{
			name:        "nonexistent_version_test",
			module:      "github.com/valid_module_name",
			version:     "v1.2.3",
			versionData: testVersion,
			wantReadErr: true,
		},
		{
			name:        "nonexistent_module_test",
			module:      "nonexistent_module_name",
			version:     "v1.0.0",
			pkgpath:     "path.to/foo",
			versionData: testVersion,
			wantReadErr: true,
		},
		{
			name: "missing_module",
			versionData: &internal.Version{
				Version:    "v1.0.0",
				Synopsis:   "This is a synopsis",
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

			if err := db.InsertVersion(ctx, tc.versionData, sampleLicenses); status.Code(err) != tc.wantWriteErrCode {
				t.Errorf("db.InsertVersion(ctx, %+v) error: %v, want write error: %v", tc.versionData, err, tc.wantWriteErrCode)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := db.InsertVersion(ctx, tc.versionData, sampleLicenses); status.Code(err) != tc.wantWriteErrCode {
				t.Errorf("db.InsertVersion(ctx, %+v) second insert error: %v, want write error: %v", tc.versionData, err, tc.wantWriteErrCode)
			}

			got, err := db.GetVersion(ctx, tc.module, tc.version)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("db.GetVersion(ctx, %q, %q) error: %v, want read error: %t", tc.module, tc.version, err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("db.GetVersion(ctx, %q, %q) = %v, want %v",
					tc.module, tc.version, got, tc.versionData)
			}

			if diff := cmp.Diff(tc.versionData, got, cmpopts.IgnoreFields(internal.Version{},
				"CreatedAt", "UpdatedAt", "Module.Series", "VersionType", "Packages", "Module.Versions")); !tc.wantReadErr && diff != "" {
				t.Errorf("db.GetVersion(ctx, %q, %q) = %v, want %v | diff is %v",
					tc.module, tc.version, got, tc.versionData, diff)
			}

			gotPkg, err := db.GetPackage(ctx, tc.pkgpath, tc.version)
			if tc.versionData == nil || tc.versionData.Packages == nil || tc.pkgpath == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("db.GetPackage(ctx, %q, %q) = %v, want %v", tc.pkgpath, tc.version, err, sql.ErrNoRows)
				}
				return
			} else {
				if err != nil {
					t.Errorf("db.GetPackage(ctx, %q, %q): %v", tc.pkgpath, tc.version, err)
				}
			}

			wantPkg := tc.versionData.Packages[0]
			if err != nil {
				t.Fatalf("db.GetPackage(ctx, %q, %q) = %v, want %v", tc.pkgpath, tc.version, gotPkg, wantPkg)
			}

			if gotPkg.Version.Version != tc.versionData.Version {
				t.Errorf("db.GetPackage(ctx, %q, %q) version.version = %v, want %v", tc.pkgpath, tc.version, gotPkg.Version.Version, tc.versionData.Version)
			}

			if diff := cmp.Diff(gotPkg, wantPkg, cmpopts.IgnoreFields(internal.Package{}, "Version")); diff != "" {
				t.Errorf("db.GetPackage(ctx, %q, %q) Package mismatch (-want +got):\n%s", tc.pkgpath, tc.version, diff)
			}
		})
	}
}

func TestPostgres_GetLatestPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownTestCase, db := SetupCleanDB(t)
	defer teardownTestCase(t)
	var (
		now = NowTruncated()
		pkg = &internal.Package{
			Path:     "path.to/foo/bar",
			Name:     "bar",
			Synopsis: "This is a package synopsis",
			Licenses: sampleLicenseInfos,
		}
		series = &internal.Series{
			Path: "myseries",
		}
		module = &internal.Module{
			Path:   "path.to/foo",
			Series: series,
		}
		testVersions = []*internal.Version{
			&internal.Version{
				Module:      module,
				Version:     "v1.0.0-alpha.1",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypePrerelease,
			},
			&internal.Version{
				Module:      module,
				Version:     "v1.0.0",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypeRelease,
			},
			&internal.Version{
				Module:      module,
				Version:     "v1.0.0-20190311183353-d8887717615a",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg},
				VersionType: internal.VersionTypePseudo,
			},
		}
	)

	testCases := []struct {
		name, path  string
		versions    []*internal.Version
		wantPkg     *internal.Package
		wantReadErr bool
	}{
		{
			name:     "want_second_package",
			path:     pkg.Path,
			versions: testVersions,
			wantPkg: &internal.Package{
				Name:     pkg.Name,
				Path:     pkg.Path,
				Synopsis: pkg.Synopsis,
				Licenses: sampleLicenseInfos,
				Version: &internal.Version{
					CreatedAt: testVersions[1].CreatedAt,
					UpdatedAt: testVersions[1].UpdatedAt,
					Module: &internal.Module{
						Path: module.Path,
					},
					Version:    testVersions[1].Version,
					Synopsis:   testVersions[1].Synopsis,
					CommitTime: testVersions[1].CommitTime,
				},
			},
		},
		{
			name:        "empty_path",
			path:        "",
			wantReadErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.versions {
				if err := db.InsertVersion(ctx, v, sampleLicenses); err != nil {
					t.Errorf("db.InsertVersion(ctx, %v): %v", v, err)
				}
			}

			gotPkg, err := db.GetLatestPackage(ctx, tc.path)
			if (err != nil) != tc.wantReadErr {
				t.Errorf("db.GetLatestPackage(ctx, %q): %v", tc.path, err)
			}

			if diff := cmp.Diff(gotPkg, tc.wantPkg, cmpopts.IgnoreFields(internal.Package{}, "Version.UpdatedAt", "Version.CreatedAt")); diff != "" {
				t.Errorf("db.GetLatestPackage(ctx, %q) mismatch (-got +want):\n%s",
					tc.path, diff)
			}
		})
	}
}

func TestPostgres_GetLatestPackageForPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownTestCase, db := SetupCleanDB(t)
	defer teardownTestCase(t)
	var (
		now  = NowTruncated()
		pkg1 = &internal.Package{
			Path:     "path.to/foo/bar",
			Name:     "bar",
			Synopsis: "This is a package synopsis",
			Licenses: sampleLicenseInfos,
		}
		pkg2 = &internal.Package{
			Path:     "path2.to/foo/bar2",
			Name:     "bar2",
			Synopsis: "This is another package synopsis",
		}
		series = &internal.Series{
			Path: "myseries",
		}
		module1 = &internal.Module{
			Path:   "path2.to/foo",
			Series: series,
		}
		module2 = &internal.Module{
			Path:   "path2.to/foo",
			Series: series,
		}
		testVersions = []*internal.Version{
			&internal.Version{
				Module:      module1,
				Version:     "v1.0.0-alpha.1",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg1},
				VersionType: internal.VersionTypePrerelease,
			},
			&internal.Version{
				Module:      module1,
				Version:     "v1.0.0",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg1},
				VersionType: internal.VersionTypeRelease,
			},
			&internal.Version{
				Module:      module2,
				Version:     "v1.0.0-20190311183353-d8887717615a",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg2},
				VersionType: internal.VersionTypePseudo,
			},
			&internal.Version{
				Module:      module2,
				Version:     "v1.0.1-beta",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				Packages:    []*internal.Package{pkg2},
				VersionType: internal.VersionTypePrerelease,
			},
		}
	)

	tc := struct {
		paths       []string
		versions    []*internal.Version
		wantPkgs    []*internal.Package
		wantReadErr bool
	}{
		paths:    []string{pkg1.Path, pkg2.Path},
		versions: testVersions,
		wantPkgs: []*internal.Package{
			&internal.Package{
				Name:     pkg1.Name,
				Path:     pkg1.Path,
				Synopsis: pkg1.Synopsis,
				Licenses: pkg1.Licenses,
				Version: &internal.Version{
					CreatedAt: testVersions[1].CreatedAt,
					UpdatedAt: testVersions[1].UpdatedAt,
					Module: &internal.Module{
						Path: module1.Path,
					},
					Version:    testVersions[1].Version,
					Synopsis:   testVersions[1].Synopsis,
					CommitTime: testVersions[1].CommitTime,
				},
			},
			&internal.Package{
				Name:     pkg2.Name,
				Path:     pkg2.Path,
				Synopsis: pkg2.Synopsis,
				Licenses: pkg2.Licenses,
				Version: &internal.Version{
					CreatedAt: testVersions[3].CreatedAt,
					UpdatedAt: testVersions[3].UpdatedAt,
					Module: &internal.Module{
						Path: module1.Path,
					},
					Version:    testVersions[3].Version,
					Synopsis:   testVersions[3].Synopsis,
					CommitTime: testVersions[3].CommitTime,
				},
			},
		},
	}

	for _, v := range tc.versions {
		if err := db.InsertVersion(ctx, v, sampleLicenses); err != nil {
			t.Errorf("db.InsertVersion(ctx, %q %q): %v", v.Module.Path, v.Version, err)
		}
	}

	gotPkgs, err := db.GetLatestPackageForPaths(ctx, tc.paths)
	if (err != nil) != tc.wantReadErr {
		t.Errorf("db.GetLatestPackageForPaths(ctx, %q): %v", tc.paths, err)
	}

	if diff := cmp.Diff(gotPkgs, tc.wantPkgs); diff != "" {
		t.Errorf("db.GetLatestPackageForPaths(ctx, %q) mismatch (-got +want):\n%s", tc.paths, diff)
	}
}

func TestPostgress_InsertVersionLogs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownTestCase, db := SetupCleanDB(t)
	defer teardownTestCase(t)

	now := NowTruncated().UTC()
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

	if err := db.InsertVersionLogs(ctx, newVersions); err != nil {
		t.Errorf("db.InsertVersionLogs(ctx, newVersions) error: %v", err)
	}

	dbTime, err := db.LatestProxyIndexUpdate(ctx)
	if err != nil {
		t.Errorf("db.LatestProxyIndexUpdate error: %v", err)
	}

	// Since now is already truncated to Microsecond precision, we should get
	// back the exact same time.
	if !dbTime.Equal(now) {
		t.Errorf("db.LatestProxyIndexUpdate(ctx) = %v, want %v", dbTime, now)
	}
}

func TestPostgres_prefixZeroes(t *testing.T) {
	testCases := []struct {
		name, input, want string
		wantErr           bool
	}{
		{
			name:  "add_16_zeroes",
			input: "1111",
			want:  "00000000000000001111",
		},
		{
			name:  "add_nothing_exactly_20",
			input: "11111111111111111111",
			want:  "11111111111111111111",
		},
		{
			name:  "add_20_zeroes_empty_string",
			input: "",
			want:  "00000000000000000000",
		},
		{
			name:    "input_longer_than_20_char",
			input:   "123456789123456789123456789",
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := prefixZeroes(tc.input); got != tc.want {
				t.Errorf("prefixZeroes(%v) = %v, want %v, err = %v, wantErr = %v", tc.input, got, tc.want, err, tc.wantErr)
			}
		})
	}
}

func TestPostgres_isNum(t *testing.T) {
	testCases := []struct {
		name, input string
		want        bool
	}{
		{
			name:  "all_numbers",
			input: "1111",
			want:  true,
		},
		{
			name:  "number_letter_mix",
			input: "111111asdf1a1111111asd",
			want:  false,
		},
		{
			name:  "empty_string",
			input: "",
			want:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNum(tc.input); got != tc.want {
				t.Errorf("isNum(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestPostgres_padPrerelease(t *testing.T) {
	testCases := []struct {
		name, input, want string
		wantErr           bool
	}{
		{
			name:  "pad_one_field",
			input: "v1.0.0-alpha.1",
			want:  "alpha.00000000000000000001",
		},
		{
			name:  "no_padding",
			input: "v1.0.0-beta",
			want:  "beta",
		},
		{
			name:  "pad_two_fields",
			input: "v1.0.0-gamma.11.theta.2",
			want:  "gamma.00000000000000000011.theta.00000000000000000002",
		},
		{
			name:  "empty_string",
			input: "v1.0.0",
			want:  "~",
		},
		{
			name:    "num_field_longer_than_20_char",
			input:   "v1.0.0-alpha.123456789123456789123456789",
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := padPrerelease(tc.input); (err != nil) == tc.wantErr && got != tc.want {
				t.Errorf("padPrerelease(%v) = %v, want %v, err = %v, wantErr = %v", tc.input, got, tc.want, err, tc.wantErr)
			}
		})
	}
}

func TestPostgres_GetTaggedAndPseudoVersionsForPackageSeries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		now  = NowTruncated()
		pkg1 = &internal.Package{
			Path:     "path.to/foo/bar",
			Name:     "bar",
			Synopsis: "This is a package synopsis",
			Suffix:   "bar",
		}
		pkg2 = &internal.Package{
			Path:     "path.to/foo/v2/bar",
			Name:     "bar",
			Synopsis: "This is another package synopsis",
			Suffix:   "bar",
		}
		pkg3 = &internal.Package{
			Path:     "path.to/some/thing/else",
			Name:     "else",
			Synopsis: "something else's package synopsis",
			Suffix:   "else",
		}
		series = &internal.Series{
			Path: "path.to/foo",
		}
		module1 = &internal.Module{
			Path:   "path.to/foo",
			Series: series,
		}
		module2 = &internal.Module{
			Path:   "path.to/foo/v2",
			Series: series,
		}
		module3 = &internal.Module{
			Path:   "path.to/some/thing",
			Series: series,
		}
		testVersions = []*internal.Version{
			&internal.Version{
				Module:      module3,
				Version:     "v3.0.0",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg3},
				VersionType: internal.VersionTypeRelease,
			},
			&internal.Version{
				Module:      module1,
				Version:     "v1.0.0-alpha.1",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg1},
				VersionType: internal.VersionTypePrerelease,
			},
			&internal.Version{
				Module:      module1,
				Version:     "v1.0.0",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg1},
				VersionType: internal.VersionTypeRelease,
			},
			&internal.Version{
				Module:      module2,
				Version:     "v2.0.1-beta",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg2},
				VersionType: internal.VersionTypePrerelease,
			},
			&internal.Version{
				Module:      module2,
				Version:     "v2.1.0",
				CommitTime:  now,
				Packages:    []*internal.Package{pkg2},
				VersionType: internal.VersionTypeRelease,
			},
		}
	)

	testCases := []struct {
		name, path         string
		numPseudo          int
		versions           []*internal.Version
		wantTaggedVersions []*internal.Version
	}{
		{
			name:      "want_releases_and_prereleases_only",
			path:      "path.to/foo/bar",
			numPseudo: 12,
			versions:  testVersions,
			wantTaggedVersions: []*internal.Version{
				&internal.Version{
					Module:     module2,
					Version:    "v2.1.0",
					Synopsis:   pkg2.Synopsis,
					CommitTime: now,
					Packages: []*internal.Package{
						&internal.Package{
							Path: pkg2.Path,
							Name: pkg2.Name,
						},
					},
				},
				&internal.Version{
					Module:     module2,
					Version:    "v2.0.1-beta",
					Synopsis:   pkg2.Synopsis,
					CommitTime: now,
					Packages: []*internal.Package{
						&internal.Package{
							Path: pkg2.Path,
							Name: pkg2.Name,
						},
					},
				},
				&internal.Version{
					Module:     module1,
					Version:    "v1.0.0",
					Synopsis:   pkg1.Synopsis,
					CommitTime: now,
					Packages: []*internal.Package{
						&internal.Package{
							Path: pkg1.Path,
							Name: pkg1.Name,
						},
					},
				},
				&internal.Version{
					Module:     module1,
					Version:    "v1.0.0-alpha.1",
					Synopsis:   pkg1.Synopsis,
					CommitTime: now,
					Packages: []*internal.Package{
						&internal.Package{
							Path: pkg1.Path,
							Name: pkg1.Name,
						},
					},
				},
			},
		},
		{
			name:     "want_zero_results_in_non_empty_db",
			path:     "not.a/real/path",
			versions: testVersions,
		},
		{
			name: "want_zero_results_in_empty_db",
			path: "not.a/real/path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			wantPseudoVersions := []*internal.Version{}
			for i := 0; i < tc.numPseudo; i++ {
				v := &internal.Version{
					Module: module1,
					// %02d makes a string that is a width of 2 and left pads with zeroes
					Version:     fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", i+1),
					CommitTime:  now,
					Packages:    []*internal.Package{pkg1},
					VersionType: internal.VersionTypePseudo,
				}
				if err := db.InsertVersion(ctx, v, nil); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}

				// GetPseudoVersions should only return the 10 most recent pseudo versions,
				// if there are more than 10 in the database
				if i < 10 {
					wantPseudoVersions = append(wantPseudoVersions, &internal.Version{
						Module:     module1,
						Version:    fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", tc.numPseudo-i),
						Synopsis:   pkg1.Synopsis,
						CommitTime: now,
						Packages: []*internal.Package{
							&internal.Package{
								Path: pkg1.Path,
								Name: pkg1.Name,
							},
						},
					})
				}
			}

			for _, v := range tc.versions {
				if err := db.InsertVersion(ctx, v, nil); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}
			}

			var (
				got []*internal.Version
				err error
			)

			got, err = db.GetPseudoVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatalf("db.GetPseudoVersionsForPackageSeries(%q) error: %v", tc.path, err)
			}

			if len(got) != len(wantPseudoVersions) {
				t.Fatalf("db.GetPseudoVersionsForPackageSeries(%q) returned list of length %v, wanted %v", tc.path, len(got), len(wantPseudoVersions))
			}

			for i, v := range got {
				if diff := cmp.Diff(wantPseudoVersions[i], v, cmpopts.IgnoreFields(internal.Version{}, "CreatedAt", "UpdatedAt", "Module.Series", "VersionType", "Packages", "Module.Versions")); diff != "" {
					t.Errorf("db.GetPseudoVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
				}
			}

			got, err = db.GetTaggedVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatalf("db.GetTaggedVersionsForPackageSeries(%q) error: %v", tc.path, err)
			}

			if len(got) != len(tc.wantTaggedVersions) {
				t.Fatalf("db.GetTaggedVersionsForPackageSeries(%q) returned list of length %v, wanted %v", tc.path, len(got), len(tc.wantTaggedVersions))
			}

			for i, v := range got {

				if diff := cmp.Diff(tc.wantTaggedVersions[i], v, cmpopts.IgnoreFields(internal.Version{},
					"CreatedAt", "UpdatedAt", "Module.Series", "VersionType", "Packages", "Module.Versions")); diff != "" {
					t.Errorf("db.GetTaggedVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
				}
			}
		})
	}
}

func TestMajorMinorPatch(t *testing.T) {
	for _, tc := range []struct {
		version                         string
		wantMajor, wantMinor, wantPatch int
	}{
		{
			version:   "v1.5.2",
			wantMajor: 1,
			wantMinor: 5,
			wantPatch: 2,
		},
		{
			version:   "v1.5.2+incompatible",
			wantMajor: 1,
			wantMinor: 5,
			wantPatch: 2,
		},
		{
			version:   "v1.5.2-alpha+buildtag",
			wantMajor: 1,
			wantMinor: 5,
			wantPatch: 2,
		},
	} {
		t.Run(tc.version, func(t *testing.T) {
			gotMajor, err := major(tc.version)
			if err != nil {
				t.Errorf("major(%q): %v", tc.version, err)
			}
			if gotMajor != tc.wantMajor {
				t.Errorf("major(%q) = %d, want = %d", tc.version, gotMajor, tc.wantMajor)
			}

			gotMinor, err := minor(tc.version)
			if err != nil {
				t.Errorf("minor(%q): %v", tc.version, err)
			}
			if gotMinor != tc.wantMinor {
				t.Errorf("minor(%q) = %d, want = %d", tc.version, gotMinor, tc.wantMinor)
			}

			gotPatch, err := patch(tc.version)
			if err != nil {
				t.Errorf("patch(%q): %v", tc.version, err)
			}
			if gotPatch != tc.wantPatch {
				t.Errorf("patch(%q) = %d, want = %d", tc.version, gotPatch, tc.wantPatch)
			}
		})
	}
}

func TestGetVersionForPackage(t *testing.T) {
	var (
		now    = NowTruncated()
		series = &internal.Series{
			Path:    "myseries",
			Modules: []*internal.Module{},
		}
		module = &internal.Module{
			Path:     "test.module",
			Series:   series,
			Versions: []*internal.Version{},
		}
		testVersion = &internal.Version{
			Module:      module,
			Version:     "v1.0.0",
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
			Packages: []*internal.Package{
				&internal.Package{
					Name:     "foo",
					Synopsis: "This is a package synopsis",
					Path:     "test.module/foo",
					Licenses: sampleLicenseInfos,
				},
				&internal.Package{
					Name:     "testmodule",
					Synopsis: "This is a package synopsis",
					Path:     "test.module",
				},
			},
		}
	)

	for _, tc := range []struct {
		name, path, version string
		wantVersion         *internal.Version
	}{
		{
			name:        "version_with_multi_packages",
			path:        "test.module/foo",
			version:     testVersion.Version,
			wantVersion: testVersion,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			if err := db.InsertVersion(ctx, tc.wantVersion, sampleLicenses); err != nil {
				t.Errorf("db.InsertVersion(ctx, %q %q): %v", tc.path, tc.version, err)
			}

			got, err := db.GetVersionForPackage(ctx, tc.path, tc.version)
			if err != nil {
				t.Errorf("db.GetVersionForPackage(ctx, %q, %q): %v", tc.path, tc.version, err)
			}
			if diff := cmp.Diff(tc.wantVersion, got,
				cmpopts.IgnoreFields(internal.Version{}, "Module.Versions", "Module.Series", "VersionType")); diff != "" {
				t.Errorf("db.GetVersionForPackage(ctx, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}
