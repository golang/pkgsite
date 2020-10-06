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
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestPageInfo(t *testing.T) {
	m := sample.LegacyModule("golang.org/x/tools", "v1.0.0", "go/packages", "cmd/godoc")

	type test struct {
		name      string
		unit      *internal.Unit
		wantTitle string
		wantType  string
	}
	var tests []*test
	for _, u := range m.Units {
		switch u.Path {
		case "golang.org/x/tools":
			tests = append(tests, &test{"module golang.org/x/tools", u, "tools", pageTypeModule})
		case "golang.org/x/tools/go/packages":
			tests = append(tests, &test{"package golang.org/x/tools/go/packages", u, "packages", pageTypePackage})
		case "golang.org/x/tools/go":
			tests = append(tests, &test{"directory golang.org/x/tools/go", u, "go/", pageTypeDirectory})
		case "golang.org/x/tools/cmd/godoc":
			u.Name = "main"
			tests = append(tests, &test{"package golang.org/x/tools/cmd/godoc", u, "godoc", pageTypeCommand})
		case "golang.org/x/tools/cmd":
			tests = append(tests, &test{"directory golang.org/x/tools/cmd", u, "cmd/", pageTypeDirectory})
		default:
			t.Fatalf("Unexpected path: %q", u.Path)
		}
	}
	std := sample.LegacyModule(stdlib.ModulePath, "v1.0.0", "")
	tests = append(tests, &test{"module std", std.Units[0], "Standard library", pageTypeStdLib})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotTitle, gotType := pageInfo(test.unit)
			if gotTitle != test.wantTitle || gotType != test.wantType {
				t.Errorf("pageInfo(%q): %q, %q; want = %q, %q", test.unit.Path, gotTitle, gotType, test.wantTitle, test.wantType)
			}
		})
	}
}

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
