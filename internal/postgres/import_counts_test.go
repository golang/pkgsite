// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestUpdateSearchDocumentsImportedByCount(t *testing.T) {
	// Dont' run in parallel because it changes countBatchSize.
	ctx := context.Background()

	defer func(old time.Duration) { moduleCountUpdateDuration = old }(moduleCountUpdateDuration)
	moduleCountUpdateDuration = 0 // Always update module counts.

	pkgPath := func(m *internal.Module) string { return m.Packages()[0].Path }

	// Insert a package into a new module "mod.com"/suffix@version, and return the module.
	insertPackageVersion := func(t *testing.T, db *DB, suffix, version string, imports []string) *internal.Module {
		t.Helper()
		m := sample.Module("mod.com/"+suffix, version, suffix)
		// Units[0] is the module itself.
		pkg := m.Units[1]
		pkg.Imports = nil
		for _, imp := range imports {
			pkg.Imports = append(pkg.Imports, fmt.Sprintf("mod.com/%s/%[1]s", imp))
		}
		db.MustInsertModule(t, m)
		return m
	}

	updateImportedByCount := func(db *DB, batchSize int) {
		t.Helper()
		if _, _, err := db.UpdateSearchDocumentsImportedByCount(ctx, batchSize); err != nil {
			t.Fatal(err)
		}
	}

	validateImportedByCountAndGetSearchDocument := func(t *testing.T, db *DB, path string, pcount, mcount int) *searchDocument {
		t.Helper()
		sd, err := getSearchDocument(ctx, db, path)
		if err != nil {
			t.Fatalf("testDB.getSearchDocument(ctx, %q): %v", path, err)
		}
		if sd.importedByCount > 0 && sd.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("importedByCountUpdatedAt for package %q should not be empty if count > 0", path)
		}
		if pcount != sd.importedByCount {
			t.Fatalf("importedByCount for package %q = %d; want = %d", path, sd.importedByCount, pcount)
		}
		if sd.importedByModuleCountUpdatedAt.IsZero() {
			t.Fatalf("importedByModuleCountUpdatedAt for package %q should not be zero", path)
		}
		if mcount != sd.importedByModuleCount {
			t.Fatalf("importedByModuleCount for package %q = %d; want = %d", path, sd.importedByModuleCount, mcount)
		}
		return sd
	}

	t.Run("basic", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		// Test imported_by_count = 0 when only pkgA is added.
		mA := insertPackageVersion(t, testDB, "A", "v1.0.0", nil)
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 0, 0)

		// Test imported_by_count = 1 for pkgA when pkgB is added.
		mB := insertPackageVersion(t, testDB, "B", "v1.0.0", []string{"A"})
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 1, 1)
		sdB := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 0, 0)
		wantSearchDocBUpdatedAt := sdB.importedByCountUpdatedAt

		// Test imported_by_count = 2 for pkgA, when C is added.
		mC := insertPackageVersion(t, testDB, "C", "v1.0.0", []string{"A"})
		updateImportedByCount(testDB, 100)
		sdA := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2, 2)
		sdC := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mC), 0, 0)

		// Nothing imports C, so it has never been updated.
		if !sdC.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("pkgC imported_by_count_updated_at should be zero, but is %v", sdC.importedByCountUpdatedAt)
		}
		if sdA.importedByCountUpdatedAt.IsZero() {
			t.Fatal("pkgA imported_by_count_updated_at should be non-zero, but is zero")
		}

		// Test imported_by_count_updated_at for B has not changed.
		sdB = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 0, 0)
		if sdB.importedByCountUpdatedAt != wantSearchDocBUpdatedAt {
			t.Fatalf("expected imported_by_count_updated_at for pkgB not to have changed; old = %v, new = %v",
				wantSearchDocBUpdatedAt, sdB.importedByCountUpdatedAt)
		}

		// When an older version of A imports D, nothing happens to the counts,
		// because imports_unique only records the latest version of each package.
		mD := insertPackageVersion(t, testDB, "D", "v1.0.0", nil)
		insertPackageVersion(t, testDB, "A", "v0.9.0", []string{"D"})
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2, 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mD), 0, 0)

		// When a newer version of A imports D, however, the counts do change.
		insertPackageVersion(t, testDB, "A", "v1.1.0", []string{"D"})
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2, 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mD), 1, 1)
	})

	t.Run("alternative", func(t *testing.T) {
		// Test with alternative modules that are removed from search_documents.
		testDB, release := acquire(t)
		defer release()

		insertPackageVersion(t, testDB, "B", "v1.0.0", nil) // mod.com/B with package mod.com/B/B
		insertPackageVersion(t, testDB, "C", "v1.0.0", nil) // mod.com/C wwith package mod.com/C/C

		// Insert a package with the canonical module path.
		canonicalModule := insertPackageVersion(t, testDB, "A", "v1.0.0", []string{"B"})
		// mod.com/A/A imports mod.com/B/B.

		// Now we see a package with an alternative path at v1.2.0.
		// We add that information to module_version_states.
		alternativeModulePath := strings.ToLower(canonicalModule.ModulePath)
		alternativeStatus := derrors.ToStatus(derrors.AlternativeModule)
		mvs := &ModuleVersionStateForUpdate{
			ModulePath: alternativeModulePath,
			Version:    "v1.2.0",
			Timestamp:  time.Now(),
			Status:     alternativeStatus,
			GoModPath:  canonicalModule.ModulePath,
		}
		if err := testDB.InsertIndexVersions(ctx, []*internal.IndexVersion{
			{
				Path:      mvs.ModulePath,
				Version:   mvs.Version,
				Timestamp: mvs.Timestamp,
			},
		}); err != nil {
			t.Fatal(err)
		}
		addLatest(ctx, t, testDB, mvs.ModulePath, mvs.Version, "")
		if err := testDB.UpdateModuleVersionState(ctx, mvs); err != nil {
			t.Fatal(err)
		}

		// Now we see an earlier version of that package, without a go.mod file, so we insert it.
		// saveModule inserts it into imports_unique, but skips search_documents.
		mAlt := sample.Module(alternativeModulePath, "v1.0.0", "A")
		mAlt.LatestVersion = mvs.Version
		mAlt.Packages()[0].Imports = []string{"mod.com/B/B", "mod.com/C/C"}
		testDB.MustInsertModule(t, mAlt)
		// Although B is in imports_unique as imported by both mod.com/A and mod.com/a,
		// only mod.com/A is in search_documents, so both its imported-by counts are 1.
		// C is only imported by mod.com/a, so both its imported-by counts are 0.
		updateImportedByCount(testDB, 100)
		validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/B/B", 1, 1)
		validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/C/C", 0, 0)
	})
	t.Run("multiple", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		// Two modules with importers.
		mA := insertPackageVersion(t, testDB, "A", "v1.0.0", nil)
		mB := insertPackageVersion(t, testDB, "B", "v1.0.0", nil)
		insertPackageVersion(t, testDB, "C", "v1.0.0", []string{"A"})
		insertPackageVersion(t, testDB, "D", "v1.0.0", []string{"A"})
		insertPackageVersion(t, testDB, "E", "v1.0.0", []string{"B"})

		updateImportedByCount(testDB, 1)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2, 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 1, 1)
	})
	t.Run("module_sharing", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		mA := insertPackageVersion(t, testDB, "A", "v1.0.0", nil)

		// Module C has two packages, C1 and C2, both importing A.
		mC := sample.Module("mod.com/C", "v1.0.0", "C1", "C2")
		for _, u := range mC.Units {
			if u.Path != "mod.com/C" {
				u.Imports = []string{pkgPath(mA)}
			}
		}
		testDB.MustInsertModule(t, mC)

		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2, 1)
	})
	t.Run("same_module", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		// Module A has a root package ("mod.com/A") and two subpackages, pkg1 and pkg2.
		// Module AB ("mod.com/AB") has a module path that has "mod.com/A" as a string
		// prefix, but is a distinct module.
		// pkg1 imports the root package mod.com/A, sibling mod.com/A/pkg2, and mod.com/AB/pkg.
		mA := sample.Module("mod.com/A", "v1.0.0", "", "pkg1", "pkg2")
		for _, u := range mA.Units {
			if u.Path == "mod.com/A/pkg1" {
				u.Imports = []string{"mod.com/A", "mod.com/A/pkg2", "mod.com/AB/pkg"}
			} else if u.IsPackage() {
				u.Imports = nil
			}
		}
		testDB.MustInsertModule(t, mA)

		mAB := sample.Module("mod.com/AB", "v1.0.0", "pkg")
		mAB.Packages()[0].Imports = nil
		testDB.MustInsertModule(t, mAB)

		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/A", 0, 1)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/A/pkg2", 0, 1)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/AB/pkg", 1, 1)
	})
	t.Run("stdlib", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		// Standard library package net/http imports fmt (both in module "std").
		// External package mod.com/A/A also imports fmt.
		mStd := sample.Module(stdlib.ModulePath, "v1.12.5", "fmt", "net/http")
		for _, u := range mStd.Units {
			switch u.Path {
			case "net/http":
				u.Imports = []string{"fmt"}
			case "fmt":
				u.Imports = nil
			}
		}
		testDB.MustInsertModule(t, mStd)

		mA := sample.Module("mod.com/A", "v1.0.0", "A")
		mA.Packages()[0].Imports = []string{"fmt"}
		testDB.MustInsertModule(t, mA)

		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, "net/http", 0, 0)
		// For fmt, net/http is excluded from package count (same module "std"),
		// while mod.com/A/A is counted. Both modules ("std" and "mod.com/A") are
		// counted in imported_by_module_count.
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, "fmt", 1, 2)
	})
}
