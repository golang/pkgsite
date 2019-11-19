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
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/testing/sample"
	"golang.org/x/discovery/internal/testing/testhelper"
	"golang.org/x/discovery/internal/version"
)

func setup(t *testing.T) (context.Context, *DataSource, func()) {
	t.Helper()
	contents := map[string]string{
		"LICENSE":    testhelper.MITLicense,
		"baz/baz.go": "//Package baz provides a helpful constant.\npackage baz\nimport \"net/http\"\nconst OK = http.StatusOK",
	}
	testVersions := []*proxy.TestVersion{
		proxy.NewTestVersion(t, "foo.com/bar", "v1.1.0", contents),
		proxy.NewTestVersion(t, "foo.com/bar", "v1.2.0", contents),
	}
	client, teardownProxy := proxy.SetupTestProxy(t, testVersions)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	return ctx, New(client), func() {
		teardownProxy()
		cancel()
	}
}

var (
	wantLicenseMD = sample.LicenseMetadata[0]
	wantLicense   = &license.License{Metadata: wantLicenseMD}
	wantPackage   = internal.Package{
		Path:     "foo.com/bar/baz",
		Name:     "baz",
		Imports:  []string{"net/http"},
		Synopsis: "Package baz provides a helpful constant.",
		V1Path:   "foo.com/bar/baz",
		Licenses: []*license.Metadata{wantLicenseMD},
		GOOS:     "linux",
		GOARCH:   "amd64",
	}
	wantVersionInfo = internal.VersionInfo{
		ModulePath:  "foo.com/bar",
		Version:     "v1.2.0",
		CommitTime:  time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
		VersionType: version.TypeRelease,
	}
	wantVersionedPackage = &internal.VersionedPackage{
		VersionInfo: wantVersionInfo,
		Package:     wantPackage,
	}
	cmpOpts = append([]cmp.Option{
		cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML"),
		cmpopts.IgnoreFields(license.License{}, "Contents"),
	}, sample.LicenseCmpOpts...)
)

func TestDataSource_GetDirectory(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	want := &internal.Directory{
		Path:        "foo.com/bar",
		VersionInfo: wantVersionInfo,
		Packages:    []*internal.Package{&wantPackage},
	}
	got, err := ds.GetDirectory(ctx, "foo.com/bar", internal.UnknownModulePath, "v1.2.0")
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

func TestDataSource_GetVersionInfo_Latest(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetVersionInfo(ctx, "foo.com/bar", internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&wantVersionInfo, got, cmpOpts...); diff != "" {
		t.Errorf("GetLatestVersionInfo diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetModuleLicenses(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetModuleLicenses(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []*license.License{wantLicense}
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
	want := []*license.License{wantLicense}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("GetPackageLicenses diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPackagesInVersion(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetPackagesInVersion(ctx, "foo.com/bar", "v1.2.0")
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
	v110 := wantVersionInfo
	v110.Version = "v1.1.0"
	want := []*internal.VersionInfo{&wantVersionInfo, &v110}
	ignore := cmpopts.IgnoreFields(internal.VersionInfo{}, "CommitTime", "VersionType")
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
	v110 := wantVersionInfo
	v110.Version = "v1.1.0"
	want := []*internal.VersionInfo{&wantVersionInfo, &v110}
	ignore := cmpopts.IgnoreFields(internal.VersionInfo{}, "CommitTime", "VersionType")
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("GetTaggedVersionsForPackageSeries diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetVersionInfo(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.GetVersionInfo(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&wantVersionInfo, got, cmpOpts...); diff != "" {
		t.Errorf("GetVersionInfo diff (-want +got):\n%s", diff)
	}
}
