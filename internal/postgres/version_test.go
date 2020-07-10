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
