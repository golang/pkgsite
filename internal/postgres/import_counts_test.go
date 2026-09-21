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
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestUpdateSearchDocumentsImportedByCount(t *testing.T) {
	// Dont' run in parallel because it changes countBatchSize.
	ctx := context.Background()

	pkgPath := func(m *internal.Module) string { return m.Packages()[0].Path }

	// insert package with suffix at version, return the module
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
		if _, err := db.UpdateSearchDocumentsImportedByCount(ctx, batchSize); err != nil {
			t.Fatal(err)
		}
	}

	validateImportedByCountAndGetSearchDocument := func(t *testing.T, db *DB, path string, count int) *searchDocument {
		t.Helper()
		sd, err := getSearchDocument(ctx, db, path)
		if err != nil {
			t.Fatalf("testDB.getSearchDocument(ctx, %q): %v", path, err)
		}
		if sd.importedByCount > 0 && sd.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("importedByCountUpdatedAt for package %q should not be empty if count > 0", path)
		}
		if count != sd.importedByCount {
			t.Fatalf("importedByCount for package %q = %d; want = %d", path, sd.importedByCount, count)
		}
		return sd
	}

	t.Run("basic", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		// Test imported_by_count = 0 when only pkgA is added.
		mA := insertPackageVersion(t, testDB, "A", "v1.0.0", nil)
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 0)

		// Test imported_by_count = 1 for pkgA when pkgB is added.
		mB := insertPackageVersion(t, testDB, "B", "v1.0.0", []string{"A"})
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 1)
		sdB := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 0)
		wantSearchDocBUpdatedAt := sdB.importedByCountUpdatedAt

		// Test imported_by_count = 2 for pkgA, when C is added.
		mC := insertPackageVersion(t, testDB, "C", "v1.0.0", []string{"A"})
		updateImportedByCount(testDB, 100)
		sdA := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		sdC := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mC), 0)

		// Nothing imports C, so it has never been updated.
		if !sdC.importedByCountUpdatedAt.IsZero() {
			t.Fatalf("pkgC imported_by_count_updated_at should be zero, but is %v", sdC.importedByCountUpdatedAt)
		}
		if sdA.importedByCountUpdatedAt.IsZero() {
			t.Fatal("pkgA imported_by_count_updated_at should be non-zero, but is zero")
		}

		// Test imported_by_count_updated_at for B has not changed.
		sdB = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 0)
		if sdB.importedByCountUpdatedAt != wantSearchDocBUpdatedAt {
			t.Fatalf("expected imported_by_count_updated_at for pkgB not to have changed; old = %v, new = %v",
				wantSearchDocBUpdatedAt, sdB.importedByCountUpdatedAt)
		}

		// When an older version of A imports D, nothing happens to the counts,
		// because imports_unique only records the latest version of each package.
		mD := insertPackageVersion(t, testDB, "D", "v1.0.0", nil)
		insertPackageVersion(t, testDB, "A", "v0.9.0", []string{"D"})
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mD), 0)

		// When a newer version of A imports D, however, the counts do change.
		insertPackageVersion(t, testDB, "A", "v1.1.0", []string{"D"})
		updateImportedByCount(testDB, 100)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mD), 1)
	})

	t.Run("alternative", func(t *testing.T) {
		// Test with alternative modules that are removed from search_documents.
		testDB, release := acquire(t)
		defer release()

		insertPackageVersion(t, testDB, "B", "v1.0.0", nil)

		// Insert a package with the canonical module path.
		canonicalModule := insertPackageVersion(t, testDB, "A", "v1.0.0", []string{"B"})

		// Imagine we see a package with an alternative path at v1.2.0.
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
		if err := testDB.UpdateModuleVersionState(ctx, mvs); err != nil {
			t.Fatal(err)
		}

		// Now we see an earlier version of that package, without a go.mod file, so we insert it.
		// It should not get inserted into search_documents.
		mAlt := sample.Module(alternativeModulePath, "v1.0.0", "A")
		mAlt.Packages()[0].Imports = []string{"B"}
		testDB.MustInsertModule(t, mAlt)
		// Although B is imported by two packages, only one is in search_documents, so its
		// imported-by count is 1.
		updateImportedByCount(testDB, 100)
		validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/B/B", 1)
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
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		_ = validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mB), 1)
	})
	t.Run("module_sharing", func(t *testing.T) {
		t.Skip("module counts disabled")
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
		sdA := validateImportedByCountAndGetSearchDocument(t, testDB, pkgPath(mA), 2)
		if sdA.importedByModuleCount != 1 {
			t.Errorf("importedByModuleCount for A = %d, want 1", sdA.importedByModuleCount)
		}
		if sdA.importedByModuleCountUpdatedAt.IsZero() {
			t.Error("importedByModuleCountUpdatedAt for A is zero, want non-zero")
		}
	})
	t.Run("same_module", func(t *testing.T) {
		t.Skip("module counts disabled")
		testDB, release := acquire(t)
		defer release()

		// Module A has two packages, pkg1 and pkg2.
		// pkg1 imports pkg2 within the same module.
		mA := sample.Module("mod.com/A", "v1.0.0", "pkg1", "pkg2")
		for _, u := range mA.Units {
			if u.Path == "mod.com/A/pkg1" {
				u.Imports = []string{"mod.com/A/pkg2"}
			} else if u.IsPackage() {
				u.Imports = nil
			}
		}
		testDB.MustInsertModule(t, mA)

		updateImportedByCount(testDB, 100)
		sdPkg2 := validateImportedByCountAndGetSearchDocument(t, testDB, "mod.com/A/pkg2", 0)
		if sdPkg2.importedByModuleCount != 1 {
			t.Errorf("importedByModuleCount for pkg2 = %d, want 1", sdPkg2.importedByModuleCount)
		}
		if sdPkg2.importedByModuleCountUpdatedAt.IsZero() {
			t.Error("importedByModuleCountUpdatedAt for pkg2 is zero, want non-zero")
		}
	})
}
