// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/testing/sample"
)

func TestPostgres_ReadAndWriteVersionAndPackages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout*2)
	defer cancel()

	testCases := []struct {
		name string

		version *internal.Version

		// identifiers to use for fetch
		wantModulePath, wantVersion, wantPkgPath string

		// error conditions
		wantWriteErr error
		wantReadErr  bool
	}{
		{
			name:           "valid test",
			version:        sample.Version(),
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantPkgPath:    sample.PackagePath,
		},
		{
			name: "valid test with internal package",
			version: func() *internal.Version {
				v := sample.Version()
				p := sample.Package()
				p.Path = sample.ModulePath + "/internal/foo"
				v.Packages = []*internal.Package{p}
				return v
			}(),
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantPkgPath:    sample.ModulePath + "/internal/foo",
		},
		{
			name: "valid test with go.mod missing",
			version: func() *internal.Version {
				v := sample.Version()
				v.HasGoMod = false
				return v
			}(),
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantPkgPath:    sample.PackagePath,
		},
		{
			name:           "nil version write error",
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name:           "nonexistent version",
			version:        sample.Version(),
			wantModulePath: sample.ModulePath,
			wantVersion:    "v1.2.3",
			wantReadErr:    true,
		},
		{
			name:           "nonexistent module",
			version:        sample.Version(),
			wantModulePath: "nonexistent_module_path",
			wantVersion:    "v1.0.0",
			wantPkgPath:    sample.PackagePath,
			wantReadErr:    true,
		},
		{
			name: "missing module path",
			version: func() *internal.Version {
				v := sample.Version()
				v.ModulePath = ""
				return v
			}(),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name: "missing version",
			version: func() *internal.Version {
				v := sample.Version()
				v.Version = ""
				return v
			}(),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name: "empty commit time",
			version: func() *internal.Version {
				v := sample.Version()
				v.CommitTime = time.Time{}
				return v
			}(),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.InvalidArgument,
			wantReadErr:    true,
		},
		{
			name: "stdlib",
			version: func() *internal.Version {
				v := sample.Version()
				v.ModulePath = "std"
				v.Version = "v1.12.5"
				v.Packages = []*internal.Package{{
					Name:              "context",
					Path:              "context",
					Synopsis:          "This is a package synopsis",
					Licenses:          sample.LicenseMetadata,
					DocumentationHTML: "This is the documentation HTML",
				}}
				return v
			}(),
			wantModulePath: "std",
			wantVersion:    "v1.12.5",
			wantPkgPath:    "context",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			if err := testDB.InsertVersion(ctx, tc.version); !errors.Is(err, tc.wantWriteErr) {
				t.Errorf("error: %v, want write error: %v", err, tc.wantWriteErr)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := testDB.InsertVersion(ctx, tc.version); !errors.Is(err, tc.wantWriteErr) {
				t.Errorf("second insert error: %v, want write error: %v", err, tc.wantWriteErr)
			}

			got, err := testDB.GetVersionInfo(ctx, tc.wantModulePath, tc.wantVersion)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("error: got %v, want read error: %t", err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("testDB.GetVersionInfo(ctx, %q, %q) = %v, want %v", tc.wantModulePath, tc.wantVersion, got, tc.version)
			}

			if tc.version != nil {
				if diff := cmp.Diff(&tc.version.VersionInfo, got, cmp.AllowUnexported(source.Info{})); !tc.wantReadErr && diff != "" {
					t.Errorf("testDB.GetVersionInfo(ctx, %q, %q) mismatch (-want +got):\n%s", tc.wantModulePath, tc.wantVersion, diff)
				}
			}

			gotPkg, err := testDB.GetPackage(ctx, tc.wantPkgPath, internal.UnknownModulePath, tc.wantVersion)
			if tc.version == nil || tc.version.Packages == nil || tc.wantPkgPath == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("got %v, want %v", err, sql.ErrNoRows)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			wantPkg := tc.version.Packages[0]
			if gotPkg.VersionInfo.Version != tc.version.Version {
				t.Errorf("testDB.GetPackage(ctx, %q, %q) version.version = %v, want %v", tc.wantPkgPath, tc.wantVersion, gotPkg.VersionInfo.Version, tc.version.Version)
			}

			opts := cmp.Options{
				cmpopts.IgnoreFields(internal.Package{}, "Imports"),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			}
			if diff := cmp.Diff(wantPkg, &gotPkg.Package, opts...); diff != "" {
				t.Errorf("testDB.GetPackage(%q, %q) Package mismatch (-want +got):\n%s", tc.wantPkgPath, tc.wantVersion, diff)
			}
		})
	}
}

func TestPostgres_ReadAndWriteVersionOtherColumns(t *testing.T) {
	// Verify that InsertVersion correctly populates the columns in the versions
	// table that are not in the VersionInfo struct.
	defer ResetTestDB(testDB, t)
	ctx := context.Background()

	type other struct {
		sortVersion, seriesPath string
	}

	v := sample.Version()
	v.ModulePath = "github.com/user/repo/path/v2"
	v.Version = "v1.2.3-beta.4.a"

	want := other{
		sortVersion: "1,2,3,~beta,4,~a",
		seriesPath:  "github.com/user/repo/path",
	}

	if err := testDB.InsertVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	query := `
	SELECT
		sort_version, series_path
	FROM
		versions
	WHERE
		module_path = $1 AND version = $2`
	row := testDB.db.QueryRow(ctx, query, v.ModulePath, v.Version)
	var got other
	if err := row.Scan(&got.sortVersion, &got.seriesPath); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("\ngot  %+v\nwant %+v", got, want)
	}
}

func TestPostgres_DeleteVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	v := sample.Version()
	if err := testDB.InsertVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetVersionInfo(ctx, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if err := testDB.DeleteVersion(ctx, nil, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetVersionInfo(ctx, v.ModulePath, v.Version); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
	}
}

func TestMakeValidUnicode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	db := testDB.Underlying()

	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS valid_unicode (contents text)`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DROP TABLE valid_unicode`)

	insert := func(s string) error {
		_, err := db.Exec(ctx, `INSERT INTO valid_unicode VALUES($1)`, s)
		return err
	}

	check := func(filename string, okRaw bool) {
		data, err := ioutil.ReadFile(filepath.Join("testdata", filename))
		if err != nil {
			t.Fatal(err)
		}
		raw := string(data)
		err = insert(raw)
		if (err == nil) != okRaw {
			t.Errorf("%s, raw: got %v, want error: %t", filename, err, okRaw)
		}
		if err := insert(makeValidUnicode(data)); err != nil {
			t.Errorf("%s, after making valid: %v", filename, err)
		}
	}

	check("final-nulls", false)
	check("gin-gonic", true)
	check("subchord", true)
}
