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
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestInsertModule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout*2)
	defer cancel()
	ctx = experiment.NewContext(ctx,
		experiment.NewSet(map[string]bool{
			internal.ExperimentInsertDirectories: true}))

	for _, test := range []struct {
		name   string
		module *internal.Module
	}{
		{
			name:   "valid test",
			module: sample.Module(),
		},
		{
			name: "valid test with internal package",
			module: func() *internal.Module {
				m := sample.Module()
				p := sample.Package()
				p.Path = sample.ModulePath + "/internal/foo"
				m.Packages = []*internal.Package{p}
				d1 := sample.DirectoryNewForModuleRoot(&m.ModuleInfo, p.Licenses)
				d2 := sample.DirectoryNewEmpty(sample.ModulePath + "/internal")
				d3 := sample.DirectoryNewForPackage(p)
				m.Directories = []*internal.DirectoryNew{d1, d2, d3}
				return m
			}(),
		},
		{
			name: "valid test with go.mod missing",
			module: func() *internal.Module {
				m := sample.Module()
				m.HasGoMod = false
				return m
			}(),
		},
		{
			name: "stdlib",
			module: func() *internal.Module {
				m := sample.Module()
				m.ModulePath = "std"
				m.Version = "v1.12.5"
				p := &internal.Package{
					Name:              "context",
					Path:              "context",
					Synopsis:          "This is a package synopsis",
					Licenses:          sample.LicenseMetadata,
					DocumentationHTML: "This is the documentation HTML",
				}
				m.Packages = []*internal.Package{p}
				d1 := sample.DirectoryNewForModuleRoot(&m.ModuleInfo, p.Licenses)
				d2 := sample.DirectoryNewForPackage(p)
				m.Directories = []*internal.DirectoryNew{d1, d2}
				return m
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			if err := testDB.InsertModule(ctx, test.module); err != nil {
				t.Fatal(err)
			}
			// Test that insertion of duplicate primary key won't fail.
			if err := testDB.InsertModule(ctx, test.module); err != nil {
				t.Fatal(err)
			}

			got, err := testDB.GetModuleInfo(ctx, test.module.ModulePath, test.module.Version)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.module.ModuleInfo, *got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Fatalf("testDB.GetModuleInfo(%q, %q) mismatch (-want +got):\n%s", test.module.ModulePath, test.module.Version, diff)
			}

			for _, want := range test.module.Packages {
				got, err := testDB.GetPackage(ctx, want.Path, test.module.ModulePath, test.module.Version)
				if err != nil {
					t.Fatal(err)
				}
				opts := cmp.Options{
					// The packages table only includes partial
					// license information; it omits the Coverage
					// field.
					cmpopts.IgnoreFields(internal.Package{}, "Imports"),
					cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
					cmpopts.EquateEmpty(),
				}
				if diff := cmp.Diff(*want, got.Package, opts...); diff != "" {
					t.Fatalf("testDB.GetPackage(%q, %q) mismatch (-want +got):\n%s", want.Path, test.module.Version, diff)
				}

			}

			for _, dir := range test.module.Directories {
				got, err := testDB.getDirectoryNew(ctx, dir.Path, test.module.ModulePath, test.module.Version)
				if err != nil {
					t.Fatal(err)
				}
				want := internal.VersionedDirectory{
					DirectoryNew: *dir,
					ModuleInfo:   test.module.ModuleInfo,
				}
				opts := cmp.Options{
					cmpopts.IgnoreFields(internal.ModuleInfo{}, "ReadmeFilePath"),
					cmpopts.IgnoreFields(internal.ModuleInfo{}, "ReadmeContents"),
					cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
					cmp.AllowUnexported(source.Info{}),
				}
				if diff := cmp.Diff(want, *got, opts); diff != "" {
					t.Errorf("testDB.getDirectoryNew(%q, %q) mismatch (-want +got):\n%s", dir.Path, test.module.Version, diff)
				}
			}
		})
	}
}
func TestInsertModuleErrors(t *testing.T) {
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
			name:           "nil version write error",
			wantModulePath: sample.ModulePath,
			wantVersion:    sample.VersionString,
			wantWriteErr:   derrors.DBModuleInsertInvalid,
		},
		{
			name:           "nonexistent version",
			module:         sample.Module(),
			wantModulePath: sample.ModulePath,
			wantVersion:    "v1.2.3",
		},
		{
			name:           "nonexistent module",
			module:         sample.Module(),
			wantModulePath: "nonexistent_module_path",
			wantVersion:    "v1.0.0",
			wantPkgPath:    sample.PackagePath,
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
			wantWriteErr:   derrors.DBModuleInsertInvalid,
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
			wantWriteErr:   derrors.DBModuleInsertInvalid,
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
			wantWriteErr:   derrors.DBModuleInsertInvalid,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			if err := testDB.InsertModule(ctx, tc.module); !errors.Is(err, tc.wantWriteErr) {
				t.Errorf("error: %v, want write error: %v", err, tc.wantWriteErr)
			}
		})
	}
}

func TestPostgres_ReadAndWriteModuleOtherColumns(t *testing.T) {
	// Verify that InsertModule correctly populates the columns in the versions
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

func TestPostgres_DeleteModule(t *testing.T) {
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
	if err := testDB.DeleteModule(ctx, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetModuleInfo(ctx, v.ModulePath, v.Version); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
	}
	var x int
	err := testDB.Underlying().QueryRow(ctx, "SELECT 1 FROM imports_unique WHERE from_module_path = $1",
		v.ModulePath).Scan(&x)
	if err != sql.ErrNoRows {
		t.Errorf("imports_unique: got %v, want ErrNoRows", err)
	}
	// TODO(b/154616892): check removal from version_map
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
		if err := insert(makeValidUnicode(string(data))); err != nil {
			t.Errorf("%s, after making valid: %v", filename, err)
		}
	}

	check("final-nulls", false)
	check("gin-gonic", true)
	check("subchord", true)
}
