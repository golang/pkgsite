// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxydatasource

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
	"golang.org/x/pkgsite/internal/version"
)

func setup(t *testing.T) (context.Context, *DataSource, func()) {
	t.Helper()
	contents := map[string]string{
		"go.mod":     "module foo.com/bar",
		"LICENSE":    testhelper.MITLicense,
		"baz/baz.go": "//Package baz provides a helpful constant.\npackage baz\nimport \"net/http\"\nconst OK = http.StatusOK",
	}
	testModules := []*proxy.TestModule{
		{
			ModulePath: "foo.com/bar",
			Version:    "v1.1.0",
			Files:      contents,
		},
		{
			ModulePath: "foo.com/bar",
			Version:    "v1.2.0",
			Files:      contents,
		},
	}
	client, teardownProxy := proxy.SetupTestProxy(t, testModules)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	return ctx, New(client), func() {
		teardownProxy()
		cancel()
	}
}

var (
	wantLicenseMD = sample.LicenseMetadata[0]
	wantLicense   = &licenses.License{Metadata: wantLicenseMD}
	wantPackage   = internal.Package{
		Path:              "foo.com/bar/baz",
		Name:              "baz",
		Imports:           []string{"net/http"},
		Synopsis:          "Package baz provides a helpful constant.",
		V1Path:            "foo.com/bar/baz",
		Licenses:          []*licenses.Metadata{wantLicenseMD},
		IsRedistributable: true,
		GOOS:              "linux",
		GOARCH:            "amd64",
	}
	wantModuleInfo = internal.ModuleInfo{
		ModulePath:        "foo.com/bar",
		Version:           "v1.2.0",
		CommitTime:        time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
		VersionType:       version.TypeRelease,
		IsRedistributable: true,
		HasGoMod:          true,
	}
	wantVersionedPackage = &internal.VersionedPackage{
		ModuleInfo: wantModuleInfo,
		Package:    wantPackage,
	}
	cmpOpts = append([]cmp.Option{
		cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML"),
		cmpopts.IgnoreFields(licenses.License{}, "Contents"),
	}, sample.LicenseCmpOpts...)
)

func TestDataSource_GetDirectory(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	want := &internal.Directory{
		Path:       "foo.com/bar",
		ModuleInfo: wantModuleInfo,
		Packages:   []*internal.Package{&wantPackage},
	}
	got, err := ds.GetDirectory(ctx, "foo.com/bar", internal.UnknownModulePath, "v1.2.0", internal.AllFields)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("GetDirectory diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetImports(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	want := []string{"net/http"}
	got, err := ds.GetImports(ctx, "foo.com/bar/baz", "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("GetImports diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPackage_Latest(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetPackage(ctx, "foo.com/bar/baz", internal.UnknownModulePath, internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantVersionedPackage, got, cmpOpts...); diff != "" {
		t.Errorf("GetLatestPackage diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetModuleInfo_Latest(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetModuleInfo(ctx, "foo.com/bar", internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&wantModuleInfo, got, cmpOpts...); diff != "" {
		t.Errorf("GetLatestModuleInfo diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetModuleLicenses(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetModuleLicenses(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []*licenses.License{wantLicense}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("GetModuleLicenses diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPackage(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetPackage(ctx, "foo.com/bar/baz", internal.UnknownModulePath, "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantVersionedPackage, got, cmpOpts...); diff != "" {
		t.Errorf("GetPackage diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPackageLicenses(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetPackageLicenses(ctx, "foo.com/bar/baz", "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []*licenses.License{wantLicense}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("GetPackageLicenses diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPackagesInVersion(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetPackagesInModule(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []*internal.Package{&wantPackage}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("GetPackagesInVersion diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetTaggedVersionsForModule(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetTaggedVersionsForModule(ctx, "foo.com/bar")
	if err != nil {
		t.Fatal(err)
	}
	v110 := wantModuleInfo
	v110.Version = "v1.1.0"
	want := []*internal.ModuleInfo{&wantModuleInfo, &v110}
	ignore := cmpopts.IgnoreFields(internal.ModuleInfo{}, "CommitTime", "VersionType", "IsRedistributable", "HasGoMod")
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("GetTaggedVersionsForPackageSeries diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetTaggedVersionsForPackageSeries(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	// // TODO (rFindley): this shouldn't be necessary.
	// _, err := ds.GetLatestPackage(ctx, "foo.com/bar/baz")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	got, err := ds.GetTaggedVersionsForPackageSeries(ctx, "foo.com/bar/baz")
	if err != nil {
		t.Fatal(err)
	}
	v110 := wantModuleInfo
	v110.Version = "v1.1.0"
	want := []*internal.ModuleInfo{&wantModuleInfo, &v110}
	ignore := cmpopts.IgnoreFields(internal.ModuleInfo{}, "CommitTime", "VersionType", "IsRedistributable", "HasGoMod")
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("GetTaggedVersionsForPackageSeries diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetModuleInfo(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetModuleInfo(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&wantModuleInfo, got, cmpOpts...); diff != "" {
		t.Errorf("GetModuleInfo diff (-want +got):\n%s", diff)
	}
}
