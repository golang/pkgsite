// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetNestedModules(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	for _, m := range []*internal.Module{
		sample.LegacyModule("cloud.google.com/go", "v0.46.2", "storage", "spanner", "pubsub"),
		sample.LegacyModule("cloud.google.com/go/pubsub", "v1.6.1", sample.Suffix),
		sample.LegacyModule("cloud.google.com/go/spanner", "v1.9.0", sample.Suffix),
		sample.LegacyModule("cloud.google.com/go/storage", "v1.10.0", sample.Suffix),
		sample.LegacyModule("cloud.google.com/go/storage/v11", "v11.0.0", sample.Suffix),
		sample.LegacyModule("cloud.google.com/go/storage/v9", "v9.0.0", sample.Suffix),
		sample.LegacyModule("cloud.google.com/go/storage/module", "v1.10.0", sample.Suffix),
		sample.LegacyModule("cloud.google.com/go/v2", "v2.0.0", "storage", "spanner", "pubsub"),
	} {
		if err := testDB.InsertModule(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		modulePath string
		want       []*NestedModule
	}{
		{
			modulePath: "cloud.google.com/go",
			want: []*NestedModule{
				{
					Suffix: "pubsub",
					URL:    "/cloud.google.com/go/pubsub",
				},
				{
					Suffix: "spanner",
					URL:    "/cloud.google.com/go/spanner",
				},
				{
					Suffix: "storage",
					URL:    "/cloud.google.com/go/storage/v11",
				},
				{
					Suffix: "storage/module",
					URL:    "/cloud.google.com/go/storage/module",
				},
			},
		},
		{
			modulePath: "cloud.google.com/go/spanner",
		},
		{
			modulePath: "cloud.google.com/go/storage",
			want: []*NestedModule{
				{
					Suffix: "module",
					URL:    "/cloud.google.com/go/storage/module",
				},
			},
		},
	} {
		t.Run(tc.modulePath, func(t *testing.T) {
			got, err := getNestedModules(ctx, testDB, &internal.UnitMeta{
				Path:       tc.modulePath,
				ModulePath: tc.modulePath,
			})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
