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

func TestPostgres_GetTaggedAndPseudoVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		modulePath1 = "path.to/foo"
		modulePath2 = "path.to/foo/v2"
		modulePath3 = "path.to/some/thing"
		testModules = []*internal.Module{
			sample.Module(modulePath3, "v3.0.0", "else"),
			sample.Module(modulePath1, "v2.0.0+incompatible", "bar"),
			sample.Module(modulePath1, "v1.0.0-alpha.1", "bar"),
			sample.Module(modulePath1, "v1.0.0", "bar"),
			sample.Module(modulePath2, "v2.0.1-beta", "bar"),
			sample.Module(modulePath2, "v2.1.0", "bar"),
		}
	)

	testCases := []struct {
		name, path, modulePath string
		numPseudo              int
		modules                []*internal.Module
		wantTaggedVersions     []*internal.ModuleInfo
	}{
		{
			name:       "want_releases_and_prereleases_only",
			path:       "path.to/foo/bar",
			modulePath: modulePath1,
			numPseudo:  12,
			modules:    testModules,
			wantTaggedVersions: []*internal.ModuleInfo{
				{
					ModulePath: modulePath2,
					Version:    "v2.1.0",
					CommitTime: sample.CommitTime,
				},
				{
					ModulePath: modulePath2,
					Version:    "v2.0.1-beta",
					CommitTime: sample.CommitTime,
				},
				{
					ModulePath: modulePath1,
					Version:    "v1.0.0",
					CommitTime: sample.CommitTime,
				},
				{
					ModulePath: modulePath1,
					Version:    "v1.0.0-alpha.1",
					CommitTime: sample.CommitTime,
				},
				{
					ModulePath: modulePath1,
					Version:    "v2.0.0+incompatible",
					CommitTime: sample.CommitTime,
				},
			},
		},
		{
			name:       "want_zero_results_in_non_empty_db",
			path:       "not.a/real/path",
			modulePath: "not.a/real/path",
			modules:    testModules,
		},
		{
			name:       "want_zero_results_in_empty_db",
			path:       "not.a/real/path",
			modulePath: "not.a/real/path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ResetTestDB(testDB, t)

			var wantPseudoVersions []*internal.ModuleInfo
			for i := 0; i < tc.numPseudo; i++ {

				pseudo := fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", i+1)
				m := sample.Module(modulePath1, pseudo, "bar")
				if err := testDB.InsertModule(ctx, m); err != nil {
					t.Fatal(err)
				}

				// LegacyGetPsuedoVersions should only return the 10 most recent pseudo versions,
				// if there are more than 10 in the database
				if i < 10 {
					wantPseudoVersions = append(wantPseudoVersions, &internal.ModuleInfo{
						ModulePath: modulePath1,
						Version:    fmt.Sprintf("v0.0.0-201806111833%02d-d8887717615a", tc.numPseudo-i),
						CommitTime: sample.CommitTime,
					})
				}
			}

			for _, m := range tc.modules {
				if err := testDB.InsertModule(ctx, m); err != nil {
					t.Fatal(err)
				}
			}

			got, err := testDB.LegacyGetPsuedoVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantPseudoVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.LegacyGetPsuedoVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}

			got, err = testDB.LegacyGetTaggedVersionsForPackageSeries(ctx, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantTaggedVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.LegacyGetTaggedVersionsForPackageSeries(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}

			got, err = testDB.LegacyGetPsuedoVersionsForModule(ctx, tc.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantPseudoVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.LegacyGetPsuedoVersionsForModule(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}

			got, err = testDB.LegacyGetTaggedVersionsForModule(ctx, tc.modulePath)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantTaggedVersions, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.LegacyGetTaggedVersionsForModule(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}

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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var want []*internal.ModuleInfo
			for _, w := range tc.want {
				mod := sample.ModuleInfo(w.ModulePath, w.Version)
				want = append(want, mod)
			}

			got, err := testDB.GetVersionsForPath(ctx, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got, cmp.AllowUnexported(source.Info{})); diff != "" {
				t.Errorf("testDB.GetVersionsForPath(%q) mismatch (-want +got):\n%s", tc.path, diff)
			}
		})
	}
}
