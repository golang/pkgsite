// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
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

func TestGetVersionMapsWithNon2xxStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer ResetTestDB(testDB, t)

	tests := []struct {
		path   string
		status int
	}{
		{"github.com/a/b", 200},
		{"github.com/a/c", 290},
		{"github.com/a/d", 400},
		{"github.com/a/e", 440},
		{"github.com/a/f", 490},
		{"github.com/a/g", 491},
		{"github.com/a/h", 500},
	}
	var paths []string
	want := map[string]bool{}
	for _, test := range tests {
		paths = append(paths, test.path)
		want[test.path] = true
		if err := testDB.UpsertVersionMap(ctx, &internal.VersionMap{
			ModulePath:       test.path,
			RequestedVersion: internal.LatestVersion,
			ResolvedVersion:  sample.VersionString,
			GoModPath:        test.path,
			Status:           test.status,
		}); err != nil {
			t.Fatal(err)
		}
	}
	vms, err := testDB.GetVersionMaps(ctx, paths, internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, vm := range vms {
		got[vm.ModulePath] = true
	}
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Fatalf("got = \n%v\nwant =\n%v", got, want)
	}
}
