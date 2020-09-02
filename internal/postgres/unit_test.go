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
		name, fullPath, modulePath, version, wantModulePath, wantVersion string
		wantSuffixes                                                     []string
		wantNotFoundErr                                                  bool
	}{
		{
			name:           "directory path is the module path",
			fullPath:       "github.com/hashicorp/vault",
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
			fullPath:       "github.com/hashicorp/vault",
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
			fullPath:       "github.com/hashicorp/vault/builtin",
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
			fullPath:       stdlib.ModulePath,
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
			fullPath:       "archive",
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
			fullPath:       "archive/zip",
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
			fullPath:        "archive/zi",
			modulePath:      stdlib.ModulePath,
			version:         "v1.13.4",
			wantNotFoundErr: true,
		},
		{
			name:           "stdlib - internal directory",
			fullPath:       "cmd/internal",
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
			fullPath:       "cmd/internal/obj",
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
			got, err := testDB.GetPackagesInUnit(ctx, tc.fullPath, tc.modulePath, tc.version)
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

	for _, test := range []struct {
		name, path, modulePath, version string
		want                            *internal.Unit
		wantNotFoundErr                 bool
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
				}),
		},
		{
			name:       "package path",
			path:       "github.com/hashicorp/vault/api",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want:       unit("github.com/hashicorp/vault/api", "github.com/hashicorp/vault", "v1.0.3", "api", nil),
		},
		{
			name:       "directory path",
			path:       "github.com/hashicorp/vault/builtin",
			modulePath: "github.com/hashicorp/vault",
			version:    "v1.0.3",
			want:       unit("github.com/hashicorp/vault/builtin", "github.com/hashicorp/vault", "v1.0.3", "", nil),
		},
		{
			name:       "stdlib directory",
			path:       "archive",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want:       unit("archive", stdlib.ModulePath, "v1.13.4", "", nil),
		},
		{
			name:       "stdlib package",
			path:       "archive/zip",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want:       unit("archive/zip", stdlib.ModulePath, "v1.13.4", "zip", nil),
		},
		{
			name:       "stdlib - internal directory",
			path:       "cmd/internal",
			modulePath: stdlib.ModulePath,
			version:    "v1.13.4",
			want:       unit("cmd/internal", stdlib.ModulePath, "v1.13.4", "", nil),
		},
		{
			name:       "directory with readme",
			path:       "a.com/m/dir",
			modulePath: "a.com/m",
			version:    "v1.2.3",
			want: unit("a.com/m/dir", "a.com/m", "v1.2.3", "", &internal.Readme{
				Filepath: "DIR_README.md",
				Contents: "dir readme",
			}),
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
				}),
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
			got, err := testDB.GetUnit(ctx, um, internal.AllFields)
			if test.wantNotFoundErr {
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
			test.want.Readme = &internal.Readme{
				Filepath: sample.ReadmeFilePath,
				Contents: sample.ReadmeContents,
			}
			test.want.SourceInfo = um.SourceInfo
			if diff := cmp.Diff(test.want, got, opts...); diff != "" {
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
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	cleanFields := func(u *internal.Unit, fields internal.FieldSet) {
		// Add/remove fields based on the FieldSet specified.
		if fields&internal.WithDocumentation != 0 {
			u.Documentation = sample.Documentation
		}
		if fields&internal.WithImports != 0 {
			u.Imports = sample.Imports
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
			name:   "WithDocumentation",
			fields: internal.WithDocumentation,
			want:   unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "", nil),
		},
		{
			name:   "WithImports",
			fields: internal.WithImports,
			want:   unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "", nil),
		},
		{
			name:   "WithLicenses",
			fields: internal.WithLicenses,
			want:   unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "", nil),
		},
		{
			name:   "WithReadme",
			fields: internal.WithReadme,
			want: unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "",
				&internal.Readme{
					Filepath: "README.md",
					Contents: "readme",
				}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pathInfo := sample.UnitMeta(
				test.want.Path,
				test.want.ModulePath,
				test.want.Version,
				test.want.Name,
				test.want.IsRedistributable,
			)
			got, err := testDB.GetUnit(ctx, pathInfo, test.fields)
			if err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
				cmpopts.IgnoreFields(internal.DirectoryMeta{}, "PathID"),
			}
			test.want.SourceInfo = pathInfo.SourceInfo
			cleanFields(test.want, test.fields)
			if diff := cmp.Diff(test.want, got, opts...); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func unit(path, modulePath, version, name string, readme *internal.Readme) *internal.Unit {
	u := &internal.Unit{
		UnitMeta: internal.UnitMeta{
			ModulePath:        modulePath,
			Version:           version,
			Path:              path,
			IsRedistributable: true,
			Licenses:          sample.LicenseMetadata,
			Name:              name,
		},
		LicenseContents: sample.Licenses,
		Readme:          readme,
	}
	if u.IsPackage() {
		u.Imports = sample.Imports
		u.Documentation = sample.Documentation
	}
	return u
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
		if d.Path == path {
			return d
		}
	}
	return nil
}
