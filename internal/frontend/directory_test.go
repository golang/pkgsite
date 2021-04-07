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
		postgres.MustInsertModuleLatest(ctx, t, testDB, m)
	}

	for _, test := range []struct {
		modulePath     string
		subdirectories []*DirectoryInfo
		want           []*DirectoryInfo
	}{
		{
			modulePath: "cloud.google.com/go",
			want: []*DirectoryInfo{
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
			want: []*DirectoryInfo{
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
			subdirectories: []*DirectoryInfo{
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
			subdirectories: []*DirectoryInfo{
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
			for _, w := range test.want {
				w.IsModule = true
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnitDirectories(t *testing.T) {
	subdirectories := []*DirectoryInfo{
		{Suffix: "accessapproval"},
		{Suffix: "accessapproval/internal"},
		{Suffix: "accessapproval/cgi"},
		{Suffix: "accessapproval/cookiejar"},
		{Suffix: "accessapproval/cookiejar/internal"},
		{Suffix: "fgci"},
		{Suffix: "httptrace"},
		{Suffix: "internal/bytesconv"},
		{Suffix: "internal/json"},
		{Suffix: "zoltan"},
	}
	nestedModules := []*DirectoryInfo{
		{Suffix: "httptest", IsModule: true},
		{Suffix: "pubsub/internal", IsModule: true},
	}
	got := unitDirectories(append(subdirectories, nestedModules...))
	want := &Directories{
		External: []*Directory{
			{
				Prefix: "accessapproval",
				Root:   &DirectoryInfo{Suffix: "accessapproval"},
				Subdirectories: []*DirectoryInfo{
					{Suffix: "cgi"},
					{Suffix: "cookiejar"},
				},
			},
			{
				Prefix: "fgci",
				Root:   &DirectoryInfo{Suffix: "fgci"},
			},
			{
				Prefix: "httptest",
				Root:   &DirectoryInfo{Suffix: "httptest", IsModule: true},
			},
			{
				Prefix: "httptrace",
				Root:   &DirectoryInfo{Suffix: "httptrace"},
			},
			{
				Prefix: "zoltan",
				Root:   &DirectoryInfo{Suffix: "zoltan"},
			},
		},
		Internal: &Directory{
			Prefix: "internal",
			Subdirectories: []*DirectoryInfo{
				{Suffix: "bytesconv"},
				{Suffix: "json"},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unitDirectories mismatch (-want +got):\n%s", diff)
	}
}
