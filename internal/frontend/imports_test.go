// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestFetchImportsDetails(t *testing.T) {
	for _, tc := range []struct {
		name        string
		imports     []string
		wantDetails *ImportsDetails
	}{
		{
			name: "want imports details with standard and internal",
			imports: []string{
				"pa.th/import/1",
				sample.PackagePath,
				"context",
			},
			wantDetails: &ImportsDetails{
				ExternalImports: []string{"pa.th/import/1"},
				InternalImports: []string{sample.PackagePath},
				StdLib:          []string{"context"},
			},
		},
		{
			name:    "want expected imports details with multiple",
			imports: []string{"pa.th/import/1", "pa.th/import/2", "pa.th/import/3"},
			wantDetails: &ImportsDetails{
				ExternalImports: []string{"pa.th/import/1", "pa.th/import/2", "pa.th/import/3"},
				StdLib:          nil,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			module := sample.Module(sample.ModulePath, sample.VersionString, sample.Suffix)
			// The first unit is the module and the second one is the package.
			pkg := module.Units[1]
			pkg.Imports = tc.imports

			if err := testDB.InsertModule(ctx, module); err != nil {
				t.Fatal(err)
			}

			got, err := fetchImportsDetails(ctx, testDB, pkg.Path, pkg.ModulePath, pkg.Version)
			if err != nil {
				t.Fatalf("fetchImportsDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					module.Units[1].Path, module.Version, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = module.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchImportsDetails(ctx, %q, %q) mismatch (-want +got):\n%s", module.Units[1].Path, module.Version, diff)
			}
		})
	}
}

func TestFetchImportedByDetails(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	newModule := func(modPath string, pkgs ...*internal.Unit) *internal.Module {
		m := sample.Module(modPath, sample.VersionString)
		for _, p := range pkgs {
			sample.AddUnit(m, p)
		}
		return m
	}

	pkg1 := sample.UnitForPackage("path.to/foo/bar", "path.to/foo", sample.VersionString, "bar", true)
	pkg2 := sample.UnitForPackage("path2.to/foo/bar2", "path2.to/foo", sample.VersionString, "bar2", true)
	pkg2.Imports = []string{pkg1.Path}

	pkg3 := sample.UnitForPackage("path3.to/foo/bar3", "path3.to/foor", sample.VersionString, "bar3", true)
	pkg3.Imports = []string{pkg2.Path, pkg1.Path}

	testModules := []*internal.Module{
		newModule("path.to/foo", pkg1),
		newModule("path2.to/foo", pkg2),
		newModule("path3.to/foo", pkg3),
	}

	for _, m := range testModules {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		pkg         *internal.Unit
		wantDetails *ImportedByDetails
	}{
		{
			pkg:         pkg3,
			wantDetails: &ImportedByDetails{TotalIsExact: true},
		},
		{
			pkg: pkg2,
			wantDetails: &ImportedByDetails{
				ImportedBy:   []*Section{{Prefix: pkg3.Path, NumLines: 0}},
				Total:        1,
				TotalIsExact: true,
			},
		},
		{
			pkg: pkg1,
			wantDetails: &ImportedByDetails{
				ImportedBy: []*Section{
					{Prefix: pkg2.Path, NumLines: 0},
					{Prefix: pkg3.Path, NumLines: 0},
				},
				Total:        2,
				TotalIsExact: true,
			},
		},
	}

	checkFetchImportedByDetails := func(ctx context.Context, pkg *internal.Unit, wantDetails *ImportedByDetails) {
		got, err := fetchImportedByDetails(ctx, testDB, pkg.Path, pkg.ModulePath)
		if err != nil {
			t.Fatalf("fetchImportedByDetails(ctx, db, %q) = %v err = %v, want %v",
				pkg.Path, got, err, wantDetails)
		}
		wantDetails.ModulePath = pkg.ModulePath
		if diff := cmp.Diff(wantDetails, got); diff != "" {
			t.Errorf("fetchImportedByDetails(ctx, db, %q) mismatch (-want +got):\n%s", pkg.Path, diff)
		}
	}
	for _, test := range tests {
		t.Run(test.pkg.Path, func(t *testing.T) {
			otherVersion := newModule(path.Dir(test.pkg.Path), test.pkg)
			otherVersion.Version = "v1.0.5"
			pkg := otherVersion.Units[1]

			t.Run("no experiments "+test.pkg.Name, func(t *testing.T) {
				checkFetchImportedByDetails(ctx, pkg, test.wantDetails)
			})
			t.Run("get imported by from search_documents "+test.pkg.Name, func(t *testing.T) {
				ctx := experiment.NewContext(ctx, internal.ExperimentGetUnitWithOneQuery)
				testDB.UpdateSearchDocumentsImportedByCount(ctx)
				checkFetchImportedByDetails(ctx, pkg, test.wantDetails)
			})
		})
	}
}
