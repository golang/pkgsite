// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
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
	} {
		t.Run(test.name, func(t *testing.T) {
			um := sample.UnitMeta(
				test.path,
				test.modulePath,
				test.version,
				test.want.Name,
				test.want.IsRedistributable,
			)
			t.Run("unit page with one query", func(t *testing.T) {
				checkUnit(ctx, t, um, test.want, internal.ExperimentUnitPage, internal.ExperimentGetUnitWithOneQuery)
			})
			t.Run("unit page", func(t *testing.T) {
				checkUnit(ctx, t, um, test.want, internal.ExperimentUnitPage)
			})
			t.Run("no experiments", func(t *testing.T) {
				test.want.Readme = &internal.Readme{
					Filepath: sample.ReadmeFilePath,
					Contents: sample.ReadmeContents,
				}
				checkUnit(ctx, t, um, test.want)
			})
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
		cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
	}
	want.SourceInfo = um.SourceInfo
	if experiment.IsActive(ctx, internal.ExperimentGetUnitWithOneQuery) {
		want.NumImports = len(want.Imports)
		opts = append(opts,
			cmpopts.IgnoreFields(internal.Documentation{}, "HTML"),
			cmpopts.IgnoreFields(internal.Unit{}, "Imports"),
			cmpopts.IgnoreFields(internal.Unit{}, "LicenseContents"),
		)
		if diff := cmp.Diff(want, got, opts...); diff != "" {
			t.Errorf("mismatch (-want, +got):\n%s", diff)
		}
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
			name:   "WithDocumentation",
			fields: internal.WithDocumentation,
			want:   unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "", nil, []string{}),
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
		{
			name:   "WithReadme",
			fields: internal.WithReadme,
			want: unit("a.com/m/dir/p", "a.com/m", "v1.2.3", "",
				&internal.Readme{
					Filepath: "README.md",
					Contents: "readme",
				}, []string{}),
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
			}
			test.want.SourceInfo = pathInfo.SourceInfo
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
		u.Documentation = sample.Documentation
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
