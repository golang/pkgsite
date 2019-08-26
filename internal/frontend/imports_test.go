// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/sample"
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

			version := sample.Version()
			pkg := sample.Package()
			pkg.Imports = tc.imports
			version.Packages = []*internal.Package{pkg}

			if err := testDB.InsertVersion(ctx, version); err != nil {
				t.Fatal(err)
			}

			got, err := fetchImportsDetails(ctx, testDB, firstVersionedPackage(version))
			if err != nil {
				t.Fatalf("fetchModuleDetails(ctx, db, %q, %q) = %v err = %v, want %v",
					version.Packages[0].Path, version.Version, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = version.VersionInfo.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchModuleDetails(ctx, %q, %q) mismatch (-want +got):\n%s", version.Packages[0].Path, version.Version, diff)
			}
		})
	}
}

func TestFetchImportedByDetails(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	newVersion := func(modPath string, pkgs ...*internal.Package) *internal.Version {
		v := sample.Version()
		v.ModulePath = modPath
		v.Packages = pkgs
		return v
	}

	pkg1 := sample.Package()
	pkg1.Path = "path.to/foo/bar"

	pkg2 := sample.Package()
	pkg2.Path = "path2.to/foo/bar2"
	pkg2.Imports = []string{pkg1.Path}

	pkg3 := sample.Package()
	pkg3.Path = "path3.to/foo/bar3"
	pkg3.Imports = []string{pkg2.Path, pkg1.Path}

	testVersions := []*internal.Version{
		newVersion("path.to/foo", pkg1),
		newVersion("path2.to/foo", pkg2),
		newVersion("path3.to/foo", pkg3),
	}

	for _, v := range testVersions {
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		pkg         *internal.Package
		wantDetails *ImportedByDetails
	}{
		{
			pkg:         pkg3,
			wantDetails: &ImportedByDetails{},
		},
		{
			pkg: pkg2,
			wantDetails: &ImportedByDetails{
				ImportedBy: []string{pkg3.Path},
			},
		},
		{
			pkg: pkg1,
			wantDetails: &ImportedByDetails{
				ImportedBy: []string{pkg2.Path, pkg3.Path},
			},
		},
	} {
		t.Run(tc.pkg.Path, func(t *testing.T) {
			otherVersion := newVersion("path.to/foo", tc.pkg)
			otherVersion.Version = "v1.0.5"
			vp := firstVersionedPackage(otherVersion)
			got, err := fetchImportedByDetails(ctx, testDB, vp, paginationParams{limit: 20, page: 1})
			if err != nil {
				t.Fatalf("fetchImportedByDetails(ctx, db, %q, 1) = %v err = %v, want %v",
					tc.pkg.Path, got, err, tc.wantDetails)
			}

			tc.wantDetails.ModulePath = vp.VersionInfo.ModulePath
			if diff := cmp.Diff(tc.wantDetails, got, cmpopts.IgnoreFields(ImportedByDetails{}, "Pagination")); diff != "" {
				t.Errorf("fetchImportedByDetails(ctx, db, %q, 1) mismatch (-want +got):\n%s", tc.pkg.Path, diff)
			}
		})
	}
}
