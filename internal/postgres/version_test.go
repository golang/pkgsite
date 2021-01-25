// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		taggedAndPseudoModule = "path.to/foo"
		taggedModuleV2        = "path.to/foo/v2"
		taggedModuleV3        = "path.to/foo/v3"
		pseudoModule          = "golang.org/x/tools"
		otherModule           = "path.to/other"
		incompatibleModule    = "path.to/incompatible"
		rootModule            = "golang.org/foo/bar"
		nestedModule          = "golang.org/foo/bar/api"
		testModules           = []*internal.Module{
			sample.Module(stdlib.ModulePath, "v1.15.0-beta.1", "cmd/go"),
			sample.Module(stdlib.ModulePath, "v1.14.6", "cmd/go"),
			sample.Module(taggedModuleV3, "v3.2.0-beta", "bar"),
			sample.Module(taggedModuleV3, "v3.2.0-alpha.2", "bar"),
			sample.Module(taggedModuleV3, "v3.2.0-alpha.1", "bar"),
			sample.Module(taggedModuleV3, "v3.1.0", "bar"),
			sample.Module(taggedModuleV3, "v3.0.0", "bar"),
			sample.Module(taggedModuleV2, "v2.0.1", "bar"),
			sample.Module(taggedAndPseudoModule, "v1.5.3-pre1", "bar"),
			sample.Module(taggedAndPseudoModule, "v1.5.2", "bar"),
			sample.Module(taggedAndPseudoModule, "v0.0.0-20200101120000-000000000000", "bar"),
			sample.Module(otherModule, "v3.0.0", "thing"),
			sample.Module(incompatibleModule, "v2.0.0+incompatible", "module"),
			sample.Module(incompatibleModule, "v0.0.0", "module"),
			sample.Module(rootModule, "v1.0.3", "api"),
			sample.Module(rootModule, "v0.11.6", "api"),
			sample.Module(nestedModule, "v1.0.4", "api"),
			sample.Module(nestedModule, "v1.0.3", "api"),
		}
	)

	// Add 12 pseudo versions to the test modules. Below we only
	// expect to return the 10 most recent.
	for i := 1; i <= 12; i++ {
		testModules = append(testModules, sample.Module(pseudoModule, fmt.Sprintf("v0.0.0-202001011200%02d-000000000000", i), "blog"))
	}

	defer ResetTestDB(testDB, t)
	for _, m := range testModules {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	stdModuleVersions := []*internal.ModuleInfo{
		{
			ModulePath: stdlib.ModulePath,
			Version:    "v1.15.0-beta.1",
		},
		{
			ModulePath: stdlib.ModulePath,
			Version:    "v1.14.6",
		},
	}

	fooModuleVersions := []*internal.ModuleInfo{
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.2.0-beta",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.2.0-alpha.2",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.2.0-alpha.1",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.1.0",
		},
		{
			ModulePath: taggedModuleV3,
			Version:    "v3.0.0",
		},
		{
			ModulePath: taggedModuleV2,
			Version:    "v2.0.1",
		},
		{
			ModulePath: taggedAndPseudoModule,
			Version:    "v1.5.3-pre1",
		},
		{
			ModulePath: taggedAndPseudoModule,
			Version:    "v1.5.2",
		},
	}

	testCases := []struct {
		name, path string
		want       []*internal.ModuleInfo
	}{
		{
			name: "std_module",
			path: stdlib.ModulePath,
			want: stdModuleVersions,
		},
		{
			name: "stdlib_path",
			path: "cmd",
			want: stdModuleVersions,
		},
		{
			name: "stdlib_package",
			path: "cmd/go",
			want: stdModuleVersions,
		},
		{
			name: "want_tagged_versions_only",
			path: "path.to/foo/bar",
			want: fooModuleVersions,
		},
		{
			name: "want_all_tagged_versions_at_v2_path",
			path: "path.to/foo/v2/bar",
			want: fooModuleVersions,
		},
		{
			name: "want_pseudo_versions_only",
			path: "golang.org/x/tools/blog",
			want: func() []*internal.ModuleInfo {
				versions := []*internal.ModuleInfo{}
				// Expect the 10 most recent in DESC order
				for i := 12; i > 2; i-- {
					versions = append(versions, &internal.ModuleInfo{
						ModulePath: pseudoModule,
						Version:    fmt.Sprintf("v0.0.0-202001011200%02d-000000000000", i),
					})
				}
				return versions
			}(),
		},
		{
			name: "want_tagged_versions_includes_incompatible",
			path: "path.to/incompatible/module",
			want: []*internal.ModuleInfo{
				{
					ModulePath: incompatibleModule,
					Version:    "v0.0.0",
				},
				{
					ModulePath: incompatibleModule,
					Version:    "v2.0.0+incompatible",
				},
			},
		},
		{
			name: "nested_module_path",
			path: "golang.org/foo/bar/api",
			want: []*internal.ModuleInfo{
				{
					ModulePath: nestedModule,
					Version:    "v1.0.4",
				},
				{
					ModulePath: nestedModule,
					Version:    "v1.0.3",
				},
				{
					ModulePath: rootModule,
					Version:    "v1.0.3",
				},
				{
					ModulePath: rootModule,
					Version:    "v0.11.6",
				},
			},
		},
		{
			name: "root_module_path",
			path: "golang.org/foo/bar",
			want: []*internal.ModuleInfo{
				{
					ModulePath: rootModule,
					Version:    "v1.0.3",
				},
				{
					ModulePath: rootModule,
					Version:    "v0.11.6",
				},
			},
		},
		{
			name: "want_zero_results_in_non_empty_db",
			path: "not.a/real/path",
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			var want []*internal.ModuleInfo
			for _, w := range test.want {
				mod := sample.ModuleInfo(w.ModulePath, w.Version)
				want = append(want, mod)
			}

			got, err := testDB.GetVersionsForPath(ctx, test.path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetVersionsForPath(%q) mismatch (-want +got):\n%s", test.path, diff)
			}
		})
	}
}

func TestGetLatestInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	for _, m := range []*internal.Module{
		sample.Module("a.com/M", "v1.1.1", "all", "most", "some", "one", "D/other"),
		sample.Module("a.com/M", "v1.2.0", "all", "most"),
		sample.Module("a.com/M/v2", "v2.0.5", "all", "most"),
		sample.Module("a.com/M/v3", "v3.0.1", "all", "some"),
		sample.Module("a.com/M/D", "v1.3.0", "other"),
		sample.Module("b.com/M/v9", "v9.0.0", ""),
		sample.Module("b.com/M/v10", "v10.0.0", ""),
	} {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		unit, module string
		want         internal.LatestInfo
	}{
		{
			// A unit that is the module.
			"a.com/M", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.2.0",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: true,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A unit that exists in all versions of the module.
			"a.com/M/all", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.2.0",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: true,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3/all",
			},
		},
		{
			// A unit that exists in most versions, but not the latest major.
			"a.com/M/most", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.2.0",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: true,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A unit that does not exist at the latest minor version, but does at the latest major.
			"a.com/M/some", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.1.1",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: false,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3/some",
			},
		},
		{
			// A unit that does not exist at the latest minor or major versions.
			"a.com/M/one", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.1.1",
				MinorModulePath:   "a.com/M",
				UnitExistsAtMinor: false,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A unit whose latest minor version is in a different module.
			"a.com/M/D/other", "a.com/M",

			internal.LatestInfo{
				MinorVersion:      "v1.3.0",
				MinorModulePath:   "a.com/M/D",
				UnitExistsAtMinor: false,
				MajorModulePath:   "a.com/M/v3",
				MajorUnitPath:     "a.com/M/v3",
			},
		},
		{
			// A module with v9 and v10 versions.
			"b.com/M/v9", "b.com/M/v9",
			internal.LatestInfo{
				MinorVersion:      "v9.0.0",
				MinorModulePath:   "b.com/M/v9",
				UnitExistsAtMinor: true,
				MajorModulePath:   "b.com/M/v10",
				MajorUnitPath:     "b.com/M/v10",
			},
		},
	} {
		t.Run(test.unit, func(t *testing.T) {
			got, err := testDB.GetLatestInfo(ctx, test.unit, test.module)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want, +got):\n%s", diff)
			}
		})
	}
}
