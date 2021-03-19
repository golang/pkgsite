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
	t.Parallel()
	ctx := context.Background()

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
			name:   "valid test for prerelease version",
			module: sample.Module(sample.ModulePath, "v1.0.0-beta", "internal/foo"),
		},
		{
			name:   "valid test for pseudoversion version",
			module: sample.Module(sample.ModulePath, "v0.0.0-20210212193344-7015347762c1", "internal/foo"),
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
			module: sample.Module(stdlib.ModulePath, "v1.12.5", "context"),
		},
		{
			name: "deprecated",
			module: func() *internal.Module {
				m := sample.DefaultModule()
				m.Deprecated = true
				m.DeprecationComment = "use v2"
				return m
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()

			MustInsertModule(ctx, t, testDB, test.module)
			// Test that insertion of duplicate primary key won't fail.
			MustInsertModule(ctx, t, testDB, test.module)
			checkModule(ctx, t, testDB, test.module)
		})
	}
}

func checkModule(ctx context.Context, t *testing.T, db *DB, want *internal.Module) {
	got, err := db.GetModuleInfo(ctx, want.ModulePath, want.Version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want.ModuleInfo, *got, cmp.AllowUnexported(source.Info{})); diff != "" {
		t.Fatalf("testDB.GetModuleInfo(%q, %q) mismatch (-want +got):\n%s", want.ModulePath, want.Version, diff)
	}

	for _, wantu := range want.Units {
		got, err := db.GetUnit(ctx, &wantu.UnitMeta, internal.AllFields)
		if err != nil {
			t.Fatal(err)
		}
		var subdirectories []*internal.PackageMeta
		for _, u := range want.Units {
			if u.IsPackage() && (strings.HasPrefix(u.Path, wantu.Path) || wantu.Path == stdlib.ModulePath) {
				subdirectories = append(subdirectories, packageMetaFromUnit(u))
			}
		}
		wantu.Subdirectories = subdirectories
		opts := cmp.Options{
			cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage", "OldCoverage"),
			cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
		}
		if diff := cmp.Diff(wantu, got, opts); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func packageMetaFromUnit(u *internal.Unit) *internal.PackageMeta {
	return &internal.PackageMeta{
		Path:              u.Path,
		IsRedistributable: u.IsRedistributable,
		Name:              u.Name,
		Synopsis:          u.Documentation[0].Synopsis,
		Licenses:          u.Licenses,
	}
}

func TestInsertModuleLicenseCheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass=%t", bypass), func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()

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
			checkHasRedistData(mod.Units[0].Readme.Contents, mod.Units[0].Documentation[0].Source, true)
			mod.IsRedistributable = false
			mod.Units[0].IsRedistributable = false

			MustInsertModule(ctx, t, db, mod)

			// New model
			u, err := db.GetUnit(ctx, newUnitMeta(mod.ModulePath, mod.ModulePath, mod.Version), internal.AllFields)
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
				source = u.Documentation[0].Source
			}
			checkHasRedistData(readme, source, bypass)
		})
	}
}

func TestUpsertModule(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	m := sample.Module("upsert.org", "v1.2.3", "dir/p")

	// Insert the module.
	MustInsertModule(ctx, t, testDB, m)
	// Change the module, and re-insert.
	m.IsRedistributable = !m.IsRedistributable
	lic := *m.Licenses[0]
	lic.Contents = append(lic.Contents, " and more"...)
	sample.ReplaceLicense(m, &lic)
	m.Units[0].Readme.Contents += " and more"

	MustInsertModule(ctx, t, testDB, m)
	// The changes should have been saved.
	checkModule(ctx, t, testDB, m)
}

func TestInsertModuleErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

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
			testDB, release := acquire(t)
			defer release()

			if _, err := testDB.InsertModule(ctx, test.module, nil); !errors.Is(err, test.wantWriteErr) {
				t.Errorf("error: %v, want write error: %v", err, test.wantWriteErr)
			}
		})
	}
}

func TestInsertModuleNewCoverage(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	m := sample.DefaultModule()
	newCoverage := licensecheck.Coverage{
		Percent: 100,
		Match:   []licensecheck.Match{{ID: "New", Start: 1, End: 10}},
	}
	m.Licenses = []*licenses.License{
		{
			Metadata: &licenses.Metadata{
				Types:    []string{sample.LicenseType},
				FilePath: sample.LicenseFilePath,
				Coverage: newCoverage,
			},
			Contents: []byte(`Lorem Ipsum`),
		},
	}
	MustInsertModule(ctx, t, testDB, m)
	u, err := testDB.GetUnit(ctx, newUnitMeta(m.ModulePath, m.ModulePath, m.Version), internal.AllFields)
	if err != nil {
		t.Fatal(err)
	}
	got := u.LicenseContents[0].Metadata
	want := &licenses.Metadata{
		Types:    []string{"MIT"},
		FilePath: sample.LicenseFilePath,
		Coverage: newCoverage,
	}
	if !cmp.Equal(got, want) {
		t.Errorf("\ngot  %+v\nwant %+v", got, want)
	}

}

func TestPostgres_ReadAndWriteModuleOtherColumns(t *testing.T) {
	t.Parallel()
	// Verify that InsertModule correctly populates the columns in the versions
	// table that are not in the ModuleInfo struct.
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	type other struct {
		sortVersion, seriesPath string
	}

	v := sample.Module("github.com/user/repo/path/v2", "v1.2.3-beta.4.a", sample.Suffix)
	want := other{
		sortVersion: "1,2,3,~beta,4,~a",
		seriesPath:  "github.com/user/repo/path",
	}

	MustInsertModule(ctx, t, testDB, v)
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

func TestInsertModuleLatest(t *testing.T) {
	// Check the first return value of InsertModule, which is whether the
	// inserted module is the latest good version. Also check that
	// latest_module_versions.good_version is populated correctly.
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx := context.Background()

	// These tests are cumulative: actions of earlier tests may affect later ones.
	for _, test := range []struct {
		version        string
		cooked         string // The latest cooked version, affects incompatible versions; empty => nil lmv
		retract        string // body of go.mod retract statement
		wantIsLatest   bool
		wantLatestGood string
	}{
		{
			version:      "v2.0.0+incompatible",
			cooked:       "v2.0.0+incompatible",
			wantIsLatest: true, // The only version, so the latest.
		},
		{
			version: "v1.5.2",
			cooked:  "v1.9.0",
			// Compatible version is later than incompatible.
			wantIsLatest: true,
		},
		{
			version: "v1.5.1",
			cooked:  "v1.9.0",
			// An earlier version; not the latest.
			wantIsLatest:   false,
			wantLatestGood: "v1.5.2",
		},
		{
			version:      "v1.4.0",
			cooked:       "v1.9.1",           // ignore incompatible
			retract:      "[v1.5.0, v1.6.0]", // all versions except this retracted
			wantIsLatest: true,
		},
		{
			version:        "v1.4.1",
			cooked:         "v1.9.2",           // ignore incompatible
			retract:        "[v1.4.0, v1.6.0]", // all versions retracted, even this one
			wantIsLatest:   false,
			wantLatestGood: "",
		},
	} {
		t.Run(test.version, func(t *testing.T) {
			m := sample.DefaultModule()
			m.Version = test.version
			var lmv *internal.LatestModuleVersions
			if test.cooked != "" {
				gomod := "module " + m.ModulePath
				if test.retract != "" {
					gomod += "\nretract " + test.retract
				}
				lmv = addLatest(ctx, t, testDB, m.ModulePath, test.cooked, gomod)
			}
			gotIsLatest, err := testDB.InsertModule(ctx, m, lmv)
			if err != nil {
				t.Fatal(err)
			}
			if gotIsLatest != test.wantIsLatest {
				t.Errorf("got latest %t, want %t", gotIsLatest, test.wantIsLatest)
			}
			gotLMV, err := testDB.GetLatestModuleVersions(ctx, m.ModulePath)
			if err != nil {
				t.Fatal(err)
			}
			if gotLMV == nil {
				t.Fatal("gotLMV is nil")
			}
			wantLatestGood := test.wantLatestGood
			if wantLatestGood == "" && test.wantIsLatest {
				wantLatestGood = test.version
			}
			if gotLMV.GoodVersion != wantLatestGood {
				t.Errorf("got good version %q, want %q", gotLMV.GoodVersion, wantLatestGood)
			}
		})
	}
}

func TestDeleteModule(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	v := sample.DefaultModule()

	MustInsertModule(ctx, t, testDB, v)
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
	t.Parallel()
	// Verify that packages are not added to search_documents if the module has a newer
	// alternative version.
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	const (
		altVersion = "v1.2.0"
		okVersion  = "v1.0.0"
	)

	mvs := &ModuleVersionStateForUpsert{
		ModulePath: "example.com/Mod",
		Version:    altVersion,
		AppVersion: "appVersion",
		Timestamp:  time.Now(),
		Status:     derrors.ToStatus(derrors.AlternativeModule),
		GoModPath:  "example.com/mod",
		FetchErr:   derrors.AlternativeModule,
	}
	addLatest(ctx, t, testDB, mvs.ModulePath, altVersion, "")
	if err := testDB.UpsertModuleVersionState(ctx, mvs); err != nil {
		t.Fatal(err)
	}
	m := sample.Module(mvs.ModulePath, okVersion, "p")
	MustInsertModule(ctx, t, testDB, m)
	if _, _, found := GetFromSearchDocuments(ctx, t, testDB, m.Packages()[0].Path); found {
		t.Fatal("found package after inserting")
	}
}

func TestMakeValidUnicode(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

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
	t.Parallel()
	// Verify that two transactions cannot both hold the same lock, but that every one
	// that wants the lock eventually gets it.
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

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

func TestIsAlternativeModulePath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	const modulePath = "m.com/a"
	altModuleStatus := derrors.ToStatus(derrors.AlternativeModule)
	// These tests form a sequence. Each test's state affects the next's.
	for _, test := range []struct {
		version string
		status  int
		want    bool
	}{
		{"v1.2.0", 200, false},                 // no 491s
		{"v1.1.0", altModuleStatus, false},     // 491 is earlier
		{"v1.3.0-pre", altModuleStatus, false}, // still earlier: release beats pre-release
		{"v1.3.0", altModuleStatus, true},      // latest version is 491
		{"v1.4.0", 200, false},                 // new latest version is OK
		{"v1.5.0", altModuleStatus, true},      // "I can do this all day." --Captain America
	} {
		addLatest(ctx, t, testDB, modulePath, test.version, "")
		if err := testDB.UpsertModuleVersionState(ctx, &ModuleVersionStateForUpsert{
			ModulePath: modulePath,
			Version:    test.version,
			AppVersion: "appVersion",
			Timestamp:  time.Now(),
			Status:     test.status,
			HasGoMod:   true,
		}); err != nil {
			t.Fatal(err)
		}

		got, err := isAlternativeModulePath(ctx, testDB.db, modulePath)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("%q, %d: got %t, want %t", test.version, test.status, got, test.want)
		}
	}
}
