// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestLegacyGetPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	suffix := func(pkgPath, modulePath string) string {
		if modulePath == stdlib.ModulePath {
			return pkgPath
		}
		if pkgPath == modulePath {
			return ""
		}
		return pkgPath[len(modulePath)+1:]
	}

	insertModule := func(pkgPath, modulePath, version string) {
		t.Helper()
		m := sample.Module(modulePath, version, suffix(pkgPath, modulePath))
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	checkPackage := func(got *internal.LegacyVersionedPackage, pkgPath, modulePath, version string) {
		t.Helper()
		want := &internal.LegacyVersionedPackage{
			LegacyModuleInfo: *sample.LegacyModuleInfo(modulePath, version),
			LegacyPackage:    *sample.LegacyPackage(modulePath, suffix(pkgPath, modulePath)),
		}
		want.Imports = nil
		opts := cmp.Options{
			cmpopts.EquateEmpty(),
			cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
			// The packages table only includes partial license information; it omits the Coverage field.
			cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
		}
		if diff := cmp.Diff(want, got, opts...); diff != "" {
			t.Errorf("testDB.LegacyGetPackage(ctx, %q, %q, %q) mismatch (-want +got):\n%s", pkgPath, modulePath, version, diff)
		}
	}

	for _, data := range []struct {
		pkgPath, modulePath, version string
	}{
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault",
			"v1.1.2",
		},
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault/api",
			"v1.0.3",
		},
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault",
			"v1.0.3",
		},
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault",
			"v1.1.0-alpha.1",
		},
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault",
			"v1.0.0-20190311183353-d8887717615a",
		},
		{
			"github.com/hashicorp/vault/api",
			"github.com/hashicorp/vault",
			"v2.0.0+incompatible",
		},
		{
			"archive/zip",
			stdlib.ModulePath,
			"v1.13.1",
		},
		{
			"archive/zip",
			stdlib.ModulePath,
			"v1.13.0",
		},
	} {
		insertModule(data.pkgPath, data.modulePath, data.version)
	}

	for _, tc := range []struct {
		name, pkgPath, modulePath, version, wantPkgPath, wantModulePath, wantVersion string
		wantNotFoundErr                                                              bool
	}{
		{
			name:           "want latest package to be most recent release version",
			pkgPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        internal.LatestVersion,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.2",
		},
		{
			name:           "want package@version for ambigious module path to be longest module path",
			pkgPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.0.3",
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "want package with prerelease version and module path",
			pkgPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.1.0-alpha.1",
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.0-alpha.1",
		},
		{
			name:           "want package for pseudoversion, only one version for module path",
			pkgPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.1.0-alpha.1",
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.0-alpha.1",
		},
		{
			name:           "latest version of github.com/hashicorp/vault/api in github.com/hashicorp/vault/api",
			pkgPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault/api",
			version:        internal.LatestVersion,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "latest version of github.com/hashicorp/vault/api in github.com/hashicorp/vault",
			pkgPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault",
			version:        internal.LatestVersion,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.2",
		},
		{
			name:            "module@version/suffix does not exist ",
			pkgPath:         "github.com/hashicorp/vault/api",
			modulePath:      "github.com/hashicorp/vault/api",
			wantPkgPath:     "github.com/hashicorp/vault/api",
			version:         "v1.1.2",
			wantNotFoundErr: true,
		},
		{
			name:           "latest version of archive/zip",
			pkgPath:        "archive/zip",
			modulePath:     stdlib.ModulePath,
			version:        internal.LatestVersion,
			wantPkgPath:    "archive/zip",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.1",
		},
		{
			name:           "specific version of archive/zip",
			pkgPath:        "archive/zip",
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.0",
			wantPkgPath:    "archive/zip",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testDB.LegacyGetPackage(ctx, tc.pkgPath, tc.modulePath, tc.version)
			if tc.wantNotFoundErr {
				if !errors.Is(err, derrors.NotFound) {
					t.Fatalf("want derrors.NotFound; got = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			checkPackage(got, tc.wantPkgPath, tc.wantModulePath, tc.wantVersion)
		})
	}
}

func TestLegacyGetPackageInvalidArguments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	for _, tc := range []struct {
		name, modulePath, version string
		wantInvalidArgumentErr    bool
	}{
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
			got, err := testDB.LegacyGetPackage(ctx, tc.modulePath+"/package", tc.modulePath, tc.version)
			if !errors.Is(err, derrors.InvalidArgument) {
				t.Fatalf("want %v; got = \n%+v, %v", derrors.InvalidArgument, got, err)
			}
		})
	}
}
