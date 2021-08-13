// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

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

func TestDeleteFromSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	const modulePath = "deleteme.com"

	initial := []searchDocumentRow{
		{modulePath + "/p1", modulePath, "v0.0.9", 0}, // oldest version of same module
		{modulePath + "/p2", modulePath, "v1.1.0", 0}, // older version of same module
		{modulePath + "/p4", modulePath, "v1.9.0", 0}, // newer version of same module
		{"other.org/p2", "other.org", "v1.1.0", 0},    // older version of a different module
	}

	insertInitial := func(db *DB) {
		t.Helper()
		for _, r := range initial {
			sm := sample.Module(r.ModulePath, r.Version, strings.TrimPrefix(r.PackagePath, r.ModulePath+"/"))
			MustInsertModule(ctx, t, db, sm)
		}
		// Older versions are deleted by InsertModule, so only the newest versions are here.
		checkSearchDocuments(ctx, t, db, initial[2:])
	}

	t.Run("DeleteOlderVersionFromSearchDocuments", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()
		insertInitial(testDB)

		if err := testDB.DeleteOlderVersionFromSearchDocuments(ctx, modulePath, "v1.2.3"); err != nil {
			t.Fatal(err)
		}

		checkSearchDocuments(ctx, t, testDB, []searchDocumentRow{
			{modulePath + "/p4", modulePath, "v1.9.0", 0}, // newer version not deleted
			{"other.org/p2", "other.org", "v1.1.0", 0},    // other module not deleted
		})
	})
	t.Run("deleteModuleFromSearchDocuments", func(t *testing.T) {
		// Non-empty list of packages tested above. This tests an empty list.
		testDB, release := acquire(t)
		defer release()
		insertInitial(testDB)

		if err := deleteModuleOrPackagesInModuleFromSearchDocuments(ctx, testDB.db, modulePath, nil); err != nil {
			t.Fatal(err)
		}
		checkSearchDocuments(ctx, t, testDB, []searchDocumentRow{
			{"other.org/p2", "other.org", "v1.1.0", 0}, // other module not deleted
		})
	})
	t.Run("deleteOtherModulePackagesFromSearchDocuments", func(t *testing.T) {
		testDB, release := acquire(t)
		defer release()

		// Insert a module with two packages.
		m0 := sample.Module(modulePath, "v1.0.0", "p1", "p2")
		MustInsertModule(ctx, t, testDB, m0)
		// Set the imported-by count to a non-zero value so we can tell which
		// rows were deleted.
		if _, err := testDB.db.Exec(ctx, `UPDATE search_documents SET imported_by_count = 1`); err != nil {
			t.Fatal(err)
		}
		// Later version of module does not have p1.
		m1 := sample.Module(modulePath, "v1.1.0", "p2", "p3")
		MustInsertModule(ctx, t, testDB, m1)

		// p1 should be gone, p2 should be there with the same
		// imported_by_count, and p3 should be there with a zero count.
		want := []searchDocumentRow{
			{modulePath + "/p2", modulePath, "v1.1.0", 1},
			{modulePath + "/p3", modulePath, "v1.1.0", 0},
		}
		checkSearchDocuments(ctx, t, testDB, want)
	})
}

type searchDocumentRow struct {
	PackagePath, ModulePath, Version string
	ImportedByCount                  int
}

func readSearchDocuments(ctx context.Context, db *DB) ([]searchDocumentRow, error) {
	var rows []searchDocumentRow
	err := db.db.CollectStructs(ctx, &rows, `SELECT package_path, module_path, version, imported_by_count FROM search_documents`)
	if err != nil {
		return nil, err
	}
	return rows, err
}

func checkSearchDocuments(ctx context.Context, t *testing.T, db *DB, want []searchDocumentRow) {
	t.Helper()
	got, err := readSearchDocuments(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	less := func(r1, r2 searchDocumentRow) bool {
		if r1.PackagePath != r2.PackagePath {
			return r1.PackagePath < r2.PackagePath
		}
		if r1.ModulePath != r2.ModulePath {
			return r1.ModulePath < r2.ModulePath
		}
		return r1.Version < r2.Version
	}
	sort.Slice(got, func(i, j int) bool { return less(got[i], got[j]) })
	sort.Slice(want, func(i, j int) bool { return less(want[i], want[j]) })
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("search documents mismatch (-want, +got):\n%s", diff)
	}
}

func TestDeletePseudoversionsExcept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testDB, release := acquire(t)
	defer release()

	pseudo1 := "v0.0.0-20190904010203-89fb59e2e920"
	versions := []string{
		sample.VersionString,
		pseudo1,
		"v0.0.0-20190904010203-89fb59e2e920",
		"v0.0.0-20190904010203-89fb59e2e920",
	}
	for _, v := range versions {
		MustInsertModule(ctx, t, testDB, sample.Module(sample.ModulePath, v, ""))
	}
	if err := testDB.DeletePseudoversionsExcept(ctx, sample.ModulePath, pseudo1); err != nil {
		t.Fatal(err)
	}
	mods, err := getPathVersions(ctx, testDB, sample.ModulePath, version.TypeRelease)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 && mods[0].Version != sample.VersionString {
		t.Errorf("module version %q was not found", sample.VersionString)
	}
	mods, err = getPathVersions(ctx, testDB, sample.ModulePath, version.TypePseudo)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("pseudoversions expected to be deleted were not")
	}
	if mods[0].Version != pseudo1 {
		t.Errorf("got %q; want %q", mods[0].Version, pseudo1)
	}
}
