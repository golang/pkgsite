// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestReadAndWriteVersionMap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	m := sample.Module("golang.org/x/tools", sample.VersionString, "go/packages")
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
	if err := testDB.UpsertVersionMap(ctx, vm); err != nil {
		t.Fatal(err)
	}

	got, err := testDB.GetVersionMap(ctx, vm.ModulePath, vm.RequestedVersion)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(vm, got, cmpopts.IgnoreFields(internal.VersionMap{}, "UpdatedAt")); diff != "" {
		t.Fatalf("t.Errorf(ctx, %q, %q) mismatch (-want +got):\n%s", vm.ModulePath, vm.RequestedVersion, diff)
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
		got, err := testDB.GetVersionMap(ctx, vm.ModulePath, vm.RequestedVersion)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(vm, got, cmpopts.IgnoreFields(internal.VersionMap{}, "UpdatedAt")); diff != "" {
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
