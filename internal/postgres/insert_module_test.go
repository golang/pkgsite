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
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml"
	"github.com/google/safehtml/testconversions"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestInsertModule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout*2)
	defer cancel()

	for _, test := range []struct {
		name   string
		module *internal.Module
	}{
		{
			name:   "valid test",
			module: sample.DefaultModule(),
		},
		{
			name:   "valid test with internal package",
			module: sample.Module(sample.ModulePath, sample.VersionString, "internal/ffoo"),
		},
		{
			name: "valid test with go.mod missing",
			module: func() *internal.Module {
				m := sample.DefaultModule()
				m.HasGoMod = false
				return m
			}(),
		},
		{
			name: "stdlib",
			module: func() *internal.Module {
				m := sample.Module("std", "v1.12.5")
				p := &internal.LegacyPackage{
					Name:              "context",
					Path:              "context",
					Synopsis:          "This is a package synopsis",
					Licenses:          sample.LicenseMetadata,
					DocumentationHTML: testconversions.MakeHTMLForTest("This is the documentation HTML"),
				}
				return sample.AddPackage(m, p)
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

			checkModule(ctx, t, test.module)
		})
	}
}

func checkModule(ctx context.Context, t *testing.T, want *internal.Module) {
	got, err := testDB.LegacyGetModuleInfo(ctx, want.ModulePath, want.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want.LegacyModuleInfo, *got, cmp.AllowUnexported(source.Info{})); diff != "" {
		t.Fatalf("testDB.LegacyGetModuleInfo(%q, %q) mismatch (-want +got):\n%s", want.ModulePath, want.Version, diff)
	}

	for _, wantp := range want.LegacyPackages {
		got, err := testDB.LegacyGetPackage(ctx, wantp.Path, want.ModulePath, want.Version)
		if err != nil {
			t.Fatal(err)
		}
		opts := cmp.Options{
			// The packages table only includes partial license information; it
			// omits the Coverage field.
			cmpopts.IgnoreFields(internal.LegacyPackage{}, "Imports"),
			cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			cmpopts.EquateEmpty(),
			cmp.AllowUnexported(safehtml.HTML{}),
		}
		if diff := cmp.Diff(*wantp, got.LegacyPackage, opts...); diff != "" {
			t.Fatalf("testDB.LegacyGetPackage(%q, %q) mismatch (-want +got):\n%s", wantp.Path, want.Version, diff)
		}
	}

	for _, dir := range want.Directories {
		got, err := testDB.GetDirectory(ctx, dir.Path, want.ModulePath, want.Version)
		if err != nil {
			t.Fatal(err)
		}
		// TODO(golang/go#38513): remove once we start displaying
		// READMEs for directories instead of the top-level module.
		dir.Readme = &internal.Readme{
			Filepath: sample.ReadmeFilePath,
			Contents: sample.ReadmeContents,
		}
		wantd := internal.VersionedDirectory{
			Directory:  *dir,
			ModuleInfo: want.ModuleInfo,
		}
		opts := cmp.Options{
			cmpopts.IgnoreFields(internal.LegacyModuleInfo{}, "LegacyReadmeFilePath"),
			cmpopts.IgnoreFields(internal.LegacyModuleInfo{}, "LegacyReadmeContents"),
			cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
		}
		if diff := cmp.Diff(wantd, *got, opts); diff != "" {
			t.Errorf("testDB.getDirectory(%q, %q) mismatch (-want +got):\n%s", dir.Path, want.Version, diff)
		}
	}
}

func TestUpsertModule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	m := sample.Module("upsert.org", "v1.2.3")
	p := &internal.LegacyPackage{
		Name:              "p",
		Path:              "upsert.org/dir/p",
		Synopsis:          "This is a package synopsis",
		Licenses:          sample.LicenseMetadata,
		DocumentationHTML: testconversions.MakeHTMLForTest("This is the documentation HTML"),
	}
	sample.AddPackage(m, p)

	// Insert the module.
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}
	// Change the module, and re-insert.
	m.IsRedistributable = !m.IsRedistributable
	m.Licenses[0].Contents = append(m.Licenses[0].Contents, " and more"...)
	// TODO(golang/go#38513): uncomment line below once we start displaying
	// READMEs for directories instead of the top-level module.
	// m.Directories[0].Readme.Contents += " and more"
	m.LegacyPackages[0].Synopsis = "New synopsis"
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	// The changes should have been saved.
	checkModule(ctx, t, m)
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
			module:         sample.DefaultModule(),
			wantModulePath: sample.ModulePath,
			wantVersion:    "v1.2.3",
		},
		{
			name:           "nonexistent module",
			module:         sample.DefaultModule(),
			wantModulePath: "nonexistent_module_path",
			wantVersion:    "v1.0.0",
			wantPkgPath:    sample.PackagePath,
		},
		{
			name:           "missing module path",
			module:         sample.Module("", sample.VersionString),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.DBModuleInsertInvalid,
		},
		{
			name: "missing version",
			module: func() *internal.Module {
				m := sample.DefaultModule()
				m.Version = ""
				return m
			}(),
			wantVersion:    sample.VersionString,
			wantModulePath: sample.ModulePath,
			wantWriteErr:   derrors.DBModuleInsertInvalid,
		},
		{
			name: "empty commit time",
			module: func() *internal.Module {
				v := sample.DefaultModule()
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
	// table that are not in the LegacyModuleInfo struct.
	defer ResetTestDB(testDB, t)
	ctx := context.Background()

	type other struct {
		sortVersion, seriesPath string
	}

	v := sample.Module("github.com/user/repo/path/v2", "v1.2.3-beta.4.a", sample.Suffix)
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

func TestDeleteModule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	v := sample.DefaultModule()

	if err := testDB.InsertModule(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.LegacyGetModuleInfo(ctx, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}

	vm := sample.DefaultVersionMap()
	if err := testDB.UpsertVersionMap(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.GetVersionMap(ctx, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}

	if err := testDB.DeleteModule(ctx, v.ModulePath, v.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.LegacyGetModuleInfo(ctx, v.ModulePath, v.Version); !errors.Is(err, derrors.NotFound) {
		t.Errorf("got %v, want NotFound", err)
	}

	var x int
	err := testDB.Underlying().QueryRow(ctx, "SELECT 1 FROM imports_unique WHERE from_module_path = $1",
		v.ModulePath).Scan(&x)
	if err != sql.ErrNoRows {
		t.Errorf("imports_unique: got %v, want ErrNoRows", err)
	}
	err = testDB.Underlying().QueryRow(
		ctx,
		"SELECT 1 FROM version_map WHERE module_path = $1 AND resolved_version = $2",
		v.ModulePath, v.Version).Scan(&x)
	if err != sql.ErrNoRows {
		t.Errorf("version_map: got %v, want ErrNoRows", err)
	}
}

func TestPostgres_NewerAlternative(t *testing.T) {
	// Verify that packages are not added to search_documents if the module has a newer
	// alternative version.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	const (
		modulePath = "example.com/Mod"
		altVersion = "v1.2.0"
		okVersion  = "v1.0.0"
	)

	err := testDB.UpsertModuleVersionState(ctx, modulePath, altVersion, "appVersion", time.Now(),
		derrors.ToHTTPStatus(derrors.AlternativeModule), "example.com/mod", derrors.AlternativeModule, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := sample.Module(modulePath, okVersion, "p")
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, _, found := GetFromSearchDocuments(ctx, t, testDB, m.LegacyPackages[0].Path); found {
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

func TestLock(t *testing.T) {
	// Verify that two transactions cannot both hold the same lock, but that every one
	// that wants the lock eventually gets it.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	db := testDB.Underlying()

	const n = 4
	errc := make(chan error)
	var (
		mu       sync.Mutex
		lockHeld bool
		count    int
	)

	for i := 0; i < n; i++ {
		go func() {
			errc <- db.Transact(ctx, sql.LevelDefault, func(tx *database.DB) error {
				if err := lock(ctx, tx, sample.ModulePath); err != nil {
					return err
				}

				mu.Lock()
				h := lockHeld
				lockHeld = true
				count++
				mu.Unlock()
				if h {
					return errors.New("lock already held")
				}
				time.Sleep(50 * time.Millisecond)
				mu.Lock()
				lockHeld = false
				mu.Unlock()
				return nil
			})
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
	if count != n {
		t.Errorf("got %d, want %d", count, n)
	}
}
