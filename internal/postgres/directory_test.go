// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/xerrors"
)

func TestGetDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	mustInsertVersion := func(modulePath, version string, packages []*internal.Package) {
		v := sample.Version()
		v.ModulePath = modulePath
		v.Version = version
		for _, p := range packages {
			p.Licenses = sample.LicenseMetadata
			v.Packages = append(v.Packages, p)
		}
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	createVersionedPackages := func(modulePath, version string, packages []*internal.Package) []*internal.VersionedPackage {
		vi := sample.VersionInfo()
		vi.ModulePath = modulePath
		vi.Version = version

		var vps []*internal.VersionedPackage
		for _, pkg := range packages {
			pkg.Licenses = sample.LicenseMetadata
			vps = append(vps, &internal.VersionedPackage{VersionInfo: *vi, Package: *pkg})
		}
		return vps
	}
	createPackage := func(name, path string) *internal.Package {
		p := sample.Package()
		p.Name = name
		p.Path = path
		p.Imports = nil
		return p
	}

	apiPackage := createPackage("api", "github.com/hashicorp/vault/api")
	auditPackages := []*internal.Package{
		createPackage("file", "github.com/hashicorp/vault/builtin/audit/file"),
		createPackage("socket", "github.com/hashicorp/vault/builtin/audit/socket"),
	}
	v112Packages := append(
		auditPackages,
		createPackage("replication", "github.com/hashicorp/vault/vault/replication"),
		createPackage("transit", "github.com/hashicorp/vault/vault/seal/transit"),
		apiPackage)

	mustInsertVersion("github.com/hashicorp/vault", "v1.0.3", append(auditPackages, apiPackage))
	mustInsertVersion("github.com/hashicorp/vault/api", "v1.0.3", []*internal.Package{apiPackage})
	mustInsertVersion("github.com/hashicorp/vault", "v1.1.2", v112Packages)

	moduleVaultPackagesV103 := createVersionedPackages("github.com/hashicorp/vault", "v1.0.3", append(auditPackages, apiPackage))
	moduleVaultPackagesV112 := createVersionedPackages("github.com/hashicorp/vault", "v1.1.2", v112Packages)
	moduleVaultAuditPackages := createVersionedPackages("github.com/hashicorp/vault", "v1.0.3", auditPackages)

	for _, tc := range []struct {
		name, path, version, wantModulePath, wantVersion string
		wantPackages                                     []*internal.VersionedPackage
		wantNotFoundErr                                  bool
	}{
		{
			name:           "get latest version",
			path:           "github.com/hashicorp/vault",
			wantVersion:    "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault",
			wantPackages:   moduleVaultPackagesV112,
		},
		{
			name:           "module containing packages with same import path in different modules",
			path:           "github.com/hashicorp/vault",
			wantModulePath: "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantPackages:   moduleVaultPackagesV103,
		},
		{
			name:            "valid directory not containing packages",
			path:            "github.com/hashicorp/vault/api",
			version:         "v1.0.3",
			wantVersion:     "v1.0.3",
			wantNotFoundErr: true,
		},
		{
			name:           "valid directory path with package",
			path:           "github.com/hashicorp/vault/builtin",
			wantModulePath: "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantPackages:   moduleVaultAuditPackages,
		},
		{
			name:            "invalid directory, incomplete last element",
			path:            "github.com/hashicorp/vault/api/builti",
			wantModulePath:  "github.com/hashicorp/vault",
			version:         "v1.0.3",
			wantVersion:     "v1.0.3",
			wantNotFoundErr: true,
		},
		{
			name:            "invalid directory, not a subpath of a module path",
			path:            "github.com/hashicorp/vault/api/builti",
			wantModulePath:  "github.com/hashicorp/vault",
			version:         "v1.0.3",
			wantVersion:     "v1.0.3",
			wantNotFoundErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testDB.GetDirectory(ctx, tc.path, tc.version)
			if tc.wantNotFoundErr {
				if !xerrors.Is(err, derrors.NotFound) {
					t.Fatalf("expected err; got = \n%+v, %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			v := sample.VersionInfo()
			v.ModulePath = tc.wantModulePath
			v.Version = tc.version
			sort.Slice(tc.wantPackages, func(i, j int) bool {
				return tc.wantPackages[i].Path < tc.wantPackages[j].Path
			})

			wantDirectory := &internal.Directory{
				Path:     tc.path,
				Version:  tc.wantVersion,
				Packages: tc.wantPackages,
			}
			if diff := cmp.Diff(wantDirectory, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("testDB.GetDirectoryAtVersion(ctx, %q, %q) mismatch (-want +got):\n%s", tc.path, tc.version, diff)
			}
		})
	}
}
