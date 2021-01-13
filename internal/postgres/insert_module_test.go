// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/licensecheck"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/database"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
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
			module: sample.Module(sample.ModulePath, sample.VersionString, "internal/foo"),
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
			name:   "stdlib",
			module: sample.Module("std", "v1.12.5", "context"),
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
	got, err := testDB.GetModuleInfo(ctx, want.ModulePath, want.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want.ModuleInfo, *got, cmp.AllowUnexported(source.Info{})); diff != "" {
		t.Fatalf("testDB.GetModuleInfo(%q, %q) mismatch (-want +got):\n%s", want.ModulePath, want.Version, diff)
	}

	for _, wantu := range want.Units {
		got, err := testDB.GetUnit(ctx, &wantu.UnitMeta, internal.AllFields)
		if err != nil {
			t.Fatal(err)
		}
		wantu.LicenseContents = sample.Licenses
		var subdirectories []*internal.PackageMeta
		for _, u := range want.Units {
			if u.IsPackage() && (strings.HasPrefix(u.Path, wantu.Path) || wantu.Path == stdlib.ModulePath) {
				subdirectories = append(subdirectories, sample.PackageMeta(u.Path))
			}
		}
		wantu.Subdirectories = subdirectories
		opts := cmp.Options{
			cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
		}
		if diff := cmp.Diff(wantu, got, opts); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestInsertModuleLicenseCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass=%t", bypass), func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			var db *DB
			if bypass {
				db = NewBypassingLicenseCheck(testDB.db)
			} else {
				db = testDB
			}
			checkHasRedistData := func(readme string, doc []byte, want bool) {
				t.Helper()
				if got := readme != ""; got != want {
					t.Errorf("readme: got %t, want %t", got, want)
				}
				if got := doc != nil; got != want {
					t.Errorf("doc: got %t, want %t", got, want)
				}
			}

			mod := sample.Module(sample.ModulePath, sample.VersionString, "")
			checkHasRedistData(mod.Units[0].Readme.Contents, mod.Units[0].Documentation.Source, true)
			mod.IsRedistributable = false
			mod.Units[0].IsRedistributable = false

			if err := db.InsertModule(ctx, mod); err != nil {
				t.Fatal(err)
			}

			// New model
			pathInfo := &internal.UnitMeta{
				Path:       mod.ModulePath,
				ModulePath: mod.ModulePath,
				Version:    mod.Version,
			}
			u, err := db.GetUnit(ctx, pathInfo, internal.AllFields)
			if err != nil {
				t.Fatal(err)
			}
			var (
				source []byte
				readme string
			)
			if u.Readme != nil {
				readme = u.Readme.Contents
			}
			if u.Documentation != nil {
				source = u.Documentation.Source
			}
			checkHasRedistData(readme, source, bypass)
		})
	}
}

func TestUpsertModule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	m := sample.Module("upsert.org", "v1.2.3", "dir/p")

	// Insert the module.
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}
	// Change the module, and re-insert.
	m.IsRedistributable = !m.IsRedistributable
	m.Licenses[0].Contents = append(m.Licenses[0].Contents, " and more"...)
	m.Units[0].Readme.Contents += " and more"

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
			wantWriteErr:   derrors.BadModule,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)
			if err := testDB.InsertModule(ctx, test.module); !errors.Is(err, test.wantWriteErr) {
				t.Errorf("error: %v, want write error: %v", err, test.wantWriteErr)
			}
		})
	}
}

func TestInsertModuleNewCoverage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	m := sample.DefaultModule()
	newCoverage := licensecheck.Coverage{
		Percent: 100,
		Match:   []licensecheck.Match{{ID: "New", Start: 1, End: 10}},
	}
	m.Licenses = []*licenses.License{
		{
			Metadata: &licenses.Metadata{
				Types:       []string{sample.LicenseType},
				FilePath:    sample.LicenseFilePath,
				NewCoverage: newCoverage,
			},
			Contents: []byte(`Lorem Ipsum`),
		},
	}
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}
	u, err := testDB.GetUnit(ctx, &internal.UnitMeta{Path: m.ModulePath, ModulePath: m.ModulePath, Version: m.Version}, internal.AllFields)
	if err != nil {
		t.Fatal(err)
	}
	got := u.LicenseContents[0].Metadata
	want := &licenses.Metadata{
		Types:       []string{"MIT"},
		FilePath:    sample.LicenseFilePath,
		NewCoverage: newCoverage,
	}
	if !cmp.Equal(got, want) {
		t.Errorf("\ngot  %+v\nwant %+v", got, want)
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

func TestLatestVersion(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx := context.Background()

	for _, mod := range []struct {
		version    string
		modulePath string
	}{
		{
			version:    "v1.5.2",
			modulePath: sample.ModulePath,
		},
		{
			version:    "v2.0.0+incompatible",
			modulePath: sample.ModulePath,
		},
		{
			version:    "v2.0.1",
			modulePath: sample.ModulePath + "/v2",
		},
		{
			version:    "v3.0.1-rc9.0.20200212222136-a4a89636720b",
			modulePath: sample.ModulePath + "/v3",
		},
		{
			version:    "v3.0.1-rc9",
			modulePath: sample.ModulePath + "/v3",
		},
	} {
		m := sample.DefaultModule()
		m.Version = mod.version
		m.ModulePath = mod.modulePath

		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name        string
		modulePath  string
		wantVersion string
	}{
		{
			name:        "test v1 version",
			modulePath:  sample.ModulePath,
			wantVersion: "v1.5.2",
		},
		{
			name:        "test v2 version",
			modulePath:  sample.ModulePath + "/v2",
			wantVersion: "v2.0.1",
		},
		{
			name:        "test v3 version - prefer prerelease over pseudo",
			modulePath:  sample.ModulePath + "/v3",
			wantVersion: "v3.0.1-rc9",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			isLatest, err := isLatestVersion(ctx, testDB.db, test.modulePath, test.wantVersion)
			if err != nil {
				t.Fatal(err)
			}
			if !isLatest {
				t.Errorf("%s is not the latest version", test.wantVersion)
			}
		})
	}
}

func TestLatestVersion_PreferIncompatibleOverPrerelease(t *testing.T) {
	defer ResetTestDB(testDB, t)
	ctx := context.Background()

	for _, mod := range []struct {
		version    string
		modulePath string
	}{
		{
			version:    "v0.0.0-20201007032633-0806396f153e",
			modulePath: sample.ModulePath,
		},
		{
			version:    "v2.0.0+incompatible",
			modulePath: sample.ModulePath,
		},
	} {
		m := sample.DefaultModule()
		m.Version = mod.version
		m.ModulePath = mod.modulePath

		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		modulePath string
		want       string
	}{
		{
			modulePath: sample.ModulePath,
			want:       "v2.0.0+incompatible",
		},
	} {
		isLatest, err := isLatestVersion(ctx, testDB.db, test.modulePath, test.want)
		if err != nil {
			t.Fatal(err)
		}
		if !isLatest {
			t.Errorf("%s is not the latest version", test.want)
		}
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
	if _, err := testDB.GetModuleInfo(ctx, v.ModulePath, v.Version); err != nil {
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
	if _, err := testDB.GetModuleInfo(ctx, v.ModulePath, v.Version); !errors.Is(err, derrors.NotFound) {
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
		derrors.ToStatus(derrors.AlternativeModule), "example.com/mod", derrors.AlternativeModule, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := sample.Module(modulePath, okVersion, "p")
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, _, found := GetFromSearchDocuments(ctx, t, testDB, m.Packages()[0].Path); found {
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
