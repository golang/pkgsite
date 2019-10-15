// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/version"
)

func TestGetPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	sampleVersion := func(version string, vtype version.Type) *internal.Version {
		v := sample.Version()
		v.Version = version
		v.VersionType = vtype
		return v
	}

	testCases := []struct {
		name, path, version string
		versions            []*internal.Version
		wantPkg             *internal.VersionedPackage
		wantReadErr         bool
	}{
		{
			name: "want latest package to be most recent release version",
			path: sample.PackagePath,
			versions: []*internal.Version{
				sampleVersion("v1.1.0-alpha.1", version.TypePrerelease),
				sampleVersion("v1.0.0", version.TypeRelease),
				sampleVersion("v1.0.0-20190311183353-d8887717615a", version.TypePseudo),
			},
			version: internal.LatestVersion,
			wantPkg: func() *internal.VersionedPackage {
				p := sample.VersionedPackage()
				p.Version = "v1.0.0"
				p.VersionType = version.TypeRelease
				// TODO(b/130367504): GetPackage does not return imports.
				p.Imports = nil
				return p
			}(),
		},
		{
			name: "want package for version",
			path: sample.PackagePath,
			versions: []*internal.Version{
				sampleVersion("v1.0.0", version.TypeRelease),
				sampleVersion("v1.1.0", version.TypeRelease),
			},
			version: "v1.1.0",
			wantPkg: func() *internal.VersionedPackage {
				p := sample.VersionedPackage()
				p.Version = "v1.1.0"
				p.VersionType = version.TypeRelease
				// TODO(b/130367504): GetPackage does not return imports.
				p.Imports = nil
				return p
			}(),
		},
		{
			name:        "empty path",
			path:        "",
			wantReadErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range tc.versions {
				if err := testDB.saveVersion(ctx, v); err != nil {
					t.Error(err)
				}
			}

			gotPkg, err := testDB.GetPackage(ctx, tc.path, tc.version)
			if (err != nil) != tc.wantReadErr {
				t.Fatal(err)
			}

			if diff := cmp.Diff(tc.wantPkg, gotPkg, cmpopts.EquateEmpty(), cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", tc.path, internal.LatestVersion, diff)
			}
		})
	}
}

func TestGetPackageInModuleVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	mustInsertVersion := func(modulePath string) {
		v := sample.Version()
		v.ModulePath = modulePath
		pkg := sample.Package()
		pkg.Path = "github.com/hashicorp/vault/api"
		v.Packages = []*internal.Package{pkg}
		v.Version = "v1.0.3"
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	checkPackage := func(pkgPath, modulePath, version string) {
		got, err := testDB.GetPackageInModuleVersion(ctx, pkgPath, modulePath, version)
		if err != nil {
			t.Fatal(err)
		}
		want := sample.VersionedPackage()
		want.Imports = nil
		want.ModulePath = modulePath
		want.Path = pkgPath
		want.Version = version
		if diff := cmp.Diff(want, got, cmpopts.EquateEmpty(), cmp.AllowUnexported(source.Info{})); diff != "" {
			t.Errorf("testDB.GetPackageInModuleVersion(ctx, %q, %q, %q) mismatch (-want +got):\n%s", pkgPath, modulePath, version, diff)
		}
	}

	mustInsertVersion("github.com/hashicorp/vault")
	mustInsertVersion("github.com/hashicorp/vault/api")
	for _, tc := range []struct {
		pkgPath, modulePath, version string
	}{
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault",
			"v1.0.3",
		},
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault/api",
			"v1.0.3",
		},
	} {
		t.Run(tc.modulePath, func(t *testing.T) {
			checkPackage(tc.pkgPath, tc.modulePath, tc.version)
		})
	}
}
