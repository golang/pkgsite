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
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/xerrors"
)

func TestGetPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	mustInsertVersion := func(pkgPath, modulePath, version string) {
		t.Helper()
		v := sample.Version()
		v.ModulePath = modulePath
		v.Version = version
		pkg := sample.Package()
		pkg.Path = pkgPath
		v.Packages = []*internal.Package{pkg}
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	checkPackage := func(got *internal.VersionedPackage, pkgPath, modulePath, version string) {
		t.Helper()
		want := sample.VersionedPackage()
		want.Imports = nil
		want.ModulePath = modulePath
		want.Path = pkgPath
		want.Version = version
		if diff := cmp.Diff(want, got, cmpopts.EquateEmpty(), cmp.AllowUnexported(source.Info{})); diff != "" {
			t.Errorf("testDB.GetPackage(ctx, %q, %q, %q) mismatch (-want +got):\n%s", pkgPath, modulePath, version, diff)
		}
	}

	testPath := "github.com/hashicorp/vault/api"
	for _, data := range []struct {
		modulePath, version string
	}{
		{
			"github.com/hashicorp/vault",
			"v1.1.2",
		},
		{
			"github.com/hashicorp/vault/api",
			"v1.0.3",
		},
		{
			"github.com/hashicorp/vault",
			"v1.0.3",
		},
		{
			"github.com/hashicorp/vault",
			"v1.1.0-alpha.1",
		},
		{
			"github.com/hashicorp/vault",
			"v1.0.0-20190311183353-d8887717615a",
		},
	} {
		mustInsertVersion(testPath, data.modulePath, data.version)
	}

	for _, tc := range []struct {
		name, modulePath, version, wantModulePath, wantVersion string
		wantNotFoundErr, wantInvalidArgumentErr                bool
	}{
		{
			name:           "want latest package to be most recent release version",
			modulePath:     internal.UnknownModulePath,
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.2",
		},
		{
			name:           "want package@version for ambigious module path to be longest module path",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "want package with prerelease version and module path",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.1.0-alpha.1",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.0-alpha.1",
		},
		{
			name:           "want package for pseudoversion, only one version for module path",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.1.0-alpha.1",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.0-alpha.1",
		},
		{
			name:            "module@version/suffix does not exist ",
			modulePath:      "github.com/hashicorp/vault/api",
			version:         "v1.1.2",
			wantNotFoundErr: true,
		},
		{
			name:                   "version cannot be empty",
			modulePath:             internal.UnknownModulePath,
			version:                "",
			wantInvalidArgumentErr: true,
		},
		{
			name:                   "module path cannot be empty",
			modulePath:             "",
			version:                internal.LatestVersion,
			wantInvalidArgumentErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testDB.GetPackage(ctx, testPath, tc.modulePath, tc.version)
			if tc.wantNotFoundErr {
				if !xerrors.Is(err, derrors.NotFound) {
					t.Fatalf("want derrors.NotFound; got = %v", err)
				}
				return
			}
			if tc.wantInvalidArgumentErr {
				if !xerrors.Is(err, derrors.InvalidArgument) {
					t.Fatalf("want derrors.InvalidArgument; got = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			checkPackage(got, testPath, tc.wantModulePath, tc.wantVersion)
		})
	}
}
