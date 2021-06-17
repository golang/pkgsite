// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

func TestGetUnitMeta(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout*2)
	defer cancel()
	testGetUnitMeta(t, ctx)
}

func testGetUnitMeta(t *testing.T, ctx context.Context) {
	testDB, release := acquire(t)
	defer release()

	for _, testModule := range []struct {
		module, version, packageSuffix string
		isMaster                       bool
		goMod                          string
	}{
		{"m.com", "v1.0.0", "a", false, ""},
		{"m.com", "v1.0.1", "dir/a", false, ""},
		{"m.com", "v2.0.0+incompatible", "a", false, ""},
		{"m.com", "v1.1.0", "a/b", false, "module m.com\nretract v1.0.1 // bad"},
		{"m.com", "v1.2.0-pre", "a", true, ""},
		{"m.com/a", "v1.1.0", "b", false, ""},
		{"m.com/b", "v2.0.0+incompatible", "a", true, ""},
		{"cloud.google.com/go", "v0.69.0", "pubsublite", false, ""},
		{"cloud.google.com/go/pubsublite", "v0.4.0", "", false, ""},
		{"cloud.google.com/go", "v0.74.0", "compute/metadata", false, ""},
		{"cloud.google.com/go/compute/metadata", "v0.0.0-20181115181204-d50f0e9b2506", "", false, "-"},
	} {
		m := sample.Module(testModule.module, testModule.version, testModule.packageSuffix)
		MustInsertModuleGoMod(ctx, t, testDB, m, testModule.goMod)
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

	checkUnitMeta := func(t *testing.T, ctx context.Context, test teststruct) {
		got, err := testDB.GetUnitMeta(ctx, test.path, test.module, test.version)
		if err != nil {
			t.Fatal(err)
		}
		opts := []cmp.Option{
			cmpopts.EquateEmpty(),
			cmpopts.IgnoreFields(licenses.Metadata{}, "Coverage", "OldCoverage"),
			cmpopts.IgnoreFields(internal.UnitMeta{}, "HasGoMod"),
			cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
		}
		if diff := cmp.Diff(test.want, got, opts...); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	}

	wantUnitMeta := func(modPath, version, name string) *internal.UnitMeta {
		return &internal.UnitMeta{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        modPath,
				Version:           version,
				IsRedistributable: true,
			},
			Name:              name,
			IsRedistributable: true,
		}
	}

	for _, test := range []teststruct{
		{
			name:    "known module and version",
			path:    "m.com/a",
			module:  "m.com",
			version: "v1.2.0-pre",
			want:    wantUnitMeta("m.com", "v1.2.0-pre", "a"),
		},
		{
			name:    "unknown module, known version",
			path:    "m.com/a/b",
			version: "v1.1.0",
			// The path is in two modules at v1.1.0. Prefer the longer one.
			want: wantUnitMeta("m.com/a", "v1.1.0", "b"),
		},
		{
			name:   "known module, unknown version",
			path:   "m.com/a",
			module: "m.com",
			// Choose the latest release version.
			want: wantUnitMeta("m.com", "v1.1.0", ""),
		},
		{

			name: "unknown module and version",
			path: "m.com/a/b",
			// Select the latest release version, longest module.
			want: wantUnitMeta("m.com/a", "v1.1.0", "b"),
		},
		{
			name: "module",
			path: "m.com",
			// Select the latest version of the module.
			want: wantUnitMeta("m.com", "v1.1.0", ""),
		},
		{
			name:    "longest module",
			path:    "m.com/a",
			version: "v1.1.0",
			// Prefer module m/a over module m, directory a.
			want: wantUnitMeta("m.com/a", "v1.1.0", ""),
		},
		{
			name: "directory",
			path: "m.com/dir",
			want: func() *internal.UnitMeta {
				um := wantUnitMeta("m.com", "v1.0.1", "")
				um.Retracted = true
				um.RetractionRationale = "bad"
				return um
			}(),
		},
		{
			name:    "module at master version",
			path:    "m.com",
			version: "master",
			want:    wantUnitMeta("m.com", "v1.2.0-pre", ""),
		},
		{
			name:    "package at master version",
			path:    "m.com/a",
			version: "master",
			want:    wantUnitMeta("m.com", "v1.2.0-pre", "a"),
		},
		{
			name:    "incompatible module",
			path:    "m.com/b",
			version: "master",
			want:    wantUnitMeta("m.com/b", "v2.0.0+incompatible", ""),
		},
		{
			name: "prefer pubsublite nested module",
			path: "cloud.google.com/go/pubsublite",
			want: wantUnitMeta("cloud.google.com/go/pubsublite", "v0.4.0", "pubsublite"),
		},
		{
			name: "prefer compute metadata in main module",
			path: "cloud.google.com/go/compute/metadata",
			want: wantUnitMeta("cloud.google.com/go", "v0.74.0", "metadata"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.module == "" {
				test.module = internal.UnknownModulePath
			}
			if test.version == "" {
				test.version = version.Latest
			}
			want := sample.UnitMeta(
				test.path,
				test.want.ModulePath,
				test.want.Version,
				test.want.Name,
				test.want.IsRedistributable,
			)
			want.CommitTime = sample.CommitTime
			want.Retracted = test.want.Retracted
			want.RetractionRationale = test.want.RetractionRationale
			test.want = want
			checkUnitMeta(t, ctx, test)
		})
	}
}

func TestGetUnitMetaBypass(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	bypassDB := NewBypassingLicenseCheck(testDB.db)

	for _, testModule := range []struct {
		module, version, packageSuffix string
		isMaster                       bool
	}{
		{"m.com", "v2.0.0+incompatible", "a", false},
		{"m.com/b", "v2.0.0+incompatible", "a", true},
		{"m.com", "v1.0.0", "a", false},
		{"m.com", "v1.0.1", "dir/a", false},
		{"m.com", "v1.1.0", "a/b", false},
		{"m.com", "v1.2.0-pre", "a", true},
		{"m.com/a", "v1.1.0", "b", false},
	} {
		m := sample.Module(testModule.module, testModule.version, testModule.packageSuffix)
		makeModuleNonRedistributable(m)

		MustInsertModule(ctx, t, bypassDB, m)
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

	wantUnitMeta := func(modPath, version, name string, isRedist bool) *internal.UnitMeta {
		return &internal.UnitMeta{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        modPath,
				Version:           version,
				IsRedistributable: false,
			},
			Name:              name,
			IsRedistributable: isRedist,
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
				want:    wantUnitMeta("m.com", "v1.2.0-pre", "a", bypassLicenseCheck),
			},
			{
				name:    "unknown module, known version",
				path:    "m.com/a/b",
				version: "v1.1.0",
				// The path is in two modules at v1.1.0. Prefer the longer one.
				want: wantUnitMeta("m.com/a", "v1.1.0", "b", bypassLicenseCheck),
			},
			{
				name:   "known module, unknown version",
				path:   "m.com/a",
				module: "m.com",
				// Choose the latest release version.
				want: wantUnitMeta("m.com", "v1.1.0", "", bypassLicenseCheck),
			},
			{
				name: "unknown module and version",
				path: "m.com/a/b",
				// Select the latest release version, longest module.
				want: wantUnitMeta("m.com/a", "v1.1.0", "b", bypassLicenseCheck),
			},
			{
				name: "module",
				path: "m.com",
				// Select the latest version of the module.
				want: wantUnitMeta("m.com", "v1.1.0", "", bypassLicenseCheck),
			},
			{
				name:    "longest module",
				path:    "m.com/a",
				version: "v1.1.0",
				// Prefer module m/a over module m, directory a.
				want: wantUnitMeta("m.com/a", "v1.1.0", "", bypassLicenseCheck),
			},
			{
				name: "directory",
				path: "m.com/dir",
				want: wantUnitMeta("m.com", "v1.0.1", "", bypassLicenseCheck),
			},
			{
				name:    "module at master version",
				path:    "m.com",
				version: "master",
				want:    wantUnitMeta("m.com", "v1.2.0-pre", "", bypassLicenseCheck),
			},
			{
				name:    "package at master version",
				path:    "m.com/a",
				version: "master",
				want:    wantUnitMeta("m.com", "v1.2.0-pre", "a", bypassLicenseCheck),
			},
			{
				name:    "incompatible module",
				path:    "m.com/b",
				version: "master",
				want:    wantUnitMeta("m.com/b", "v2.0.0+incompatible", "", bypassLicenseCheck),
			},
		} {
			name := fmt.Sprintf("bypass %v %s", bypassLicenseCheck, test.name)
			t.Run(name, func(t *testing.T) {
				if test.module == "" {
					test.module = internal.UnknownModulePath
				}
				if test.version == "" {
					test.version = version.Latest
				}
				test.want = sample.UnitMeta(
					test.path,
					test.want.ModulePath,
					test.want.Version,
					test.want.Name,
					test.want.IsRedistributable,
				)
				test.want.ModuleInfo.IsRedistributable = false
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
					cmpopts.EquateEmpty(),
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

func TestGetLatestUnitVersion(t *testing.T) {
	ctx := context.Background()

	type pgm struct { // package as mod@ver/pkg, and go.mod
		pkg   string
		goMod string // go.mod file contents after "module" line, or "0" to omit
	}

	for _, test := range []struct {
		name                 string
		packages             []pgm
		badModules           []pgm  // have latest-version info, but not in the DB
		fullPath, modulePath string // inputs to getLatestUnitVersion
		want                 string
		wantNilLMV           bool // lmv return value is null
	}{
		{
			name: "known module",
			// If the module is known and there is latest-version information
			// for it, then the result is the latest version that contains the
			// full path.
			packages: []pgm{
				{"m.com@v1.4.0/b", ""},     // latest version, but no package "a"
				{"m.com@v1.3.0-pre/a", ""}, // pre-release
				{"m.com@v1.2.0/a", ""},     // latest version with package a
				{"m.com@v1.1.0/a", ""},
			},
			fullPath:   "m.com/a",
			modulePath: "m.com",
			want:       "m.com@v1.2.0",
		},
		{
			name: "longest",
			// If the module is unknown, then the longest module with the full
			// path is used, even if a shorter module has a later version.
			packages: []pgm{
				{"m.com@v1.3.0/a/b", ""}, // shorter path, later version
				{"m.com/a@v1.0.0/b", ""}, // longer path, earlier version
			},
			fullPath: "m.com/a/b",
			want:     "m.com/a@v1.0.0",
		},
		{
			name: "prerelease",
			// Use the latest pre-release version if there are no release versions.
			packages: []pgm{
				{"m.com@v1.2.0-pre/a", ""},
				{"m.com@v1.3.0-pre/a", ""},
				{"m.com@v1.1.0-pre/a", ""},
			},
			fullPath: "m.com/a",
			want:     "m.com@v1.3.0-pre",
		},
		{
			name: "retracted",
			// Skip retracted versions.
			packages: []pgm{
				{"m.com@v2.0.0+incompatible/a", ""},  // ignored: cooked latest version is compatible
				{"m.com@v1.3.0/a", "retract v1.3.0"}, // retracts itself
				{"m.com@v1.2.0/a", ""},
			},
			fullPath: "m.com/a",
			want:     "m.com@v1.2.0", // latest unretracted compatible version
		},
		{
			name: "latest bad",
			// The latest version is not in the DB, but is still used for
			// retractions and to decide whether to ignore incompatible
			// versions.
			packages: []pgm{
				{"m.com@v2.0.0+incompatible/a", ""}, // ignored: cooked latest version is compatible
				{"m.com@v1.3.0/a", ""},
				{"m.com@v1.2.0/a", ""},
			},
			// Latest raw version retracts v1.3.0.
			// Latest cooked version is compatible, so ignore incompatible.
			badModules: []pgm{{"m.com@v1.4.0", "retract v1.3.0"}},
			fullPath:   "m.com/a",
			want:       "m.com@v1.2.0", // latest unretracted compatible version
		},
		{
			name: "incompatible",
			// Incompatible versions aren't skipped if the latest compatible
			// version does not have a go.mod file, indicated by the
			// latest-version info having an incompatible cooked version.
			packages: []pgm{
				{"m.com@v1.3.0/a", "-"},
				{"m.com@v1.2.0/a", ""},
				{"m.com@v2.0.0+incompatible/a", ""},
			},
			fullPath: "m.com/a",
			want:     "m.com@v2.0.0+incompatible",
		},
		{
			name: "only incompatible",
			// If incompatible versions are all we've got, return one.
			packages: []pgm{
				{"m.com@v3.0.0+incompatible/a", ""},
				{"m.com@v2.0.0+incompatible/a", ""},
			},
			badModules: []pgm{{"m.com@v1.2.3", ""}}, // latest version is compatible but bad
			fullPath:   "m.com/a",
			want:       "m.com@v3.0.0+incompatible",
			wantNilLMV: true,
		},
		{
			name: "no latest",
			// Without latest-version information, use whatever the DB has.
			// Prefer longer paths, and release versions to pre-release.
			packages: []pgm{
				{"m.com@v1.4.0/a/b", "-"},     // shorter module path
				{"m.com/a@v1.3.0/c", "-"},     // does not contain full path
				{"m.com/a@v1.2.0-pre/b", "-"}, // pre-release
				{"m.com/a@v1.1.0/b", "-"},
			},
			fullPath:   "m.com/a/b",
			want:       "m.com/a@v1.1.0",
			wantNilLMV: true,
		},
		{
			name: "no latest incompatible",
			// Without latest-version information, use whatever the DB has.
			// Prefer longer paths, and later versions even if they are
			// incompatible.
			packages: []pgm{
				{"m.com@v2.3.0+incompatible/a/b", "-"}, // shorter module path
				{"m.com/a@v2.1.0+incompatible/b", "-"},
				{"m.com/a@v2.0.0+incompatible/b", "-"},
				{"m.com/a@v2.2.0+incompatible/c", "-"}, // does not contain path
			},
			fullPath:   "m.com/a/b",
			want:       "m.com/a@v2.1.0+incompatible",
			wantNilLMV: true,
		},
		{
			name: "prefer latest info",
			// Given a choice between a longer module path with no
			// latest-version info and a shorter with it, choose the shorter.
			// Real-world example: prefer
			// cloud.google.com/go@vX/compute/metadata to
			// cloud.google.com/go/compute/metadata@vX because the latter, while
			// a module, has no latest-version info.
			packages: []pgm{
				{"m.com@v1.0.0/a/b", ""},
				{"m.com/a@v1.0.0/b", "-"},
			},
			fullPath: "m.com/a/b",
			want:     "m.com@v1.0.0",
		},
		{
			name: "all retracted",
			// If all the versions containing the path are retracted, then return
			// the latest anyway.
			packages: []pgm{
				{"m.com@v1.1.0/a/b", ""},
				{"m.com@v1.2.0/a/b", ""},
				{"m.com@v1.3.0/a/c", "retract [v1.0.0, v1.2.0]"},
			},
			fullPath:   "m.com/a/b",
			want:       "m.com@v1.2.0",
			wantNilLMV: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testDB, release := acquire(t)
			defer release()

			for _, p := range test.packages {
				mod, ver, pkg := parseModuleVersionPackage(p.pkg)
				m := sample.Module(mod, ver, pkg)
				MustInsertModuleGoMod(ctx, t, testDB, m, p.goMod)
			}
			for _, b := range test.badModules {
				mod, ver, _ := parseModuleVersionPackage(b.pkg)
				addLatest(ctx, t, testDB, mod, ver, b.goMod)
			}

			if test.modulePath == "" {
				test.modulePath = internal.UnknownModulePath
			}
			gotPath, gotVersion, gotLMV, err := testDB.getLatestUnitVersion(ctx, test.fullPath, test.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			got := gotPath + "@" + gotVersion
			if got != test.want {
				t.Errorf("got %s, want %s", got, test.want)
			}
			gotNilLMV := gotLMV == nil
			if gotNilLMV != test.wantNilLMV {
				t.Errorf("got nil LMV %t, want %t", gotNilLMV, test.wantNilLMV)
			}
		})
	}
}

// parse mod@ver/pkg into parts.
func parseModuleVersionPackage(s string) (mod, ver, pkg string) {
	at := strings.IndexRune(s, '@')
	mod, s = s[:at], s[at+1:]
	slash := strings.IndexRune(s, '/')
	if slash < 0 {
		ver = s
	} else {
		ver, pkg = s[:slash], s[slash+1:]
	}
	return mod, ver, pkg
}

func TestGetUnit(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
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
	MustInsertModule(ctx, t, testDB, m)

	// Add a module that has documentation for two Go build contexts.
	m = sample.Module("a.com/twodoc", "v1.2.3", "p")
	pkg := m.Packages()[0]
	docs2 := []*internal.Documentation{
		sample.Documentation("linux", "amd64", `package p; var L int`),
		sample.Documentation("windows", "amd64", `package p; var W int`),
	}
	pkg.Documentation = docs2
	MustInsertModule(ctx, t, testDB, m)

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
				u.Documentation = docs2[:1]
				u.BuildContexts = []internal.BuildContext{internal.BuildContextLinux, internal.BuildContextWindows}
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
			checkUnit(ctx, t, testDB, um, test.want)
		})
	}
}

func checkUnit(ctx context.Context, t *testing.T, db *DB, um *internal.UnitMeta, want *internal.Unit) {
	t.Helper()
	got, err := db.GetUnit(ctx, um, internal.AllFields, internal.BuildContext{})
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
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreFields(internal.Unit{}, "Imports", "LicenseContents"),
	)
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Errorf("mismatch (-want, +got):\n%s", diff)
	}
}

func TestGetUnit_SubdirectoriesShowNonRedistPackages(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	m := sample.DefaultModule()
	m.IsRedistributable = false
	m.Packages()[0].IsRedistributable = false
	MustInsertModule(ctx, t, testDB, m)
}

func TestGetUnitFieldSet(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	readme := &internal.Readme{
		Filepath: "a.com/m/dir/p/README.md",
		Contents: "readme",
	}
	// Add a module that has READMEs in a directory and a package.
	m := sample.Module("a.com/m", "v1.2.3", "dir/p")
	m.Packages()[0].Readme = readme
	MustInsertModule(ctx, t, testDB, m)

	cleanFields := func(u *internal.Unit, fields internal.FieldSet) {
		// Add/remove fields based on the FieldSet specified.
		if fields&internal.WithMain != 0 {
			u.Documentation = []*internal.Documentation{sample.Doc}
			u.BuildContexts = []internal.BuildContext{internal.BuildContextAll}
			u.Readme = readme
			u.NumImports = len(sample.Imports())
			u.Subdirectories = []*internal.PackageMeta{
				{
					Path:              "a.com/m/dir/p",
					Name:              "p",
					Synopsis:          sample.Doc.Synopsis,
					IsRedistributable: true,
					Licenses:          sample.LicenseMetadata(),
				},
			}
		}
		if fields&internal.WithImports != 0 {
			imps := sample.Imports()
			u.Imports = imps
			u.NumImports = len(imps)
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
			got, err := testDB.GetUnit(ctx, um, test.fields, internal.BuildContext{})
			if err != nil {
				t.Fatal(err)
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(source.Info{}, safehtml.HTML{}),
				cmpopts.EquateEmpty(),
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

func TestGetUnitBuildContext(t *testing.T) {
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Add a module that has documentation for two Go build contexts.
	m := sample.Module("a.com/twodoc", "v1.2.3", "p")
	pkg := m.Packages()[0]
	linuxDoc := sample.Documentation("linux", "amd64", `package p; var L int`)
	windowsDoc := sample.Documentation("windows", "amd64", `package p; var W int`)
	pkg.Documentation = []*internal.Documentation{linuxDoc, windowsDoc}
	MustInsertModule(ctx, t, testDB, m)

	um := sample.UnitMeta(
		"a.com/twodoc/p",
		"a.com/twodoc",
		"v1.2.3",
		"p",
		true)
	for _, test := range []struct {
		goos, goarch string
		want         *internal.Documentation
	}{
		{"", "", linuxDoc},
		{"linux", "amd64", linuxDoc},
		{"windows", "amd64", windowsDoc},
		{"linux", "", linuxDoc},
		{"wasm", "js", nil},
	} {
		t.Run(fmt.Sprintf("%s-%s", test.goos, test.goarch), func(t *testing.T) {
			bc := internal.BuildContext{GOOS: test.goos, GOARCH: test.goarch}
			u, err := testDB.GetUnit(ctx, um, internal.WithMain, bc)
			if err != nil {
				t.Fatal(err)
			}
			got := u.Documentation
			var want []*internal.Documentation
			if test.want != nil {
				want = []*internal.Documentation{test.want}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
			wantb := []internal.BuildContext{internal.BuildContextLinux, internal.BuildContextWindows}
			if got := u.BuildContexts; !cmp.Equal(got, wantb) {
				t.Errorf("got %v, want %v", got, wantb)
			}
		})
	}
}

func unit(fullPath, modulePath, version, name string, readme *internal.Readme, suffixes []string) *internal.Unit {
	u := &internal.Unit{
		UnitMeta: internal.UnitMeta{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        modulePath,
				Version:           version,
				IsRedistributable: true,
			},
			Path:              fullPath,
			IsRedistributable: true,
			Licenses:          sample.LicenseMetadata(),
			Name:              name,
		},
		LicenseContents: sample.Licenses(),
		Readme:          readme,
	}

	u.Subdirectories = subdirectories(modulePath, suffixes)
	if u.IsPackage() {
		imps := sample.Imports()
		u.Imports = imps
		u.NumImports = len(imps)
		u.Documentation = []*internal.Documentation{sample.Doc}
		u.BuildContexts = []internal.BuildContext{internal.BuildContextAll}
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
	t.Parallel()
	testDB, release := acquire(t)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	bypassDB := NewBypassingLicenseCheck(testDB.db)

	m := nonRedistributableModule()
	MustInsertModule(ctx, t, bypassDB, m)

	for _, test := range []struct {
		db        *DB
		wantEmpty bool
	}{
		{testDB, true},
		{bypassDB, false},
	} {
		pathInfo := newUnitMeta(m.ModulePath, m.ModulePath, m.Version)
		d, err := test.db.GetUnit(ctx, pathInfo, internal.AllFields, internal.BuildContext{})
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

func newUnitMeta(path, modulePath, version string) *internal.UnitMeta {
	return &internal.UnitMeta{
		Path: path,
		ModuleInfo: internal.ModuleInfo{
			ModulePath: modulePath,
			Version:    version,
		},
	}
}
