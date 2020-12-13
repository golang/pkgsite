// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetUnitMeta(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	for _, testModule := range []struct {
		module, version, packageSuffix string
		isMaster                       bool
	}{
		{"m.com", "v1.0.0", "a", false},
		{"m.com", "v1.0.1", "dir/a", false},
		{"m.com", "v1.1.0", "a/b", false},
		{"m.com", "v1.2.0-pre", "a", true},
		{"m.com", "v2.0.0+incompatible", "a", false},
		{"m.com/a", "v1.1.0", "b", false},
		{"m.com/b", "v2.0.0+incompatible", "a", true},
	} {
		m := sample.Module(testModule.module, testModule.version, testModule.packageSuffix)
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
		requested := m.Version
		if testModule.isMaster {
			requested = "master"
		}
		if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       m.ModulePath,
			RequestedVersion: requested,
			ResolvedVersion:  m.Version,
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name                  string
		path, module, version string
		want                  *internal.UnitMeta
	}{
		{
			name:    "known module and version",
			path:    "m.com/a",
			module:  "m.com",
			version: "v1.2.0-pre",
			want: &internal.UnitMeta{
				ModulePath:        "m.com",
				Version:           "v1.2.0-pre",
				Name:              "a",
				IsRedistributable: true,
			},
		},
		{
			name:    "unknown module, known version",
			path:    "m.com/a/b",
			version: "v1.1.0",
			// The path is in two modules at v1.1.0. Prefer the longer one.
			want: &internal.UnitMeta{
				ModulePath:        "m.com/a",
				Version:           "v1.1.0",
				Name:              "b",
				IsRedistributable: true,
			},
		},
		{
			name:   "known module, unknown version",
			path:   "m.com/a",
			module: "m.com",
			// Choose the latest release version.
			want: &internal.UnitMeta{
				ModulePath:        "m.com",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			name: "unknown module and version",
			path: "m.com/a/b",
			// Select the latest release version, longest module.
			want: &internal.UnitMeta{
				ModulePath:        "m.com/a",
				Version:           "v1.1.0",
				Name:              "b",
				IsRedistributable: true,
			},
		},
		{
			name: "module",
			path: "m.com",
			// Select the latest version of the module.
			want: &internal.UnitMeta{
				ModulePath:        "m.com",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			name:    "longest module",
			path:    "m.com/a",
			version: "v1.1.0",
			// Prefer module m/a over module m, directory a.
			want: &internal.UnitMeta{
				ModulePath:        "m.com/a",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			name: "directory",
			path: "m.com/dir",
			want: &internal.UnitMeta{
				ModulePath:        "m.com",
				Version:           "v1.0.1",
				IsRedistributable: true,
			},
		},
		{
			name:    "module at master version",
			path:    "m.com",
			version: "master",
			want: &internal.UnitMeta{
				ModulePath:        "m.com",
				Version:           "v1.2.0-pre",
				IsRedistributable: true,
			},
		},
		{
			name:    "package at master version",
			path:    "m.com/a",
			version: "master",
			want: &internal.UnitMeta{
				ModulePath:        "m.com",
				Version:           "v1.2.0-pre",
				Name:              "a",
				IsRedistributable: true,
			},
		},
		{
			name:    "incompatible module",
			path:    "m.com/b",
			version: "master",
			want: &internal.UnitMeta{
				ModulePath:        "m.com/b",
				Version:           "v2.0.0+incompatible",
				IsRedistributable: true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.module == "" {
				test.module = internal.UnknownModulePath
			}
			if test.version == "" {
				test.version = internal.LatestVersion
			}
			test.want = sample.UnitMeta(
				test.path,
				test.want.ModulePath,
				test.want.Version,
				test.want.Name,
				test.want.IsRedistributable,
			)
			test.want.CommitTime = sample.CommitTime
			got, err := testDB.GetUnitMeta(ctx, test.path, test.module, test.version)
			if err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
				cmpopts.IgnoreFields(internal.UnitMeta{}, "HasGoMod"),
				cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
			}
			if diff := cmp.Diff(test.want, got, opts...); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetUnitMetaBypass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)
	bypassDB := NewBypassingLicenseCheck(testDB.db)

	for _, testModule := range []struct {
		module, version, packageSuffix string
		isMaster                       bool
	}{
		{"m.com", "v1.0.0", "a", false},
		{"m.com", "v1.0.1", "dir/a", false},
		{"m.com", "v1.1.0", "a/b", false},
		{"m.com", "v1.2.0-pre", "a", true},
		{"m.com", "v2.0.0+incompatible", "a", false},
		{"m.com/a", "v1.1.0", "b", false},
		{"m.com/b", "v2.0.0+incompatible", "a", true},
	} {
		m := sample.Module(testModule.module, testModule.version, testModule.packageSuffix)
		makeModuleNonRedistributable(m)

		if err := bypassDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
		requested := m.Version
		if testModule.isMaster {
			requested = "master"
		}
		if err := bypassDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       m.ModulePath,
			RequestedVersion: requested,
			ResolvedVersion:  m.Version,
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, bypassLicenseCheck := range []bool{false, true} {
		for _, test := range []struct {
			name                  string
			path, module, version string
			want                  *internal.UnitMeta
		}{
			{
				name:    "known module and version",
				path:    "m.com/a",
				module:  "m.com",
				version: "v1.2.0-pre",
				want: &internal.UnitMeta{
					ModulePath:        "m.com",
					Version:           "v1.2.0-pre",
					Name:              "a",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name:    "unknown module, known version",
				path:    "m.com/a/b",
				version: "v1.1.0",
				// The path is in two modules at v1.1.0. Prefer the longer one.
				want: &internal.UnitMeta{
					ModulePath:        "m.com/a",
					Version:           "v1.1.0",
					Name:              "b",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name:   "known module, unknown version",
				path:   "m.com/a",
				module: "m.com",
				// Choose the latest release version.
				want: &internal.UnitMeta{
					ModulePath:        "m.com",
					Version:           "v1.1.0",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name: "unknown module and version",
				path: "m.com/a/b",
				// Select the latest release version, longest module.
				want: &internal.UnitMeta{
					ModulePath:        "m.com/a",
					Version:           "v1.1.0",
					Name:              "b",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name: "module",
				path: "m.com",
				// Select the latest version of the module.
				want: &internal.UnitMeta{
					ModulePath:        "m.com",
					Version:           "v1.1.0",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name:    "longest module",
				path:    "m.com/a",
				version: "v1.1.0",
				// Prefer module m/a over module m, directory a.
				want: &internal.UnitMeta{
					ModulePath:        "m.com/a",
					Version:           "v1.1.0",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name: "directory",
				path: "m.com/dir",
				want: &internal.UnitMeta{
					ModulePath:        "m.com",
					Version:           "v1.0.1",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name:    "module at master version",
				path:    "m.com",
				version: "master",
				want: &internal.UnitMeta{
					ModulePath:        "m.com",
					Version:           "v1.2.0-pre",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name:    "package at master version",
				path:    "m.com/a",
				version: "master",
				want: &internal.UnitMeta{
					ModulePath:        "m.com",
					Version:           "v1.2.0-pre",
					Name:              "a",
					IsRedistributable: bypassLicenseCheck,
				},
			},
			{
				name:    "incompatible module",
				path:    "m.com/b",
				version: "master",
				want: &internal.UnitMeta{
					ModulePath:        "m.com/b",
					Version:           "v2.0.0+incompatible",
					IsRedistributable: bypassLicenseCheck,
				},
			},
		} {
			name := fmt.Sprintf("bypass %v %s", bypassLicenseCheck, test.name)
			t.Run(name, func(t *testing.T) {
				if test.module == "" {
					test.module = internal.UnknownModulePath
				}
				if test.version == "" {
					test.version = internal.LatestVersion
				}
				test.want = sample.UnitMeta(
					test.path,
					test.want.ModulePath,
					test.want.Version,
					test.want.Name,
					test.want.IsRedistributable,
				)
				test.want.CommitTime = sample.CommitTime

				var db *DB

				if bypassLicenseCheck {
					db = bypassDB
				} else {
					db = testDB
				}

				got, err := db.GetUnitMeta(ctx, test.path, test.module, test.version)
				if err != nil {
					t.Fatal(err)
				}
				opts := []cmp.Option{
					cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
					cmpopts.IgnoreFields(internal.UnitMeta{}, "HasGoMod"),
					cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
				}
				if diff := cmp.Diff(test.want, got, opts...); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			})
		}
	}
}
