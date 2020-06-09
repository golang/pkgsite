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
			module.LegacyPackages[0].Imports = tc.imports

			if err := testDB.InsertModule(ctx, module); err != nil {
				t.Fatal(err)
			}

			pkg := firstVersionedPackage(module)
			got, err := fetchImportsDetails(ctx, testDB, pkg.Path, pkg.ModulePath, pkg.Version)
			if err != nil {
				t.Fatalf("fetchImportsDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					module.LegacyPackages[0].Path, module.Version, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = module.LegacyModuleInfo.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchImportsDetails(ctx, %q, %q) mismatch (-want +got):\n%s", module.LegacyPackages[0].Path, module.Version, diff)
			}
		})
	}
}

// firstVersionedPackage is a helper function that returns an
// *internal.LegacyVersionedPackage corresponding to the first package in the
// version.
func firstVersionedPackage(m *internal.Module) *internal.LegacyVersionedPackage {
	return &internal.LegacyVersionedPackage{
		LegacyPackage:    *m.LegacyPackages[0],
		LegacyModuleInfo: m.LegacyModuleInfo,
	}
}

func TestFetchImportedByDetails(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	newModule := func(modPath string, pkgs ...*internal.LegacyPackage) *internal.Module {
		m := sample.Module(modPath, sample.VersionString)
		for _, p := range pkgs {
			sample.AddPackage(m, p)
		}
		return m
	}

	pkg1 := sample.LegacyPackage("path.to/foo", "bar")
	pkg2 := sample.LegacyPackage("path2.to/foo", "bar2")
	pkg2.Imports = []string{pkg1.Path}

	pkg3 := sample.LegacyPackage("path3.to/foo", "bar3")
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

	for _, tc := range []struct {
		pkg         *internal.LegacyPackage
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
	} {
		t.Run(tc.pkg.Path, func(t *testing.T) {
			otherVersion := newModule(path.Dir(tc.pkg.Path), tc.pkg)
			otherVersion.Version = "v1.0.5"
			vp := firstVersionedPackage(otherVersion)
			got, err := fetchImportedByDetails(ctx, testDB, vp.Path, vp.ModulePath)
			if err != nil {
				t.Fatalf("fetchImportedByDetails(ctx, db, %q) = %v err = %v, want %v",
					tc.pkg.Path, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = vp.LegacyModuleInfo.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchImportedByDetails(ctx, db, %q) mismatch (-want +got):\n%s", tc.pkg.Path, diff)
			}
		})
	}
}
