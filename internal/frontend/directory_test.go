// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"strings"
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
			Directory: &Directory{
				Path:     dirPath,
				Version:  v.Version,
				Packages: pkgs,
			},
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
	checkFetchDirectory(internal.LatestVersion, pagev111)
}

func TestFetchPackageDirectoryDetailsAndFetchModuleDirectoryDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	createPackageForVersion := func(path string) *internal.Package {
		pkg := sample.Package()
		pkg.Path = path
		return pkg
	}

	v := sample.Version()
	pkg1 := createPackageForVersion(v.ModulePath)
	pkg2 := createPackageForVersion(v.ModulePath + "/a")
	pkg3 := createPackageForVersion(v.ModulePath + "/a/b")
	v.Packages = []*internal.Package{pkg1, pkg2, pkg3}

	checkDirectoryDetails := func(fnName string, dirPath string, got *DirectoryDetails, wantPackages []*internal.Package) {
		t.Helper()

		var wantPkgs []*Package
		for _, p := range wantPackages {
			pkg, err := createPackage(p, &v.VersionInfo)
			if err != nil {
				t.Fatal(err)
			}
			pkg.Suffix = strings.TrimPrefix(strings.TrimPrefix(pkg.Path, dirPath), "/")
			if pkg.Suffix == "" {
				pkg.Suffix = fmt.Sprintf("%s (root)", effectiveName(p))
			}
			wantPkgs = append(wantPkgs, pkg)
		}

		want := &DirectoryDetails{
			ModulePath: v.ModulePath,
			Directory: &Directory{
				Path:     dirPath,
				Version:  v.Version,
				Packages: wantPkgs,
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s(ctx, %q, %q) mismatch (-want +got):\n%s", fnName, pkg1.Path, v.Version, diff)
		}
	}

	if err := testDB.InsertVersion(ctx, v); err != nil {
		t.Fatal(err)
	}

	got, err := fetchPackageDirectoryDetails(ctx, testDB, pkg2.Path, &v.VersionInfo)
	if err != nil {
		t.Fatalf("fetchPackageDirectoryDetails(ctx, db, %q, %q): %v", pkg2.Path, v.Version, err)
	}
	checkDirectoryDetails("fetchPackageDirectoryDetails", pkg2.Path, got, []*internal.Package{pkg3})

	got, err = fetchModuleDirectoryDetails(ctx, testDB, &v.VersionInfo)
	if err != nil {
		t.Fatalf("fetchModuleDirectoryDetails(ctx, db, %q, %q): %v", v.ModulePath, v.Version, err)
	}
	checkDirectoryDetails("fetchModuleDirectoryDetails", v.ModulePath, got, []*internal.Package{pkg1, pkg2, pkg3})
}
