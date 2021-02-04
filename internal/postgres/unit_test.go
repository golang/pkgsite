// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
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
		{"cloud.google.com/go", "v0.69.0", "pubsublite", false},
		{"cloud.google.com/go/pubsublite", "v0.4.0", "", false},
		{"cloud.google.com/go", "v0.74.0", "compute/metadata", false},
		{"cloud.google.com/go/compute/metadata", "v0.0.0-20181115181204-d50f0e9b2506", "", false},
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

	type teststruct struct {
		name                  string
		path, module, version string
		want                  *internal.UnitMeta
	}

	checkUnitMeta := func(ctx context.Context, test teststruct) {
		got, err := testDB.GetUnitMeta(ctx, test.path, test.module, test.version)
		if err != nil {
			t.Fatal(err)
		}
		opts := []cmp.Option{
			cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage", "OldCoverage"),
			cmpopts.IgnoreFields(internal.UnitMeta{}, "HasGoMod"),
			cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
		}
		if diff := cmp.Diff(test.want, got, opts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	}

	for _, test := range []teststruct{
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
		{
			name: "prefer pubsublite nested module",
			path: "cloud.google.com/go/pubsublite",
			want: &internal.UnitMeta{
				ModulePath:        "cloud.google.com/go/pubsublite",
				Name:              "pubsublite",
				Version:           "v0.4.0",
				IsRedistributable: true,
			},
		},
		{
			name: "prefer compute metadata in main module",
			path: "cloud.google.com/go/compute/metadata",
			want: &internal.UnitMeta{
				ModulePath:        "cloud.google.com/go",
				Name:              "metadata",
				Version:           "v0.74.0",
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
			checkUnitMeta(ctx, test)
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
					cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage", "OldCoverage"),
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

func TestGetUnit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)
	InsertSampleDirectoryTree(ctx, t, testDB)

	// Add a module that has READMEs in a directory and a package.
	m := sample.Module("a.com/m", "v1.2.3", "dir/p")
	d := findDirectory(m, "a.com/m/dir")
	d.Readme = &internal.Readme{
		Filepath: "DIR_README.md",
		Contents: "dir readme",
	}
	d = findDirectory(m, "a.com/m/dir/p")
	d.Readme = &internal.Readme{
		Filepath: "PKG_README.md",
		Contents: "pkg readme",
	}
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	// Add a module that has documentation for two Go build contexts.
	m = sample.Module("a.com/twodoc", "v1.2.3", "p")
	pkg := m.Packages()[0]
	docs2 := []*internal.Documentation{
		{
			GOOS:     "linux",
			GOARCH:   "amd64",
			Synopsis: sample.Synopsis + " for linux",
			Source:   sample.Documentation.Source,
		},
		{
			GOOS:     "windows",
			GOARCH:   "amd64",
			Synopsis: sample.Synopsis + " for windows",
			Source:   sample.Documentation.Source,
		},
	}
	pkg.Documentation = docs2
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, path, modulePath, version string
		want                            *internal.Unit
	}{
		{
			name:       "module path",
			path:       "github.com/hashicorp/vault",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want: unit("github.com/hashicorp/vault", "github.com/hashicorp/vault", "v1.0.3", "",
				&internal.Readme{
					Filepath: sample.ReadmeFilePath,
					Contents: sample.ReadmeContents,
				},
				[]string{
					"api",
					"builtin/audit/file",
					"builtin/audit/socket",
				},
			),
		},
		{
			name:       "package path",
			path:       "github.com/hashicorp/vault/api",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want: unit("github.com/hashicorp/vault/api", "github.com/hashicorp/vault", "v1.0.3", "api", nil,
				[]string{
					"api",
				},
			),
		},
		{
			name:       "directory path",
			path:       "github.com/hashicorp/vault/builtin",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want: unit("github.com/hashicorp/vault/builtin", "github.com/hashicorp/vault", "v1.0.3", "", nil,
				[]string{
					"builtin/audit/file",
					"builtin/audit/socket",
				},
			),
		},
		{
			name:       "stdlib directory",
			path:       "archive",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want: unit("archive", stdlib.ModulePath, "v1.13.4", "", nil,
				[]string{
					"archive/tar",
					"archive/zip",
				},
			),
		},
		{
			name:       "stdlib package",
			path:       "archive/zip",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want: unit("archive/zip", stdlib.ModulePath, "v1.13.4", "zip", nil,
				[]string{
					"archive/zip",
				},
			),
		},
		{
			name:       "stdlib - internal directory",
			path:       "cmd/internal",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want: unit("cmd/internal", stdlib.ModulePath, "v1.13.4", "", nil,
				[]string{
					"cmd/internal/obj",
					"cmd/internal/obj/arm",
					"cmd/internal/obj/arm64",
				},
			),
		},
		{
			name:       "directory with readme",
			path:       "a.com/m/dir",
			modulePath: "a.com/m",
			version:    "v1.2.3",
			want: unit("a.com/m/dir", "a.com/m", "v1.2.3", "", &internal.Readme{
				Filepath: "DIR_README.md",
				Contents: "dir readme",
			},
				[]string{
					"dir/p",
				},
			),
		},
		{
			name:       "package with readme",
			path:       "a.com/m/dir/p",
			modulePath: "a.com/m",
			version:    "v1.2.3",
			want: unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "p",
				&internal.Readme{
					Filepath: "PKG_README.md",
					Contents: "pkg readme",
				},
				[]string{
					"dir/p",
				},
			),
		},
		{
			name:       "package with two docs",
			path:       "a.com/twodoc/p",
			modulePath: "a.com/twodoc",
			version:    "v1.2.3",
			want: func() *internal.Unit {
				u := unit("a.com/twodoc/p", "a.com/twodoc", "v1.2.3", "p",
					nil,
					[]string{"p"})
				u.Documentation = docs2
				u.Subdirectories[0].Synopsis = docs2[0].Synopsis
				return u
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			um := sample.UnitMeta(
				test.path,
				test.modulePath,
				test.version,
				test.want.Name,
				test.want.IsRedistributable,
			)
			test.want.CommitTime = um.CommitTime
			checkUnit(ctx, t, um, test.want)
		})
	}
}

func checkUnit(ctx context.Context, t *testing.T, um *internal.UnitMeta, want *internal.Unit, experiments ...string) {
	t.Helper()
	ctx = experiment.NewContext(ctx, experiments...)
	got, err := testDB.GetUnit(ctx, um, internal.AllFields)
	if err != nil {
		t.Fatal(err)
	}
	opts := []cmp.Option{
		cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
		// The packages table only includes partial license information; it omits the Coverage field.
		cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage", "OldCoverage"),
	}
	want.SourceInfo = um.SourceInfo
	want.NumImports = len(want.Imports)
	opts = append(opts,
		cmpopts.IgnoreFields(internal.Unit{}, "Imports", "LicenseContents"),
	)
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestGetUnit_SubdirectoriesShowNonRedistPackages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	m := sample.DefaultModule()
	m.IsRedistributable = false
	m.Packages()[0].IsRedistributable = false
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}
}

func TestGetUnitFieldSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	readme := &internal.Readme{
		Filepath: "a.com/m/dir/p/README.md",
		Contents: "readme",
	}
	// Add a module that has READMEs in a directory and a package.
	m := sample.Module("a.com/m", "v1.2.3", "dir/p")
	m.Packages()[0].Readme = readme
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	cleanFields := func(u *internal.Unit, fields internal.FieldSet) {
		// Add/remove fields based on the FieldSet specified.
		if fields&internal.WithMain != 0 {
			u.Documentation = []*internal.Documentation{sample.Documentation}
			u.Readme = readme
			u.NumImports = len(sample.Imports)
			u.Subdirectories = []*internal.PackageMeta{
				{
					Path:              "a.com/m/dir/p",
					Name:              "p",
					Synopsis:          sample.Synopsis,
					IsRedistributable: true,
					Licenses:          sample.LicenseMetadata,
				},
			}
		}
		if fields&internal.WithImports != 0 {
			u.Imports = sample.Imports
			u.NumImports = len(sample.Imports)
		}
		if fields&internal.WithLicenses == 0 {
			u.LicenseContents = nil
		}
	}

	for _, test := range []struct {
		name   string
		fields internal.FieldSet
		want   *internal.Unit
	}{
		{
			name:   "WithMain",
			fields: internal.WithMain,
			want:   unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "", readme, []string{}),
		},
		{
			name:   "WithImports",
			fields: internal.WithImports,
			want:   unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "", nil, []string{}),
		},
		{
			name:   "WithLicenses",
			fields: internal.WithLicenses,
			want:   unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "", nil, []string{}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			um := sample.UnitMeta(
				test.want.Path,
				test.want.ModulePath,
				test.want.Version,
				test.want.Name,
				test.want.IsRedistributable,
			)
			got, err := testDB.GetUnit(ctx, um, test.fields)
			if err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage", "OldCoverage"),
			}
			test.want.CommitTime = um.CommitTime
			test.want.SourceInfo = um.SourceInfo
			cleanFields(test.want, test.fields)
			if diff := cmp.Diff(test.want, got, opts...); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func unit(fullPath, modulePath, version, name string, readme *internal.Readme, suffixes []string) *internal.Unit {
	u := &internal.Unit{
		UnitMeta: internal.UnitMeta{
			ModulePath:        modulePath,
			Version:           version,
			Path:              fullPath,
			IsRedistributable: true,
			Licenses:          sample.LicenseMetadata,
			Name:              name,
		},
		LicenseContents: sample.Licenses,
		Readme:          readme,
	}

	u.Subdirectories = subdirectories(modulePath, suffixes)
	if u.IsPackage() {
		u.Imports = sample.Imports
		u.NumImports = len(sample.Imports)
		u.Documentation = []*internal.Documentation{sample.Documentation}
	}
	return u
}

func subdirectories(modulePath string, suffixes []string) []*internal.PackageMeta {
	var want []*internal.PackageMeta
	for _, suffix := range suffixes {
		p := suffix
		if modulePath != stdlib.ModulePath {
			p = path.Join(modulePath, suffix)
		}
		want = append(want, sample.PackageMeta(p))
	}
	return want
}

func TestGetUnitBypass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)
	bypassDB := NewBypassingLicenseCheck(testDB.db)

	m := nonRedistributableModule()
	if err := bypassDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		db        *DB
		wantEmpty bool
	}{
		{testDB, true},
		{bypassDB, false},
	} {
		pathInfo := &internal.UnitMeta{
			Path:       m.ModulePath,
			ModulePath: m.ModulePath,
			Version:    m.Version,
		}
		d, err := test.db.GetUnit(ctx, pathInfo, internal.AllFields)
		if err != nil {
			t.Fatal(err)
		}
		if got := (d.Readme == nil); got != test.wantEmpty {
			t.Errorf("readme empty: got %t, want %t", got, test.wantEmpty)
		}
		if got := (d.Documentation == nil); got != test.wantEmpty {
			t.Errorf("synopsis empty: got %t, want %t", got, test.wantEmpty)
		}
		if got := (d.Documentation == nil); got != test.wantEmpty {
			t.Errorf("doc empty: got %t, want %t", got, test.wantEmpty)
		}
		pkgs := d.Subdirectories
		if len(pkgs) != 1 {
			t.Fatal("len(pkgs) != 1")
		}
		if got := (pkgs[0].Synopsis == ""); got != test.wantEmpty {
			t.Errorf("synopsis empty: got %t, want %t", got, test.wantEmpty)
		}
	}
}

func findDirectory(m *internal.Module, path string) *internal.Unit {
	for _, d := range m.Units {
		if d.Path == path {
			return d
		}
	}
	return nil
}
