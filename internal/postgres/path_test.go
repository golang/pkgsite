// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"path"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

func TestGetPathInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	ctx = experiment.NewContext(ctx, experiment.NewSet(map[string]bool{
		internal.ExperimentInsertDirectories: true,
	}))

	defer ResetTestDB(testDB, t)

	for _, testModule := range []struct {
		module, version, packageSuffix string
	}{
		{"m.com", "v1.0.0", "a"},
		{"m.com", "v1.0.1", "dir/a"},
		{"m.com", "v1.1.0", "a/b"},
		{"m.com", "v1.2.0-pre", "a"},
		{"m.com/a", "v1.1.0", "b"},
	} {
		vtype, err := version.ParseType(testModule.version)
		if err != nil {
			t.Fatal(err)
		}

		pkgName := path.Base(testModule.packageSuffix)
		pkgPath := path.Join(testModule.module, testModule.packageSuffix)
		m := &internal.Module{
			ModuleInfo: internal.ModuleInfo{
				ModulePath:  testModule.module,
				Version:     testModule.version,
				VersionType: vtype,
				CommitTime:  time.Now(),
			},
			LegacyPackages: []*internal.LegacyPackage{{
				Name: pkgName,
				Path: pkgPath,
			}},
		}
		for d := pkgPath; d != "." && len(d) >= len(testModule.module); d = path.Dir(d) {
			dir := &internal.DirectoryNew{Path: d}
			if d == pkgPath {
				dir.Package = &internal.PackageNew{
					Path:          pkgPath,
					Name:          pkgName,
					Documentation: &internal.Documentation{},
				}
			}
			m.Directories = append(m.Directories, dir)
		}
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name                    string
		path, module, version   string
		wantModule, wantVersion string
		wantIsPackage           bool
	}{
		{
			name:          "known module and version",
			path:          "m.com/a",
			module:        "m.com",
			version:       "v1.2.0-pre",
			wantModule:    "m.com",
			wantVersion:   "v1.2.0-pre",
			wantIsPackage: true,
		},
		{
			name:    "unknown module, known version",
			path:    "m.com/a/b",
			version: "v1.1.0",
			// The path is in two modules at v1.1.0. Prefer the longer one.
			wantModule:    "m.com/a",
			wantVersion:   "v1.1.0",
			wantIsPackage: true,
		},
		{
			name:   "known module, unknown version",
			path:   "m.com/a",
			module: "m.com",
			// Choose the latest release version.
			wantModule:    "m.com",
			wantVersion:   "v1.1.0",
			wantIsPackage: false,
		},
		{
			name: "unknown module and version",
			path: "m.com/a/b",
			// Select the latest release version, longest module.
			wantModule:    "m.com/a",
			wantVersion:   "v1.1.0",
			wantIsPackage: true,
		},
		{
			name: "module",
			path: "m.com",
			// Select the latest version of the module.
			wantModule:    "m.com",
			wantVersion:   "v1.1.0",
			wantIsPackage: false,
		},
		{
			name:    "longest module",
			path:    "m.com/a",
			version: "v1.1.0",
			// Prefer module m/a over module m, directory a.
			wantModule:    "m.com/a",
			wantVersion:   "v1.1.0",
			wantIsPackage: false, //  m/a is a module
		},
		{
			name:          "directory",
			path:          "m.com/dir",
			wantModule:    "m.com",
			wantVersion:   "v1.0.1",
			wantIsPackage: false, //  m/dir is a directory
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.module == "" {
				test.module = internal.UnknownModulePath
			}
			if test.version == "" {
				test.version = internal.LatestVersion
			}
			gotModule, gotVersion, gotIsPackage, err := testDB.GetPathInfo(ctx, test.path, test.module, test.version)
			if err != nil {
				t.Fatal(err)
			}
			if gotModule != test.wantModule || gotVersion != test.wantVersion || gotIsPackage != test.wantIsPackage {
				t.Errorf("got (%q, %q, %t), want (%q, %q, %t)",
					gotModule, gotVersion, gotIsPackage,
					test.wantModule, test.wantVersion, test.wantIsPackage)
			}
		})
	}
}

func TestGetStdlibPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	ctx = experiment.NewContext(ctx, experiment.NewSet(map[string]bool{
		internal.ExperimentInsertDirectories: true,
	}))
	defer ResetTestDB(testDB, t)

	// Insert two versions of some stdlib packages.
	for _, data := range []struct {
		version  string
		suffixes []string
	}{
		{
			// earlier version; should be ignored
			"v1.1.0",
			[]string{"bad/json"},
		},
		{
			"v1.2.0",
			[]string{
				"encoding/json",
				"archive/json",
				"net/http",     // no "json"
				"foo/json/moo", // "json" not the last component
				"bar/xjson",    // "json" not alone
				"baz/jsonx",    // ditto
			},
		},
	} {
		m := sample.Module(stdlib.ModulePath, data.version, data.suffixes...)
		for _, p := range m.LegacyPackages {
			p.Imports = nil
		}
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	got, err := testDB.GetStdlibPathsWithSuffix(ctx, "json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"archive/json", "encoding/json"}
	if !cmp.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
