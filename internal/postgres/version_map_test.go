// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestReadAndWriteVersionMap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	m := sample.DefaultModule()
	m.ModulePath = "golang.org/x/tools"
	p := sample.DefaultPackage()
	p.Path = "golang.org/x/tools/go/packages"
	m.Packages = []*internal.Package{p}
	err := testDB.InsertModule(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	vm := &internal.VersionMap{
		ModulePath:       m.ModulePath,
		RequestedVersion: "master",
		ResolvedVersion:  "v1.0.0",
		Status:           200,
	}
	err = testDB.UpsertVersionMap(ctx, vm)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, path, modulePath, version string
	}{
		{
			name:       "package path - latest version, unknown module",
			path:       "golang.org/x/tools/go/packages",
			modulePath: internal.UnknownModulePath,
			version:    internal.LatestVersion,
		},
		{
			name:       "package path - latest version, known module",
			path:       "golang.org/x/tools/go/packages",
			modulePath: "golang.org/x/tools",
			version:    internal.LatestVersion,
		},
		{
			name:       "package path - specified version, unknown module",
			path:       "golang.org/x/tools/go/packages",
			modulePath: internal.UnknownModulePath,
			version:    "master",
		},
		{
			name:       "package path - specified version, known module",
			path:       "golang.org/x/tools/go/packages",
			modulePath: "golang.org/x/tools",
			version:    "master",
		},
		{
			name:       "directory path - latest version, unknown module",
			path:       "golang.org/x/tools/go",
			modulePath: internal.UnknownModulePath,
			version:    internal.LatestVersion,
		},
		{
			name:       "directory path - latest version, known module",
			path:       "golang.org/x/tools/go",
			modulePath: "golang.org/x/tools",
			version:    internal.LatestVersion,
		},
		{
			name:       "directory path - specified version, unknown module",
			path:       "golang.org/x/tools/go",
			modulePath: internal.UnknownModulePath,
			version:    "master",
		},
		{
			name:       "directory path - specified version, known module",
			path:       "golang.org/x/tools/go",
			modulePath: "golang.org/x/tools",
			version:    "master",
		},
		{
			name:       "module path - latest version, unknown module",
			path:       "golang.org/x/tools",
			modulePath: internal.UnknownModulePath,
			version:    internal.LatestVersion,
		},
		{
			name:       "module path - latest version, known module",
			path:       "golang.org/x/tools",
			modulePath: "golang.org/x/tools",
			version:    internal.LatestVersion,
		},
		{
			name:       "module path - specified version, unknown module",
			path:       "golang.org/x/tools",
			modulePath: internal.UnknownModulePath,
			version:    "master",
		},
		{
			name:       "module path - specified version, known module",
			path:       "golang.org/x/tools",
			modulePath: "golang.org/x/tools",
			version:    "master",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := testDB.GetVersionMap(ctx, test.path, test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(vm, got); diff != "" {
				t.Errorf("t.Errorf(ctx, %q, %q, %q) mismatch (-want +got):\n%s",
					test.path, test.modulePath, test.version, diff)
			}
		})
	}
}
func TestUpsertVersionMap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	upsertAndVerifyVersionMap := func(vm *internal.VersionMap) {
		err := testDB.UpsertVersionMap(ctx, vm)
		if err != nil {
			t.Fatal(err)
		}
		got, err := testDB.GetVersionMap(ctx, vm.ModulePath, vm.ModulePath, vm.RequestedVersion)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(vm, got); diff != "" {
			t.Fatalf("t.Errorf(ctx, %q, %q) mismatch (-want +got):\n%s",
				vm.ModulePath, vm.RequestedVersion, diff)
		}
	}

	vm := &internal.VersionMap{
		ModulePath:       "github.com/module",
		RequestedVersion: "master",
		ResolvedVersion:  "",
		Status:           404,
		Error:            "not found",
	}
	upsertAndVerifyVersionMap(vm)

	vm.ResolvedVersion = "v1.0.0"
	vm.Status = 200
	upsertAndVerifyVersionMap(vm)
}
