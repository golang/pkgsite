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
		sample.Module("cloud.google.com/go", "v0.46.2", "storage", "spanner", "pubsub"),
		sample.Module("cloud.google.com/go/pubsub", "v1.6.1", sample.Suffix),
		sample.Module("cloud.google.com/go/spanner", "v1.9.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage", "v1.10.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/v11", "v11.0.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/v9", "v9.0.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/module", "v1.10.0", sample.Suffix),
		sample.Module("cloud.google.com/go/storage/v9/module", "v9.0.0", sample.Suffix),
		sample.Module("cloud.google.com/go/v2", "v2.0.0", "storage", "spanner", "pubsub"),
	} {
		postgres.MustInsertModule(ctx, t, testDB, m)
	}

	for _, test := range []struct {
		modulePath     string
		subdirectories []*Subdirectory
		want           []*NestedModule
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
				{
					Suffix: "storage/v9/module",
					URL:    "/cloud.google.com/go/storage/v9/module",
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
				{
					Suffix: "v9/module",
					URL:    "/cloud.google.com/go/storage/v9/module",
				},
			},
		},
		{
			modulePath: "cloud.google.com/go/storage",
			subdirectories: []*Subdirectory{
				{
					Suffix: "module",
					URL:    "/cloud.google.com/go/storage/module",
				},
				{
					Suffix: "v9/module",
					URL:    "/cloud.google.com/go/storage/v9/module",
				},
			},
		},
		{
			modulePath: "cloud.google.com/go/storage/v9",
			subdirectories: []*Subdirectory{
				{
					Suffix: "module",
					URL:    "/cloud.google.com/go/storage/v9/module",
				},
			},
		},
	} {
		t.Run(test.modulePath, func(t *testing.T) {
			got, err := getNestedModules(ctx, testDB, &internal.UnitMeta{
				Path:       test.modulePath,
				ModuleInfo: internal.ModuleInfo{ModulePath: test.modulePath},
			}, test.subdirectories)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnitDirectories(t *testing.T) {
	subdirectories := []*Subdirectory{
		{Suffix: "accessapproval"},
		{Suffix: "accessapproval/cgi"},
		{Suffix: "accessapproval/cookiejar"},
		{Suffix: "fgci"},
		{Suffix: "httptrace"},
		{Suffix: "internal/bytesconv"},
		{Suffix: "internal/json"},
		{Suffix: "zoltan"},
	}
	nestedModules := []*NestedModule{
		{Suffix: "httptest"},
	}

	got := unitDirectories(subdirectories, nestedModules)
	want := []*UnitDirectory{
		{
			Prefix: "accessapproval",
			Root:   &Subdirectory{Suffix: "accessapproval"},
			Subdirectories: []*Subdirectory{
				{Suffix: "cgi"},
				{Suffix: "cookiejar"},
			},
		},
		{
			Prefix: "fgci",
			Root:   &Subdirectory{Suffix: "fgci"},
		},
		{
			Prefix: "httptest",
			Root:   &Subdirectory{Suffix: "httptest", IsModule: true},
		},
		{
			Prefix: "httptrace",
			Root:   &Subdirectory{Suffix: "httptrace"},
		},
		{
			Prefix: "internal",
			Subdirectories: []*Subdirectory{
				{Suffix: "bytesconv"},
				{Suffix: "json"},
			},
		},
		{
			Prefix: "zoltan",
			Root:   &Subdirectory{Suffix: "zoltan"},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unitDirectories mismatch (-want +got):\n%s", diff)
	}
}
