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

			version := sample.Version(func(v *internal.Version) {
				pkg := sample.Package()
				pkg.Imports = tc.imports
				v.Packages = []*internal.Package{pkg}
			})
			if err := testDB.InsertVersion(ctx, version, sample.Licenses); err != nil {
				t.Fatalf("db.InsertVersion(ctx, %v, %v): %v", version, sample.Licenses, err)
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

	var (
		pkg1    = sample.Package(sample.WithPath("path.to/foo/bar"))
		pkg2    = sample.Package(sample.WithPath("path2.to/foo/bar2"), sample.WithImports(pkg1.Path))
		pkg3    = sample.Package(sample.WithPath("path3.to/foo/bar3"), sample.WithImports(pkg2.Path, pkg1.Path))
		sampler = sample.VersionSampler(func() *internal.Version {
			return sample.Version(sample.WithModulePath("path.to/foo"))
		})
		testVersions = []*internal.Version{
			sampler.Sample(sample.WithPackages(pkg1)),
			sampler.Sample(sample.WithModulePath("path2.to/foo"), sample.WithPackages(pkg2)),
			sampler.Sample(sample.WithModulePath("path3.to/foo"), sample.WithPackages(pkg3)),
		}
	)

	for _, v := range testVersions {
		if err := testDB.InsertVersion(ctx, v, sample.Licenses); err != nil {
			t.Fatalf("db.InsertVersion(%v): %v", v, err)
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
			otherVersion := sampler.Sample(sample.WithVersion("v1.0.5"), sample.WithPackages(tc.pkg))
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
