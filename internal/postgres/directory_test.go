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

func TestGetPackagesInUnit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	InsertSampleDirectoryTree(ctx, t, testDB)

	for _, tc := range []struct {
		name, dirPath, modulePath, version, wantModulePath, wantVersion string
		wantSuffixes                                                    []string
		wantNotFoundErr                                                 bool
	}{
		{
			name:           "directory path is the module path",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.2.3",
			wantVersion:    "v1.2.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes: []string{
				"builtin/audit/file",
				"builtin/audit/socket",
				"internal/foo",
				"vault/replication",
				"vault/seal/transit",
			},
		},
		{
			name:           "directory path is a package path",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes: []string{
				"api",
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
		{
			name:           "directory path is not a package or module",
			dirPath:        "github.com/hashicorp/vault/builtin",
			modulePath:     "github.com/hashicorp/vault",
			wantModulePath: "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantSuffixes: []string{
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
		{
			name:           "stdlib module",
			dirPath:        stdlib.ModulePath,
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.4",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantSuffixes: []string{
				"archive/tar",
				"archive/zip",
				"cmd/go",
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
		{
			name:           "stdlib directory",
			dirPath:        "archive",
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.4",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantSuffixes: []string{
				"archive/tar",
				"archive/zip",
			},
		},
		{
			name:           "stdlib package",
			dirPath:        "archive/zip",
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.4",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantSuffixes: []string{
				"archive/zip",
			},
		},
		{
			name:            "stdlib package -  incomplete last element",
			dirPath:         "archive/zi",
			modulePath:      stdlib.ModulePath,
			version:         "v1.13.4",
			wantNotFoundErr: true,
		},
		{
			name:           "stdlib - internal directory",
			dirPath:        "cmd/internal",
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.4",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantSuffixes: []string{
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
		{
			name:           "stdlib - directory nested within an internal directory",
			dirPath:        "cmd/internal/obj",
			modulePath:     stdlib.ModulePath,
			version:        "v1.13.4",
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantSuffixes: []string{
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testDB.GetPackagesInUnit(ctx, tc.dirPath, tc.modulePath, tc.version)
			if tc.wantNotFoundErr {
				if !errors.Is(err, derrors.NotFound) {
					t.Fatalf("got error %v; want %v", err, derrors.NotFound)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			var wantPackages []*internal.PackageMeta
			for _, suffix := range tc.wantSuffixes {
				pkg := sample.PackageMeta(tc.wantModulePath, suffix)
				wantPackages = append(wantPackages, pkg)
			}

			opts := []cmp.Option{
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			}
			if diff := cmp.Diff(wantPackages, got, opts...); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetPackagesInUnitBypass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)
	bypassDB := NewBypassingLicenseCheck(testDB.db)

	// Insert a non-redistributable module.
	m := nonRedistributableModule()
	if err := bypassDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		db   *DB
		want string
	}{
		{bypassDB, sample.Synopsis}, // Reading with license bypass returns the synopsis.
		{testDB, ""},                // Without bypass, the synopsis is empty.
	} {
		pkgs, err := test.db.GetPackagesInUnit(ctx, m.ModulePath, m.ModulePath, m.Version)
		if err != nil {
			t.Fatal(err)
		}
		if len(pkgs) != 1 {
			t.Fatal("len(pkgs) != 1")
		}
		if got := pkgs[0].Synopsis; got != test.want {
			t.Errorf("bypass %t: got %q, want %q", test.db == bypassDB, got, test.want)
		}
	}
}

func TestLegacyGetDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	InsertSampleDirectoryTree(ctx, t, testDB)

	for _, tc := range []struct {
		name, dirPath, modulePath, version, wantModulePath, wantVersion string
		wantSuffixes                                                    []string
		wantNotFoundErr                                                 bool
	}{
		{
			name:           "latest with ambigious module path, should match longest module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        internal.LatestVersion,
			wantVersion:    "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantSuffixes:   []string{""},
		},
		{
			name:           "specified version with ambigious module path, should match longest module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.1.2",
			wantVersion:    "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault/api",
			wantSuffixes:   []string{""},
		},
		{
			name:           "specified version with ambigous module path, but only shorter module path matches for specified version",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes:   []string{"api"},
		},
		{
			name:           "specified version with ambiguous module path, two module versions exist, but only shorter module path contains matching package",
			dirPath:        "github.com/hashicorp/vault/builtin/audit",
			modulePath:     internal.UnknownModulePath,
			version:        "v1.1.2",
			wantVersion:    "v1.1.2",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes: []string{
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
		{
			name:           "specified module path and version, should match specified shorter module path",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes:   []string{"api"},
		},
		{
			name:           "directory path is the module path at latest",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        internal.LatestVersion,
			wantVersion:    "v1.2.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes: []string{
				"builtin/audit/file",
				"builtin/audit/socket",
				"internal/foo",
				"vault/replication",
				"vault/seal/transit",
			},
		},
		{
			name:           "directory path is the module path with specified version",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes: []string{
				"api",
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
		{
			name:           "directory path is a package path",
			dirPath:        "github.com/hashicorp/vault",
			modulePath:     "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantModulePath: "github.com/hashicorp/vault",
			wantSuffixes: []string{
				"api",
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
		{
			name:           "valid directory path with package at version, no module path",
			dirPath:        "github.com/hashicorp/vault/builtin",
			modulePath:     internal.UnknownModulePath,
			wantModulePath: "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantSuffixes: []string{
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
		{
			name:           "valid directory path with package, specified version and module path",
			dirPath:        "github.com/hashicorp/vault/builtin",
			modulePath:     "github.com/hashicorp/vault",
			wantModulePath: "github.com/hashicorp/vault",
			version:        "v1.0.3",
			wantVersion:    "v1.0.3",
			wantSuffixes: []string{
				"builtin/audit/file",
				"builtin/audit/socket",
			},
		},
		{
			name:           "latest version of github.com/hashicorp/vault/api in github.com/hashicorp/vault",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault",
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.1.2",
			wantSuffixes: []string{
				"api",
			},
		},
		{
			name:           "latest version of github.com/hashicorp/vault/api in github.com/hashicorp/vault/api",
			dirPath:        "github.com/hashicorp/vault/api",
			modulePath:     "github.com/hashicorp/vault/api",
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault/api",
			wantVersion:    "v1.1.2",
			wantSuffixes:   []string{""},
		},
		{
			name:           "latest version of internal directory in github.com/hashicorp/vault",
			dirPath:        "github.com/hashicorp/vault/internal",
			modulePath:     internal.UnknownModulePath,
			version:        internal.LatestVersion,
			wantModulePath: "github.com/hashicorp/vault",
			wantVersion:    "v1.2.3",
			wantSuffixes:   []string{"internal/foo"},
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
			wantSuffixes: []string{
				"archive/tar",
				"archive/zip",
			},
		},
		{
			name:           "stdlib package",
			dirPath:        "archive/zip",
			modulePath:     stdlib.ModulePath,
			version:        internal.LatestVersion,
			wantModulePath: stdlib.ModulePath,
			wantVersion:    "v1.13.4",
			wantSuffixes: []string{
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
			wantSuffixes: []string{
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
			wantSuffixes: []string{
				"cmd/internal/obj",
				"cmd/internal/obj/arm",
				"cmd/internal/obj/arm64",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testDB.LegacyGetDirectory(ctx, tc.dirPath, tc.modulePath, tc.version, internal.AllFields)
			if tc.wantNotFoundErr {
				if !errors.Is(err, derrors.NotFound) {
					t.Fatalf("got error %v; want %v", err, derrors.NotFound)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			mi := sample.LegacyModuleInfo(tc.wantModulePath, tc.wantVersion)
			var wantPackages []*internal.LegacyPackage
			for _, suffix := range tc.wantSuffixes {
				pkg := sample.LegacyPackage(tc.wantModulePath, suffix)
				pkg.Imports = nil
				wantPackages = append(wantPackages, pkg)
			}

			wantDirectory := &internal.LegacyDirectory{
				LegacyModuleInfo: *mi,
				Packages:         wantPackages,
				Path:             tc.dirPath,
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
				cmpopts.IgnoreFields(internal.DirectoryMeta{}, "PathID"),
			}
			if diff := cmp.Diff(wantDirectory, got, opts...); diff != "" {
				t.Errorf("testDB.LegacyGetDirectory(ctx, %q, %q, %q) mismatch (-want +got):\n%s", tc.dirPath, tc.modulePath, tc.version, diff)
			}
		})
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

	for _, tc := range []struct {
		name, dirPath, modulePath, version string
		want                               *internal.Unit
		wantNotFoundErr                    bool
	}{
		{
			name:       "module path",
			dirPath:    "github.com/hashicorp/vault",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want: newVdir("github.com/hashicorp/vault", "github.com/hashicorp/vault", "v1.0.3",
				&internal.Readme{
					Filepath: sample.ReadmeFilePath,
					Contents: sample.ReadmeContents,
				}, nil),
		},
		{
			name:       "package path",
			dirPath:    "github.com/hashicorp/vault/api",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want: newVdir("github.com/hashicorp/vault/api", "github.com/hashicorp/vault", "v1.0.3", nil,
				newPackage("api", "github.com/hashicorp/vault/api")),
		},
		{
			name:       "directory path",
			dirPath:    "github.com/hashicorp/vault/builtin",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want:       newVdir("github.com/hashicorp/vault/builtin", "github.com/hashicorp/vault", "v1.0.3", nil, nil),
		},
		{
			name:       "stdlib directory",
			dirPath:    "archive",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want:       newVdir("archive", stdlib.ModulePath, "v1.13.4", nil, nil),
		},
		{
			name:       "stdlib package",
			dirPath:    "archive/zip",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want:       newVdir("archive/zip", stdlib.ModulePath, "v1.13.4", nil, newPackage("zip", "archive/zip")),
		},
		{
			name:            "stdlib package - incomplete last element",
			dirPath:         "archive/zi",
			modulePath:      stdlib.ModulePath,
			version:         "v1.13.4",
			wantNotFoundErr: true,
		},
		{
			name:       "stdlib - internal directory",
			dirPath:    "cmd/internal",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want:       newVdir("cmd/internal", stdlib.ModulePath, "v1.13.4", nil, nil),
		},
		{
			name:       "directory with readme",
			dirPath:    "a.com/m/dir",
			modulePath: "a.com/m",
			version:    "v1.2.3",
			want: newVdir("a.com/m/dir", "a.com/m", "v1.2.3", &internal.Readme{
				Filepath: "DIR_README.md",
				Contents: "dir readme",
			}, nil),
		},
		{
			name:       "package with readme",
			dirPath:    "a.com/m/dir/p",
			modulePath: "a.com/m",
			version:    "v1.2.3",
			want: newVdir("a.com/m/dir/p", "a.com/m", "v1.2.3",
				&internal.Readme{
					Filepath: "PKG_README.md",
					Contents: "pkg readme",
				},
				newPackage("p", "a.com/m/dir/p")),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testDB.GetUnit(ctx, tc.dirPath, tc.modulePath, tc.version, 0, internal.AllFields)
			if tc.wantNotFoundErr {
				if !errors.Is(err, derrors.NotFound) {
					t.Fatalf("want %v; got = \n%+v, %v", derrors.NotFound, got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
				cmpopts.IgnoreFields(internal.DirectoryMeta{}, "PathID"),
			}
			// TODO(golang/go#38513): remove once we start displaying
			// READMEs for directories instead of the top-level module.
			tc.want.Readme = &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: sample.ReadmeContents,
			}
			if tc.want.Package != nil {
				tc.want.Name = tc.want.Package.Name
				tc.want.Imports = sample.Imports
			}
			if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGetUnitFieldSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	// Add a module that has READMEs in a directory and a package.
	m := sample.Module("a.com/m", "v1.2.3", "dir/p")
	dir := findDirectory(m, "a.com/m/dir/p")
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	cleanFields := func(dir *internal.Unit, fields internal.FieldSet) {
		// Remove fields based on the FieldSet specified.
		dir.DirectoryMeta = internal.DirectoryMeta{
			Path:              dir.Path,
			IsRedistributable: true,
			ModuleInfo: internal.ModuleInfo{
				ModulePath: dir.ModulePath,
				Version:    dir.Version,
			},
		}
		if fields&internal.WithImports != 0 {
			dir.Imports = sample.Imports
		}
		if dir.Package != nil {
			dir.Package.Name = ""
		}
	}

	for _, test := range []struct {
		name   string
		fields internal.FieldSet
		want   *internal.Unit
	}{
		{
			name:   "WithDocumentation",
			fields: internal.WithDocumentation,
			want: newVdir("a.com/m/dir/p", "a.com/m", "v1.2.3",
				nil, newPackage("p", "a.com/m/dir/p")),
		},
		{
			name:   "WithImports",
			fields: internal.WithImports,
			want: newVdir("a.com/m/dir/p", "a.com/m", "v1.2.3",
				nil, nil),
		},
		{
			name:   "WithReadme",
			fields: internal.WithReadme,
			want: newVdir("a.com/m/dir/p", "a.com/m", "v1.2.3",
				&internal.Readme{
					Filepath: "README.md",
					Contents: "readme",
				}, nil),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := testDB.GetUnit(ctx, dir.Path, dir.ModulePath, dir.Version, 0, test.fields)
			if err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
				cmpopts.IgnoreFields(internal.DirectoryMeta{}, "PathID"),
			}
			cleanFields(test.want, test.fields)
			if diff := cmp.Diff(test.want, got, opts...); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func newVdir(path, modulePath, version string, readme *internal.Readme, pkg *internal.Package) *internal.Unit {
	dir := &internal.Unit{
		DirectoryMeta: internal.DirectoryMeta{
			ModuleInfo:        *sample.ModuleInfo(modulePath, version),
			Path:              path,
			V1Path:            path,
			IsRedistributable: true,
			Licenses:          sample.LicenseMetadata,
		},
		Readme:  readme,
		Package: pkg,
	}
	return dir
}

func newPackage(name, path string) *internal.Package {
	return &internal.Package{
		Name: name,
		Path: path,
		Documentation: &internal.Documentation{
			Synopsis: sample.Synopsis,
			HTML:     sample.DocumentationHTML,
			GOOS:     sample.GOOS,
			GOARCH:   sample.GOARCH,
		},
	}
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
		d, err := test.db.GetUnit(ctx, m.ModulePath, m.ModulePath, m.Version, 0, internal.AllFields)
		if err != nil {
			t.Fatal(err)
		}
		if got := (d.Readme == nil); got != test.wantEmpty {
			t.Errorf("readme empty: got %t, want %t", got, test.wantEmpty)
		}
		if got := (d.Package.Documentation == nil); got != test.wantEmpty {
			t.Errorf("synopsis empty: got %t, want %t", got, test.wantEmpty)
		}
		if got := (d.Package.Documentation == nil); got != test.wantEmpty {
			t.Errorf("doc empty: got %t, want %t", got, test.wantEmpty)
		}

		ld, err := test.db.LegacyGetDirectory(ctx, m.ModulePath, m.ModulePath, m.Version, internal.AllFields)
		if err != nil {
			t.Fatal(err)
		}
		if got := (ld.Packages[0].Synopsis == ""); got != test.wantEmpty {
			t.Errorf("legacy synopsis empty: got %t, want %t", got, test.wantEmpty)
		}
		if got := (ld.Packages[0].DocumentationHTML == safehtml.HTML{}); got != test.wantEmpty {
			t.Errorf("legacy doc empty: got %t, want %t", got, test.wantEmpty)
		}
	}
}

func findDirectory(m *internal.Module, path string) *internal.Unit {
	for _, d := range m.Units {
		d.ModuleInfo = m.ModuleInfo
		if d.Path == path {
			return d
		}
	}
	return nil
}

func TestLegacyGetDirectoryFieldSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	m := sample.Module("m.c", sample.VersionString, "d/p")
	m.LegacyPackages[0].Imports = nil
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	got, err := testDB.LegacyGetDirectory(ctx, "m.c/d", "m.c", sample.VersionString, internal.MinimalFields)
	if err != nil {
		t.Fatal(err)
	}
	if g, w := got.Packages[0].DocumentationHTML.String(), internal.StringFieldMissing; g != w {
		t.Errorf("DocumentationHTML = %q, want %q", g, w)
	}
}
