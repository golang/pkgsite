// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/sample"
)

func TestFetchPackagesInDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	defer postgres.ResetTestDB(testDB, t)

	modulePath := "github.com/foo"
	dirPath := modulePath + "/bar"
	mustInsertVersionAndGetDirectoryPage := func(version string) *DirectoryPage {
		t.Helper()
		v := sample.Version()
		v.ModulePath = modulePath
		v.Version = version
		v.Packages = []*internal.Package{
			{Name: "A", Path: fmt.Sprintf("%s/%s", dirPath, "A"), Licenses: sample.LicenseMetadata},
			{Name: "B", Path: fmt.Sprintf("%s/%s", dirPath, "B"), Licenses: sample.LicenseMetadata},
		}
		if err := testDB.InsertVersion(ctx, v); err != nil {
			t.Fatal(err)
		}

		var pkgs []*Package
		for _, p := range v.Packages {
			pkg, err := createPackage(p, &v.VersionInfo)
			if err != nil {
				t.Fatal(err)
			}
			pkg.Suffix = p.Name
			pkgs = append(pkgs, pkg)
		}
		return &DirectoryPage{
			Directory:  dirPath,
			ModulePath: modulePath,
			Version:    v.Version,
			Packages:   pkgs,
		}
	}
	checkFetchDirectory := func(version string, want *DirectoryPage) {
		t.Helper()
		got, err := fetchPackagesInDirectory(ctx, testDB, dirPath, version)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(DirectoryPage{})); diff != "" {
			t.Errorf("fetchPackagesInDirectory(%q, %q) mismatch (-want +got):\n%s", dirPath, version, diff)
		}
	}

	pagev110 := mustInsertVersionAndGetDirectoryPage("v1.0.0")
	pagev111 := mustInsertVersionAndGetDirectoryPage("v1.1.0")

	checkFetchDirectory("v1.0.0", pagev110)
	checkFetchDirectory("", pagev111)
}
