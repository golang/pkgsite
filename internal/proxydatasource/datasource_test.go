// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxydatasource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/licensecheck"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
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
	// nrContents is the same as contents, except the license is non-redistributable.
	nrContents := map[string]string{
		"go.mod":     "module foo.com/nr",
		"LICENSE":    "unknown",
		"baz/baz.go": "//Package baz provides a helpful constant.\npackage baz\nimport \"net/http\"\nconst OK = http.StatusOK",
	}

	testModules := []*proxy.Module{
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
		{
			ModulePath: "foo.com/nr",
			Version:    "v1.1.0",
			Files:      nrContents,
		},
	}
	client, teardownProxy := proxy.SetupTestClient(t, testModules)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	return ctx, New(client), func() {
		teardownProxy()
		cancel()
	}
}

var (
	wantLicenseMD  = sample.LicenseMetadata[0]
	wantLicense    = &licenses.License{Metadata: wantLicenseMD}
	wantLicenseMIT = &licenses.License{
		Metadata: &licenses.Metadata{
			Types:    []string{"MIT"},
			FilePath: "LICENSE",
			Coverage: licensecheck.Coverage{
				Percent: 100,
				Match:   []licensecheck.Match{{Name: "MIT", Type: licensecheck.MIT, Percent: 100, End: 1049}},
			},
		},
		Contents: []byte(testhelper.MITLicense),
	}
	wantLicenseBSD = &licenses.License{
		Metadata: &licenses.Metadata{
			Types:    []string{"BSD-0-Clause"},
			FilePath: "qux/LICENSE",
			Coverage: licensecheck.Coverage{
				Percent: 100,
				Match:   []licensecheck.Match{{Name: "BSD-0-Clause", Type: licensecheck.BSD, Percent: 100, End: 633}},
			},
		},
		Contents: []byte(testhelper.BSD0License),
	}
	wantPackage = internal.LegacyPackage{
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
	wantVersionedPackage = &internal.LegacyVersionedPackage{
		LegacyModuleInfo: internal.LegacyModuleInfo{
			ModuleInfo: wantModuleInfo,
		},
		LegacyPackage: wantPackage,
	}
	cmpOpts = append([]cmp.Option{
		cmpopts.IgnoreFields(internal.LegacyPackage{}, "DocumentationHTML"),
		cmpopts.IgnoreFields(licenses.License{}, "Contents"),
	}, sample.LicenseCmpOpts...)
)

func TestDataSource_LegacyGetDirectory(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	want := &internal.LegacyDirectory{
		LegacyModuleInfo: internal.LegacyModuleInfo{ModuleInfo: wantModuleInfo},
		Path:             "foo.com/bar",
		Packages:         []*internal.LegacyPackage{&wantPackage},
	}
	got, err := ds.LegacyGetDirectory(ctx, "foo.com/bar", internal.UnknownModulePath, "v1.2.0", internal.AllFields)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("LegacyGetDirectory diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_LegacyGetImports(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	want := []string{"net/http"}
	got, err := ds.LegacyGetImports(ctx, "foo.com/bar/baz", "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("LegacyGetImports diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPackage_Latest(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetPackage(ctx, "foo.com/bar/baz", internal.UnknownModulePath, internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantVersionedPackage, got, cmpOpts...); diff != "" {
		t.Errorf("GetLatestPackage diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_LegacyGetModuleInfo_Latest(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetModuleInfo(ctx, "foo.com/bar", internal.LatestVersion)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantModuleInfo, got.ModuleInfo, cmpOpts...); diff != "" {
		t.Errorf("GetLatestModuleInfo diff (-want +got):\n%s", diff)
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

func TestDataSource_GetLicenses(t *testing.T) {
	t.Helper()
	testModules := []*proxy.Module{
		{
			ModulePath: "foo.com/bar",
			Version:    "v1.1.0",
			Files: map[string]string{
				"go.mod":  "module foo.com/bar",
				"LICENSE": testhelper.MITLicense,
				"bar.go":  "//Package bar provides a helpful constant.\npackage bar\nimport \"net/http\"\nconst OK = http.StatusOK",

				"baz/baz.go": "//Package baz provides a helpful constant.\npackage baz\nimport \"net/http\"\nconst OK = http.StatusOK",

				"qux/LICENSE": testhelper.BSD0License,
				"qux/qux.go":  "//Package qux provides a helpful constant.\npackage qux\nimport \"net/http\"\nconst OK = http.StatusOK",
			},
		},
	}
	client, teardownProxy := proxy.SetupTestClient(t, testModules)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	ds := New(client)
	teardown := func() {
		teardownProxy()
		cancel()
	}
	defer teardown()

	tests := []struct {
		err        error
		name       string
		fullPath   string
		modulePath string
		want       []*licenses.License
	}{
		{name: "no license dir", fullPath: "foo.com", modulePath: "foo.com/bar", err: derrors.NotFound},
		{name: "invalid dir", fullPath: "foo.com/invalid", modulePath: "foo.com/invalid", err: derrors.NotFound},
		{name: "root dir", fullPath: "foo.com/bar", modulePath: "foo.com/bar", want: []*licenses.License{wantLicenseMIT}},
		{name: "package with no extra license", fullPath: "foo.com/bar/baz", modulePath: "foo.com/bar", want: []*licenses.License{wantLicenseMIT}},
		{name: "package with additional license", fullPath: "foo.com/bar/qux", modulePath: "foo.com/bar", want: []*licenses.License{wantLicenseMIT, wantLicenseBSD}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ds.GetLicenses(ctx, test.fullPath, test.modulePath, "v1.1.0")
			if !errors.Is(err, test.err) {
				t.Fatal(err)
			}

			sort.Slice(got, func(i, j int) bool {
				return got[i].FilePath < got[j].FilePath
			})
			sort.Slice(test.want, func(i, j int) bool {
				return test.want[i].FilePath < test.want[j].FilePath
			})

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("GetLicenses diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDataSource_LegacyGetModuleLicenses(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetModuleLicenses(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []*licenses.License{wantLicense}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("LegacyGetModuleLicenses diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_LegacyGetPackage(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetPackage(ctx, "foo.com/bar/baz", internal.UnknownModulePath, "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantVersionedPackage, got, cmpOpts...); diff != "" {
		t.Errorf("GetPackage diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_LegacyGetPackageLicenses(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetPackageLicenses(ctx, "foo.com/bar/baz", "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []*licenses.License{wantLicense}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("LegacyGetPackageLicenses diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPackagesInVersion(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetPackagesInModule(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []*internal.LegacyPackage{&wantPackage}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("GetPackagesInVersion diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_LegacyGetTaggedVersionsForModule(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetTaggedVersionsForModule(ctx, "foo.com/bar")
	if err != nil {
		t.Fatal(err)
	}
	v110 := wantModuleInfo
	v110.Version = "v1.1.0"
	want := []*internal.ModuleInfo{
		&wantModuleInfo,
		&v110,
	}
	ignore := cmpopts.IgnoreFields(internal.ModuleInfo{}, "CommitTime", "VersionType", "IsRedistributable", "HasGoMod")
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("LegacyGetTaggedVersionsForPackageSeries diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_LegacyGetTaggedVersionsForPackageSeries(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	// // TODO (rFindley): this shouldn't be necessary.
	// _, err := ds.GetLatestPackage(ctx, "foo.com/bar/baz")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	got, err := ds.LegacyGetTaggedVersionsForPackageSeries(ctx, "foo.com/bar/baz")
	if err != nil {
		t.Fatal(err)
	}
	v110 := wantModuleInfo
	v110.Version = "v1.1.0"
	want := []*internal.ModuleInfo{
		&wantModuleInfo,
		&v110,
	}
	ignore := cmpopts.IgnoreFields(internal.ModuleInfo{}, "CommitTime", "VersionType", "IsRedistributable", "HasGoMod")
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("LegacyGetTaggedVersionsForPackageSeries diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_LegacyGetModuleInfo(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()
	got, err := ds.LegacyGetModuleInfo(ctx, "foo.com/bar", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantModuleInfo, got.ModuleInfo, cmpOpts...); diff != "" {
		t.Errorf("LegacyGetModuleInfo diff (-want +got):\n%s", diff)
	}
}

func TestDataSource_GetPathInfo(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()

	for _, test := range []struct {
		path, modulePath, version string
		want                      *internal.PathInfo
	}{
		{
			path:       "foo.com/bar",
			modulePath: "foo.com/bar",
			version:    "v1.1.0",
			want: &internal.PathInfo{
				ModulePath:        "foo.com/bar",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			path:       "foo.com/bar/baz",
			modulePath: "foo.com/bar",
			version:    "v1.1.0",
			want: &internal.PathInfo{
				ModulePath:        "foo.com/bar",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			path:       "foo.com/bar/baz",
			modulePath: internal.UnknownModulePath,
			version:    "v1.1.0",
			want: &internal.PathInfo{
				ModulePath:        "foo.com/bar",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			path:       "foo.com/bar/baz",
			modulePath: internal.UnknownModulePath,
			version:    internal.LatestVersion,
			want: &internal.PathInfo{
				ModulePath:        "foo.com/bar",
				Version:           "v1.2.0",
				IsRedistributable: true,
			},
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			got, err := ds.GetPathInfo(ctx, test.path, test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}
			test.want.Path = test.path
			if diff := cmp.Diff(got, test.want); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDataSource_Bypass(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass=%t", bypass), func(t *testing.T) {
			// re-create the data source to get around caching
			ctx, ds, teardown := setup(t)
			defer teardown()
			ds.bypassLicenseCheck = bypass
			for _, mpath := range []string{"foo.com/bar", "foo.com/nr"} {
				wantEmpty := !bypass && strings.HasSuffix(mpath, "/nr")

				got, err := ds.getModule(ctx, mpath, "v1.1.0")
				if err != nil {
					t.Fatal(err)
				}
				// Assume internal.Module.RemoveNonRedistributableData is correct; we just
				// need to check one value to confirm that it was called.
				if gotEmpty := (got.Licenses[0].Contents == nil); gotEmpty != wantEmpty {
					t.Errorf("bypass %t for %q: got empty %t, want %t", bypass, mpath, gotEmpty, wantEmpty)
				}
			}
		})
	}
}

func TestDataSource_GetLatestMajorVersion(t *testing.T) {
	t.Helper()
	testModules := []*proxy.Module{
		{
			ModulePath: "foo.com/bar",
		},
		{
			ModulePath: "foo.com/bar/v2",
		},
		{
			ModulePath: "foo.com/bar/v3",
		},
		{
			ModulePath: "bar.com/foo",
		},
	}
	client, teardownProxy := proxy.SetupTestClient(t, testModules)
	defer teardownProxy()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ds := New(client)

	for _, test := range []struct {
		seriesPath  string
		wantVersion string
		wantErr     error
	}{
		{
			seriesPath:  "foo.com/bar",
			wantVersion: "/v3",
		},
		{
			seriesPath:  "bar.com/foo",
			wantVersion: "",
		},
		{
			seriesPath: "boo.com/far",
			wantErr:    derrors.NotFound,
		},
	} {
		gotVersion, err := ds.GetLatestMajorVersion(ctx, test.seriesPath)
		if err != nil {
			if test.wantErr == nil {
				t.Fatalf("got unexpected error %v", err)
			}
			if !errors.Is(err, test.wantErr) {
				t.Errorf("got err = %v, want Is(%v)", err, test.wantErr)
			}
		}
		if gotVersion != test.wantVersion {
			t.Errorf("GetLatestMajorVersion(%v) = %v, want %v", test.seriesPath, gotVersion, test.wantVersion)
		}
	}
}
