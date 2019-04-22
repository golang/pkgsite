// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
)

const (
	testTimeout         = 5 * time.Second
	sampleSeriesPath    = "github.com/valid_module_name"
	sampleModulePath    = "github.com/valid_module_name"
	sampleVersionString = "v1.0.0"
)

var (
	now                = NowTruncated()
	sampleLicenseInfos = []*internal.LicenseInfo{
		{Type: "licensename", FilePath: "bar/LICENSE"},
	}
	sampleLicenses = []*internal.License{
		{LicenseInfo: *sampleLicenseInfos[0], Contents: []byte("Lorem Ipsum")},
	}
)

func sampleVersion(mutators ...func(*internal.Version)) *internal.Version {
	v := &internal.Version{
		VersionInfo: internal.VersionInfo{
			SeriesPath:  sampleSeriesPath,
			ModulePath:  sampleModulePath,
			Version:     sampleVersionString,
			ReadMe:      []byte("readme"),
			CommitTime:  now,
			VersionType: internal.VersionTypeRelease,
		},
		Packages: []*internal.Package{
			&internal.Package{
				Name:     "foo",
				Synopsis: "This is a package synopsis",
				Path:     "path.to/foo",
				Imports: []*internal.Import{
					&internal.Import{
						Name: "bar",
						Path: "path/to/bar",
					},
					&internal.Import{
						Name: "fmt",
						Path: "fmt",
					},
				},
			},
		},
	}
	for _, mut := range mutators {
		mut(v)
	}
	return v
}

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

	testCases := []struct {
		name string

		version *internal.Version

		// identifiers to use for fetch
		getVersion, getModule, getPkg string

		// error conditions
		wantWriteErrType derrors.ErrorType
		wantReadErr      bool
	}{
		{
			name:       "valid test",
			version:    sampleVersion(),
			getModule:  sampleModulePath,
			getVersion: sampleVersionString,
			getPkg:     "path.to/foo",
		},
		{
			name:             "nil version write error",
			getModule:        sampleModulePath,
			getVersion:       sampleVersionString,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
		{
			name:        "nonexistent version",
			version:     sampleVersion(),
			getModule:   sampleModulePath,
			getVersion:  "v1.2.3",
			wantReadErr: true,
		},
		{
			name:        "nonexistent module",
			version:     sampleVersion(),
			getModule:   "nonexistent_module_name",
			getVersion:  "v1.0.0",
			getPkg:      "path.to/foo",
			wantReadErr: true,
		},
		{
			name: "missing module path",
			version: sampleVersion(func(v *internal.Version) {
				v.ModulePath = ""
			}),
			getVersion:       sampleVersionString,
			getModule:        sampleModulePath,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
		{
			name: "missing version",
			version: sampleVersion(func(v *internal.Version) {
				v.Version = ""
			}),
			getVersion:       sampleVersionString,
			getModule:        sampleModulePath,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
		{
			name: "empty commit time",
			version: sampleVersion(func(v *internal.Version) {
				v.CommitTime = time.Time{}
			}),
			getVersion:       sampleVersionString,
			getModule:        sampleModulePath,
			wantWriteErrType: derrors.InvalidArgumentType,
			wantReadErr:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			if err := db.InsertVersion(ctx, tc.version, sampleLicenses); derrors.Type(err) != tc.wantWriteErrType {
				t.Errorf("db.InsertVersion(ctx, %+v) error: %v, want write error: %v", tc.version, err, tc.wantWriteErrType)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := db.InsertVersion(ctx, tc.version, sampleLicenses); derrors.Type(err) != tc.wantWriteErrType {
				t.Errorf("db.InsertVersion(ctx, %+v) second insert error: %v, want write error: %v", tc.version, err, tc.wantWriteErrType)
			}

			got, err := db.GetVersion(ctx, tc.getModule, tc.getVersion)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("db.GetVersion(ctx, %q, %q) error: %v, want read error: %t", tc.getModule, tc.getVersion, err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("db.GetVersion(ctx, %q, %q) = %v, want %v", tc.getModule, tc.getVersion, got, tc.version)
			}

			if tc.version != nil {
				if diff := cmp.Diff(&tc.version.VersionInfo, got, cmpopts.IgnoreFields(internal.VersionInfo{},
					"VersionType")); !tc.wantReadErr && diff != "" {
					t.Errorf("db.GetVersion(ctx, %q, %q) mismatch (-want +got):\n%s", tc.getModule, tc.getVersion, diff)
				}
			}

			gotPkg, err := db.GetPackage(ctx, tc.getPkg, tc.getVersion)
			if tc.version == nil || tc.version.Packages == nil || tc.getPkg == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("db.GetPackage(ctx, %q, %q) = %v, want %v", tc.getPkg, tc.getVersion, err, sql.ErrNoRows)
				}
				return
			}
			if err != nil {
				t.Errorf("db.GetPackage(ctx, %q, %q): %v", tc.getPkg, tc.getVersion, err)
			}

			wantPkg := tc.version.Packages[0]
			if err != nil {
				t.Fatalf("db.GetPackage(ctx, %q, %q) = %v, want %v", tc.getPkg, tc.getVersion, gotPkg, wantPkg)
			}

			if gotPkg.VersionInfo.Version != tc.version.Version {
				t.Errorf("db.GetPackage(ctx, %q, %q) version.version = %v, want %v", tc.getPkg, tc.getVersion, gotPkg.VersionInfo.Version, tc.version.Version)
			}

			if diff := cmp.Diff(wantPkg, &gotPkg.Package, cmpopts.IgnoreFields(internal.Package{}, "Imports")); diff != "" {
				t.Errorf("db.GetPackage(%q, %q) Package mismatch (-want +got):\n%s", tc.getPkg, tc.getVersion, diff)
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
		seriesPath   = "path.to/foo"
		modulePath   = "path.to/foo"
		testVersions = []*internal.Version{
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath,
					Version:     "v1.0.0-alpha.1",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypePrerelease,
				},
				Packages: []*internal.Package{pkg},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath,
					Version:     "v1.0.0",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
				},
				Packages: []*internal.Package{pkg},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath,
					Version:     "v1.0.0-20190311183353-d8887717615a",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypePseudo,
				},
				Packages: []*internal.Package{pkg},
			},
		}
	)

	testCases := []struct {
		name, path  string
		versions    []*internal.Version
		wantPkg     *internal.VersionedPackage
		wantReadErr bool
	}{
		{
			name:     "want_second_package",
			path:     pkg.Path,
			versions: testVersions,
			wantPkg: &internal.VersionedPackage{
				Package: internal.Package{
					Name:     pkg.Name,
					Path:     pkg.Path,
					Synopsis: pkg.Synopsis,
					Licenses: sampleLicenseInfos,
				},
				VersionInfo: internal.VersionInfo{
					SeriesPath: seriesPath,
					ModulePath: testVersions[1].ModulePath,
					Version:    testVersions[1].Version,
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

			if diff := cmp.Diff(gotPkg, tc.wantPkg); diff != "" {
				t.Errorf("db.GetLatestPackage(ctx, %q) mismatch (-got +want):\n%s",
					tc.path, diff)
			}
		})
	}
}

func TestPostgres_GetImportsAndImportedBy(t *testing.T) {
	var (
		now  = NowTruncated()
		pkg1 = &internal.Package{
			Name:     "bar",
			Path:     "path.to/foo/bar",
			Synopsis: "This is a package synopsis",
		}
		pkg2 = &internal.Package{
			Name:     "bar2",
			Path:     "path2.to/foo/bar2",
			Synopsis: "This is another package synopsis",
			Imports: []*internal.Import{
				&internal.Import{
					Name: pkg1.Name,
					Path: pkg1.Path,
				},
			},
		}
		pkg3 = &internal.Package{
			Name:     "bar3",
			Path:     "path3.to/foo/bar3",
			Synopsis: "This is another package synopsis",
			Imports: []*internal.Import{
				&internal.Import{
					Name: pkg2.Name,
					Path: pkg2.Path,
				},
				&internal.Import{
					Name: pkg1.Name,
					Path: pkg1.Path,
				},
			},
		}
		seriesPath   = "myseries"
		modulePath1  = "path.to/foo"
		modulePath2  = "path2.to/foo"
		modulePath3  = "path3.to/foo"
		testVersions = []*internal.Version{
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath1,
					Version:     "v1.1.0",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypePrerelease,
				},
				Packages: []*internal.Package{pkg1},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath2,
					Version:     "v1.2.0",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypePseudo,
				},
				Packages: []*internal.Package{pkg2},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath3,
					Version:     "v1.3.0",
					ReadMe:      []byte("readme"),
					CommitTime:  now,
					VersionType: internal.VersionTypePseudo,
				},
				Packages: []*internal.Package{pkg3},
			},
		}
	)

	for _, tc := range []struct {
		path, version  string
		wantImports    []*internal.Import
		wantImportedBy []string
	}{
		{
			path:           pkg3.Path,
			version:        "v1.3.0",
			wantImports:    pkg3.Imports,
			wantImportedBy: nil,
		},
		{
			path:           pkg2.Path,
			version:        "v1.2.0",
			wantImports:    pkg2.Imports,
			wantImportedBy: []string{pkg3.Path},
		},
		{
			path:           pkg1.Path,
			version:        "v1.1.0",
			wantImports:    nil,
			wantImportedBy: []string{pkg2.Path, pkg3.Path},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			teardownTestCase, db := SetupCleanDB(t)
			defer teardownTestCase(t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			for _, v := range testVersions {
				if err := db.InsertVersion(ctx, v, sampleLicenses); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}
			}

			got, err := db.GetImports(ctx, tc.path, tc.version)
			if err != nil {
				t.Fatalf("db.GetImports(%q, %q): %v", tc.path, tc.version, err)
			}

			sort.Slice(got, func(i, j int) bool {
				return got[i].Name > got[j].Name
			})
			sort.Slice(tc.wantImports, func(i, j int) bool {
				return tc.wantImports[i].Name > tc.wantImports[j].Name
			})
			if diff := cmp.Diff(tc.wantImports, got); diff != "" {
				t.Errorf("db.GetImports(%q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}

			gotImportedBy, err := db.GetImportedBy(ctx, tc.path)
			if err != nil {
				t.Fatalf("db.GetImports(%q, %q): %v", tc.path, tc.version, err)
			}

			if diff := cmp.Diff(tc.wantImportedBy, gotImportedBy); diff != "" {
				t.Errorf("db.GetImportedBy(%q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
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
		seriesPath   = "path.to/foo"
		modulePath1  = "path.to/foo"
		modulePath2  = "path.to/foo/v2"
		modulePath3  = "path.to/some/thing"
		testVersions = []*internal.Version{
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath3,
					Version:     "v3.0.0",
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
				},
				Packages: []*internal.Package{pkg3},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath1,
					Version:     "v1.0.0-alpha.1",
					CommitTime:  now,
					VersionType: internal.VersionTypePrerelease,
				},
				Packages: []*internal.Package{pkg1},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath1,
					Version:     "v1.0.0",
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
				},
				Packages: []*internal.Package{pkg1},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath2,
					Version:     "v2.0.1-beta",
					CommitTime:  now,
					VersionType: internal.VersionTypePrerelease,
				},
				Packages: []*internal.Package{pkg2},
			},
			&internal.Version{
				VersionInfo: internal.VersionInfo{
					SeriesPath:  seriesPath,
					ModulePath:  modulePath2,
					Version:     "v2.1.0",
					CommitTime:  now,
					VersionType: internal.VersionTypeRelease,
				},
				Packages: []*internal.Package{pkg2},
			},
		}
	)

	testCases := []struct {
		name, path         string
		numPseudo          int
		versions           []*internal.Version
		wantTaggedVersions []*internal.VersionInfo
	}{
		{
			name:      "want_releases_and_prereleases_only",
			path:      "path.to/foo/bar",
			numPseudo: 12,
			versions:  testVersions,
			wantTaggedVersions: []*internal.VersionInfo{
				&internal.VersionInfo{
					SeriesPath: seriesPath,
					ModulePath: modulePath2,
					Version:    "v2.1.0",
					CommitTime: now,
				},
				&internal.VersionInfo{
					SeriesPath: seriesPath,
					ModulePath: modulePath2,
					Version:    "v2.0.1-beta",
					CommitTime: now,
				},
				&internal.VersionInfo{
					SeriesPath: seriesPath,
					ModulePath: modulePath1,
					Version:    "v1.0.0",
					CommitTime: now,
				},
				&internal.VersionInfo{
					SeriesPath: seriesPath,
					ModulePath: modulePath1,
					Version:    "v1.0.0-alpha.1",
					CommitTime: now,
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

			wantPseudoVersions := []*internal.VersionInfo{}
			for i := 0; i < tc.numPseudo; i++ {
				v := &internal.Version{
					VersionInfo: internal.VersionInfo{
						SeriesPath: seriesPath,
						ModulePath: modulePath1,
						// %02d makes a string that is a width of 2 and left pads with zeroes
						Version:     fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", i+1),
						CommitTime:  now,
						VersionType: internal.VersionTypePseudo,
					},
					Packages: []*internal.Package{pkg1},
				}
				if err := db.InsertVersion(ctx, v, nil); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}

				// GetPseudoVersions should only return the 10 most recent pseudo versions,
				// if there are more than 10 in the database
				if i < 10 {
					wantPseudoVersions = append(wantPseudoVersions, &internal.VersionInfo{
						SeriesPath: seriesPath,
						ModulePath: modulePath1,
						Version:    fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", tc.numPseudo-i),
						CommitTime: now,
					})
				}
			}

			for _, v := range tc.versions {
				if err := db.InsertVersion(ctx, v, nil); err != nil {
					t.Errorf("db.InsertVersion(%v): %v", v, err)
				}
			}

			var (
				got []*internal.VersionInfo
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
				if diff := cmp.Diff(wantPseudoVersions[i], v, cmpopts.IgnoreFields(internal.VersionInfo{}, "VersionType")); diff != "" {
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

				if diff := cmp.Diff(tc.wantTaggedVersions[i], v, cmpopts.IgnoreFields(internal.VersionInfo{},
					"VersionType")); diff != "" {
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
		now         = NowTruncated()
		seriesPath  = "myseries"
		modulePath  = "test.module"
		testVersion = &internal.Version{
			VersionInfo: internal.VersionInfo{
				SeriesPath:  seriesPath,
				ModulePath:  modulePath,
				Version:     "v1.0.0",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				VersionType: internal.VersionTypeRelease,
			},
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
				cmpopts.IgnoreFields(internal.Version{}, "VersionType")); diff != "" {
				t.Errorf("db.GetVersionForPackage(ctx, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}

func TestGetLicenses(t *testing.T) {
	var (
		now         = NowTruncated()
		seriesPath  = "myseries"
		modulePath  = "test.module"
		testVersion = &internal.Version{
			VersionInfo: internal.VersionInfo{
				SeriesPath:  seriesPath,
				ModulePath:  modulePath,
				Version:     "v1.0.0",
				ReadMe:      []byte("readme"),
				CommitTime:  now,
				VersionType: internal.VersionTypeRelease,
			},
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

	tests := []struct {
		label, pkgPath string
		wantLicenses   []*internal.License
	}{
		{
			label:        "package with licenses",
			pkgPath:      "test.module/foo",
			wantLicenses: sampleLicenses,
		}, {
			label:        "package with no licenses",
			pkgPath:      "test.module",
			wantLicenses: nil,
		},
	}

	teardownTestCase, db := SetupCleanDB(t)
	defer teardownTestCase(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := db.InsertVersion(ctx, testVersion, sampleLicenses); err != nil {
		t.Errorf("db.InsertVersion(ctx, %q, licenses): %v", testVersion.Version, err)
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			got, err := db.GetLicenses(ctx, test.pkgPath, testVersion.Version)
			if err != nil {
				t.Fatalf("db.GetLicenses(ctx, %q, %q): %v", test.pkgPath, testVersion.Version, err)
			}
			if diff := cmp.Diff(got, test.wantLicenses); diff != "" {
				t.Errorf("db.GetLicenses(ctx, %q, %q) mismatch (-got +want):\n%s", test.pkgPath, testVersion.Version, diff)
			}
		})
	}
}
