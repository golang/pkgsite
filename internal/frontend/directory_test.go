// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/testing/sample"
	"golang.org/x/xerrors"
)

func TestFetchDirectoryDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	postgres.InsertSampleDirectoryTree(ctx, t, testDB)

	checkDirectory := func(got *Directory, dirPath, modulePath, version string, pkgPaths []string) {
		t.Helper()

		vi := sample.VersionInfo()
		vi.ModulePath = modulePath
		vi.Version = version

		var wantPkgs []*Package
		for _, path := range pkgPaths {
			sp := sample.Package()
			sp.Path = path
			pkg, err := createPackage(sp, vi, false)
			if err != nil {
				t.Fatal(err)
			}
			pkg.Suffix = strings.TrimPrefix(strings.TrimPrefix(sp.Path, dirPath), "/")
			if pkg.Suffix == "" {
				pkg.Suffix = fmt.Sprintf("%s (root)", effectiveName(sp))
			}
			wantPkgs = append(wantPkgs, pkg)
		}

		mod, err := createModule(vi, sample.LicenseMetadata, false)
		if err != nil {
			t.Fatal(err)
		}

		formattedVersion := vi.Version
		if vi.ModulePath == stdlib.ModulePath {
			formattedVersion, err = stdlib.TagForVersion(vi.Version)
			if err != nil {
				t.Fatal(err)
			}
		}
		want := &Directory{
			Module:   *mod,
			Path:     dirPath,
			Packages: wantPkgs,
			URL:      constructDirectoryURL(dirPath, vi.ModulePath, formattedVersion),
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("fetchDirectoryDetails(ctx, %q, %q, %q) mismatch (-want +got):\n%s", dirPath, modulePath, version, diff)
		}
	}

	for _, tc := range []struct {
		name, dirPath, modulePath, version, wantModulePath, wantVersion string
		wantPkgPaths                                                    []string
		includeDirPath, wantInvalidArgumentErr                          bool
	}{
		{
			name:           "dirPath is modulePath, includeDirPath = true, want longest module path",
			includeDirPath: true,
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault/api",
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault/api",
			wantVersion:    "v1.1.2",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
			},
		},
		{
			name:           "only dirPath provided, includeDirPath = false, want longest module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault/api",
			wantVersion:    "v1.1.2",
			wantPkgPaths:   []string{},
		},
		{
			name:           "dirPath@version, includeDirPath = false, want longest module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantVersion:    "v1.1.2",
			wantPkgPaths:   []string{},
		},
		{
			name:           "dirPath@version,  includeDirPath = false, version only exists for shorter module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.0.3",
			wantPkgPaths:   []string{},
		},
		{
			name:           "valid directory for modulePath@version/suffix, includeDirPath = false",
			dirPath:        "github.com/hashicorp/vault/builtin",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.0.3",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
			},
		},
		{
			name:           "standard library",
			dirPath:        stdlib.ModulePath,
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.4",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantPkgPaths: []string{
				"archive/tar",
				"archive/zip",
				"cmd/go",
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
		{
			name:           "cmd",
			dirPath:        "cmd",
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.4",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantPkgPaths: []string{
				"cmd/go",
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vi := sample.VersionInfo()
			vi.ModulePath = tc.modulePath
			vi.Version = tc.version

			got, err := fetchDirectoryDetails(ctx, testDB,
				tc.dirPath, vi, sample.LicenseMetadata, tc.includeDirPath)
			if err != nil {
				t.Fatal(err)
			}
			checkDirectory(got, tc.dirPath, tc.wantModulePath, tc.wantVersion, tc.wantPkgPaths)
		})
	}
}

func TestFetchDirectoryDetailsInvalidArguments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)
	postgres.InsertSampleDirectoryTree(ctx, t, testDB)

	for _, tc := range []struct {
		name, dirPath, modulePath, version, wantModulePath, wantVersion string
		includeDirPath                                                  bool
		wantPkgPaths                                                    []string
	}{
		{
			name:       "dirPath is empty",
			dirPath:    "github.com/hashicorp/vault/api",
			modulePath: "",
			version:    internal.LatestVersion,
		},
		{
			name:       "modulePath is empty",
			dirPath:    "github.com/hashicorp/vault/api",
			modulePath: "",
			version:    internal.LatestVersion,
		},
		{
			name:       "version is empty",
			dirPath:    "github.com/hashicorp/vault/api",
			modulePath: internal.UnknownModulePath,
			version:    "",
		},
		{
			name:           "dirPath is not modulePath, includeDirPath = true",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault",
			version:        internal.LatestVersion,
			includeDirPath: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vi := sample.VersionInfo()
			vi.ModulePath = tc.modulePath
			vi.Version = tc.version

			got, err := fetchDirectoryDetails(ctx, testDB,
				tc.dirPath, vi, sample.LicenseMetadata, tc.includeDirPath)
			if !xerrors.Is(err, derrors.InvalidArgument) {
				t.Fatalf("expected err; got = \n%+v, %v", got, err)
			}
		})
	}
}
