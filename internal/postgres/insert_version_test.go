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

func TestPostgres_ReadAndWriteModuleAndPackages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout*2)
	defer cancel()

	testCases := []struct {
		name string

		module *internal.Module

		// identifiers to use for fetch
		wantModulePath, wantVersion, wantPkgPath string

		// error conditions
		wantWriteErr error
		wantReadErr  bool
	}{
		{
			name:           "valid test",
			module:         sample.Module(),
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantPkgPath:    sample.PackagePath,
		},
		{
			name: "valid test with internal package",
			module: func() *internal.Module {
				v := sample.Module()
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
			module: func() *internal.Module {
				v := sample.Module()
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
			module:         sample.Module(),
			wantModulePath: sample.ModulePath,
			wantVersion:    "v1.2.3",
			wantReadErr:    true,
		},
		{
			name:           "nonexistent module",
			module:         sample.Module(),
			wantModulePath: "nonexistent_module_path",
			wantVersion:    "v1.0.0",
			wantPkgPath:    sample.PackagePath,
			wantReadErr:    true,
		},
		{
			name: "missing module path",
			module: func() *internal.Module {
				v := sample.Module()
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
			module: func() *internal.Module {
				v := sample.Module()
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
			module: func() *internal.Module {
				v := sample.Module()
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
			module: func() *internal.Module {
				m := sample.Module()
				m.ModulePath = "std"
				m.Version = "v1.12.5"
				m.Packages = []*internal.Package{{
					Name:              "context",
					Path:              "context",
					Synopsis:          "This is a package synopsis",
					Licenses:          sample.LicenseMetadata,
					DocumentationHTML: "This is the documentation HTML",
				}}
				return m
			}(),
			wantModulePath: "std",
			wantVersion:    "v1.12.5",
			wantPkgPath:    "context",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			if err := testDB.InsertModule(ctx, tc.module); !errors.Is(err, tc.wantWriteErr) {
				t.Errorf("error: %v, want write error: %v", err, tc.wantWriteErr)
			}

			// Test that insertion of duplicate primary key won't fail.
			if err := testDB.InsertModule(ctx, tc.module); !errors.Is(err, tc.wantWriteErr) {
				t.Errorf("second insert error: %v, want write error: %v", err, tc.wantWriteErr)
			}

			got, err := testDB.GetModuleInfo(ctx, tc.wantModulePath, tc.wantVersion)
			if tc.wantReadErr != (err != nil) {
				t.Fatalf("error: got %v, want read error: %t", err, tc.wantReadErr)
			}

			if !tc.wantReadErr && got == nil {
				t.Fatalf("testDB.GetModuleInfo(ctx, %q, %q) = %v, want %v", tc.wantModulePath, tc.wantVersion, got, tc.module)
			}

			if tc.module != nil {
				if diff := cmp.Diff(&tc.module.ModuleInfo, got, cmp.AllowUnexported(source.Info{})); !tc.wantReadErr && diff != "" {
					t.Errorf("testDB.GetModuleInfo(ctx, %q, %q) mismatch (-want +got):\n%s", tc.wantModulePath, tc.wantVersion, diff)
				}
			}

			gotPkg, err := testDB.GetPackage(ctx, tc.wantPkgPath, internal.UnknownModulePath, tc.wantVersion)
			if tc.module == nil || tc.module.Packages == nil || tc.wantPkgPath == "" {
				if tc.wantReadErr != (err != nil) {
					t.Fatalf("got %v, want %v", err, sql.ErrNoRows)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			wantPkg := tc.module.Packages[0]
			if gotPkg.ModuleInfo.Version != tc.module.Version {
				t.Errorf("testDB.GetPackage(ctx, %q, %q) version.version = %v, want %v", tc.wantPkgPath, tc.wantVersion, gotPkg.ModuleInfo.Version, tc.module.Version)
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

func TestPostgres_ReadAndWriteModuleOtherColumns(t *testing.T) {
	// Verify that InsertVersion correctly populates the columns in the versions
	// table that are not in the ModuleInfo struct.
	defer ResetTestDB(testDB, t)
	ctx := context.Background()

	type other struct {
		sortVersion, seriesPath string
	}

	v := sample.Module()
	v.ModulePath = "github.com/user/repo/path/v2"
	v.Version = "v1.2.3-beta.4.a"

	want := other{
		sortVersion: "1,2,3,~beta,4,~a",
		seriesPath:  "github.com/user/repo/path",
	}

	if err := testDB.InsertModule(ctx, v); err != nil {
		t.Fatal(err)
	}
	query := `
	SELECT
		sort_version, series_path
	FROM
		modules
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

	v := sample.Module()
	if err := testDB.InsertModule(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetModuleInfo(ctx, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if err := testDB.DeleteModule(ctx, nil, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetModuleInfo(ctx, v.ModulePath, v.Version); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
	}
}

func TestPostgres_NewerAlternative(t *testing.T) {
	// Verify that packages are not added to search_documents if the module has a newer
	// alternative version.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	const (
		modulePath  = "example.com/Mod"
		packagePath = modulePath + "/p"
		altVersion  = "v1.2.0"
		okVersion   = "v1.0.0"
	)

	err := testDB.UpsertModuleVersionState(ctx, modulePath, altVersion, "appVersion", time.Now(),
		derrors.ToHTTPStatus(derrors.AlternativeModule), "example.com/mod", derrors.AlternativeModule, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := sample.Module()
	m.ModulePath = modulePath
	m.Version = okVersion
	m.Packages[0].Name = "p"
	m.Packages[0].Path = packagePath
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, _, found := GetFromSearchDocuments(ctx, t, testDB, packagePath); found {
		t.Fatal("found package after inserting")
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
