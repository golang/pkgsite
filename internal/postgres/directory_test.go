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
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetDirectory(t *testing.T) {
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
				"internal/foo",
				"builtin/audit/file",
				"builtin/audit/socket",
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

			mi := sample.ModuleInfo(tc.wantModulePath, tc.wantVersion)
			var wantPackages []*internal.Package
			for _, suffix := range tc.wantSuffixes {
				pkg := sample.Package(tc.wantModulePath, suffix)
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

func TestGetDirectoryNew(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	ctx = experiment.NewContext(ctx,
		experiment.NewSet(map[string]bool{
			internal.ExperimentInsertDirectories: true}))

	defer ResetTestDB(testDB, t)

	InsertSampleDirectoryTree(ctx, t, testDB)

	// Add a module that has READMEs in a directory and a package.
	m := sample.Module("a.com/m", "v1.2.3", "dir/p")
	d := sample.DirectoryNewEmpty("a.com/m/dir")
	d.Readme = &internal.Readme{
		Filepath: "DIR_README.md",
		Contents: "dir readme",
	}
	m.Directories = append(m.Directories, d)
	d = sample.DirectoryNewEmpty("a.com/m/dir/p")
	d.Readme = &internal.Readme{
		Filepath: "PKG_README.md",
		Contents: "pkg readme",
	}
	m.Directories = append(m.Directories, d)
	if err := testDB.InsertModule(ctx, m); err != nil {
		t.Fatal(err)
	}

	newVdir := func(path, modulePath, version string, readme *internal.Readme, pkg *internal.PackageNew) *internal.VersionedDirectory {
		return &internal.VersionedDirectory{
			ModuleInfo: *sample.ModuleInfo(modulePath, version),
			DirectoryNew: internal.DirectoryNew{
				Path:              path,
				V1Path:            path,
				IsRedistributable: true,
				Licenses:          sample.LicenseMetadata,
				Readme:            readme,
				Package:           pkg,
			},
		}
	}

	newPackage := func(name, path string) *internal.PackageNew {
		return &internal.PackageNew{
			Name: name,
			Path: path,
			Documentation: &internal.Documentation{
				Synopsis: sample.Synopsis,
				HTML:     sample.DocumentationHTML,
				GOOS:     sample.GOOS,
				GOARCH:   sample.GOARCH,
			},
			Imports: sample.Imports,
		}
	}

	for _, tc := range []struct {
		name, dirPath, modulePath, version string
		want                               *internal.VersionedDirectory
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
			got, err := testDB.GetDirectoryNew(ctx, tc.dirPath, tc.modulePath, tc.version)
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
				cmp.AllowUnexported(source.Info{}),
				// The packages table only includes partial license information; it omits the Coverage field.
				cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage"),
			}
			if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}

func TestGetDirectoryFieldSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer ResetTestDB(testDB, t)

	m := sample.Module("m.c", sample.VersionString, "d/p")
	m.Packages[0].Imports = nil
	if err := testDB.InsertModule(ctx, m); err != nil {
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
