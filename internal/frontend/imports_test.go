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
	for _, test := range []struct {
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
		t.Run(test.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			module := sample.Module(sample.ModulePath, sample.VersionString, sample.Suffix)
			// The first unit is the module and the second one is the package.
			pkg := module.Units[1]
			pkg.Imports = test.imports

			postgres.MustInsertModuleLatest(ctx, t, testDB, module)

			got, err := fetchImportsDetails(ctx, testDB, pkg.Path, pkg.ModulePath, pkg.Version)
			if err != nil {
				t.Fatalf("fetchImportsDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					module.Units[1].Path, module.Version, got, err, test.wantDetails)
			}

			test.wantDetails.ModulePath = module.ModulePath
			if diff := cmp.Diff(test.wantDetails, got); diff != "" {
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
		postgres.MustInsertModuleLatest(ctx, t, testDB, m)
	}

	tests := []struct {
		pkg         *internal.Unit
		wantDetails *ImportedByDetails
	}{
		{
			pkg: pkg3,
			wantDetails: &ImportedByDetails{
				NumImportedByDisplay: "0",
			},
		},
		{
			pkg: pkg2,
			wantDetails: &ImportedByDetails{
				ImportedBy:           []*Section{{Prefix: pkg3.Path, NumLines: 0}},
				NumImportedByDisplay: "0 (1 including internal and invalid packages)",
				Total:                1,
			},
		},
		{
			pkg: pkg1,
			wantDetails: &ImportedByDetails{
				ImportedBy: []*Section{
					{Prefix: pkg2.Path, NumLines: 0},
					{Prefix: pkg3.Path, NumLines: 0},
				},
				NumImportedByDisplay: "0 (2 including internal and invalid packages)",
				Total:                2,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.pkg.Path, func(t *testing.T) {
			otherVersion := newModule(path.Dir(test.pkg.Path), test.pkg)
			otherVersion.Version = "v1.0.5"
			pkg := otherVersion.Units[1]
			checkFetchImportedByDetails(ctx, t, pkg, test.wantDetails)
		})
	}
}

func TestFetchImportedByDetails_ExceedsLimit(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	old := importedByLimit
	importedByLimit = 3
	defer func() { importedByLimit = old }()

	m := sample.Module("m.com/a", sample.VersionString, "foo")
	postgres.MustInsertModuleLatest(ctx, t, testDB, m)
	for _, mod := range []string{"m1.com/a", "m2.com/a", "m3.com/a"} {
		m2 := sample.Module(mod, sample.VersionString, "p")
		m2.Packages()[0].Imports = []string{"m.com/a/foo"}
		postgres.MustInsertModuleLatest(ctx, t, testDB, m2)
	}
	wantDetails := &ImportedByDetails{
		ModulePath: "m.com/a",
		ImportedBy: []*Section{
			{Prefix: "m1.com/a/p"},
			{Prefix: "m2.com/a/p"},
		},

		NumImportedByDisplay: "0 (more than 2 including internal and invalid packages)",
		Total:                3,
	}
	checkFetchImportedByDetails(ctx, t, m.Packages()[0], wantDetails)
}

func checkFetchImportedByDetails(ctx context.Context, t *testing.T, pkg *internal.Unit, wantDetails *ImportedByDetails) {
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
