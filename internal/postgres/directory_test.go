// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/licenses"
	"golang.org/x/discovery/internal/source"
	"golang.org/x/discovery/internal/stdlib"
	"golang.org/x/discovery/internal/testing/sample"
)

func TestGetDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	InsertSampleDirectoryTree(ctx, t, testDB)

	for _, tc := range []struct {
		name, dirPath, modulePath, version, wantModulePath, wantVersion string
		wantPkgPaths                                                    []string
		wantNotFoundErr                                                 bool
	}{
		{
			name:           "latest with ambigious module path, should match longest module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        internal.LatestVersion,
			wantVersion:    "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
			},
		},
		{
			name:           "specified version with ambigious module path, should match longest module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.1.2",
			wantVersion:    "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
			},
		},
		{
			name:           "specified version with ambigous module path, but only shorter module path matches for specified version",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
			},
		},
		{
			name:           "specified version with ambiguous module path, two module versions exist, but only shorter module path contains matching package",
			dirPath:        "github.com/hashicorp/vault/builtin/audit",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.1.2",
			wantVersion:    "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
			},
		},
		{
			name:           "specified module path and version, should match specified shorter module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
			},
		},
		{
			name:           "directory path is the module path at latest",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        internal.LatestVersion,
			wantVersion:    "v1.2.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/internal/foo",
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
				"github.com/hashicorp/vault/vault/replication",
				"github.com/hashicorp/vault/vault/seal/transit",
			},
		},
		{
			name:           "directory path is the module path with specified version",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
			},
		},
		{
			name:           "directory path is a package path",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
			},
		},
		{
			name:           "valid directory path with package at version, no module path",
			dirPath:        "github.com/hashicorp/vault/builtin",
			modulePath:     internal.UnknownModulePath,
			wantModulePath: "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
			},
		},
		{
			name:           "valid directory path with package, specified version and module path",
			dirPath:        "github.com/hashicorp/vault/builtin",
			modulePath:     "github.com/hashicorp/vault",
			wantModulePath: "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/builtin/audit/file",
				"github.com/hashicorp/vault/builtin/audit/socket",
			},
		},
		{
			name:           "latest version of github.com/hashicorp/vault/api in github.com/hashicorp/vault",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault",
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.2",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/api",
			},
		},
		{
			name:           "latest version of github.com/hashicorp/vault/api in github.com/hashicorp/vault/api",
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
			name:           "latest version of internal directory in github.com/hashicorp/vault",
			dirPath:        "github.com/hashicorp/vault/internal",
			modulePath:     internal.UnknownModulePath,
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.2.3",
			wantPkgPaths: []string{
				"github.com/hashicorp/vault/internal/foo",
			},
		},
		{
			name:            "invalid directory, incomplete last element",
			dirPath:         "github.com/hashicorp/vault/builti",
			modulePath:      internal.UnknownModulePath,
			version:         "v1.0.3",
			wantNotFoundErr: true,
		},
		{
			name:           "stdlib directory",
			dirPath:        "archive",
			modulePath:     stdlib.ModulePath,
			version:        internal.LatestVersion,
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantPkgPaths: []string{
				"archive/zip",
				"archive/tar",
			},
		},
		{
			name:           "stdlib package",
			dirPath:        "archive/zip",
			modulePath:     stdlib.ModulePath,
			version:        internal.LatestVersion,
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantPkgPaths: []string{
				"archive/zip",
			},
		},
		{
			name:            "stdlib package -  incomplete last element",
			dirPath:         "archive/zi",
			modulePath:      stdlib.ModulePath,
			version:         internal.LatestVersion,
			wantNotFoundErr: true,
		},
		{
			name:           "stdlib - internal directory",
			dirPath:        "cmd/internal",
			modulePath:     stdlib.ModulePath,
			version:        internal.LatestVersion,
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantPkgPaths: []string{
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
		{
			name:           "stdlib - directory nested within an internal directory",
			dirPath:        "cmd/internal/obj",
			modulePath:     stdlib.ModulePath,
			version:        internal.LatestVersion,
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantPkgPaths: []string{
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testDB.GetDirectory(ctx, tc.dirPath, tc.modulePath, tc.version, internal.AllFields)
			if tc.wantNotFoundErr {
				if !errors.Is(err, derrors.NotFound) {
					t.Fatalf("want %v; got = \n%+v, %v", derrors.NotFound, got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			mi := sample.ModuleInfo()
			mi.ModulePath = tc.wantModulePath
			mi.Version = tc.wantVersion

			var wantPackages []*internal.Package
			for _, path := range tc.wantPkgPaths {
				pkg := sample.Package()
				pkg.Path = path
				pkg.Imports = nil
				wantPackages = append(wantPackages, pkg)
			}
			sort.Slice(wantPackages, func(i, j int) bool {
				return wantPackages[i].Path < wantPackages[j].Path
			})

			wantDirectory := &internal.Directory{
				ModuleInfo: *mi,
				Packages:   wantPackages,
				Path:       tc.dirPath,
			}
			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmp.AllowUnexported(source.Info{}),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			}
			if diff := cmp.Diff(wantDirectory, got, opts...); diff != "" {
				t.Errorf("testDB.GetDirectory(ctx, %q, %q, %q) mismatch (-want +got):\n%s", tc.dirPath, tc.modulePath, tc.version, diff)
			}
		})
	}
}

func TestGetDirectoryFieldSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	p := sample.Package()
	p.Path = "m.c/d/p"
	p.Imports = nil
	v := sample.Version()
	v.ModulePath = "m.c"
	v.Packages = []*internal.Package{p}
	if err := testDB.InsertVersion(ctx, v); err != nil {
		t.Fatal(err)
	}

	got, err := testDB.GetDirectory(ctx, "m.c/d", "m.c", sample.VersionString, internal.MinimalFields)
	if err != nil {
		t.Fatal(err)
	}
	if g, w := got.ReadmeContents, internal.StringFieldMissing; g != w {
		t.Errorf("ReadmeContents = %q, want %q", g, w)
	}
	if g, w := got.Packages[0].DocumentationHTML, internal.StringFieldMissing; g != w {
		t.Errorf("DocumentationHTML = %q, want %q", g, w)
	}
}
