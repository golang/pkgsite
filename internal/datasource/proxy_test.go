// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package datasource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/proxy/proxytest"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

var testModules []*proxytest.Module

func TestMain(m *testing.M) {
	dochtml.LoadTemplates(template.TrustedSourceFromConstant("../../static/doc"))
	testModules = proxytest.LoadTestModules("../proxy/testdata")
	licenses.OmitExceptions = true
	defer setupLocal()()
	os.Exit(m.Run())
}

func setup(t *testing.T, bypassLicenseCheck bool) (context.Context, *ProxyDataSource, func()) {
	t.Helper()
	client, teardownProxy := proxytest.SetupTestClient(t, testModules)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	return ctx, NewForTesting(client, bypassLicenseCheck), func() {
		teardownProxy()
		cancel()
	}
}

var (
	wantLicenseMD = sample.LicenseMetadata()[0]
	wantPackage   = internal.Unit{
		UnitMeta: internal.UnitMeta{
			Path: "foo.com/bar/baz",
			Name: "baz",
			ModuleInfo: internal.ModuleInfo{
				ModulePath:        "foo.com/bar",
				Version:           "v1.2.0",
				CommitTime:        time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
				IsRedistributable: true,
			},
			Licenses:          []*licenses.Metadata{wantLicenseMD},
			IsRedistributable: true,
		},
		Imports: []string{"net/http"},
		Documentation: []*internal.Documentation{{
			Synopsis: "Package baz provides a helpful constant.",
			GOOS:     "linux",
			GOARCH:   "amd64",
		}},
	}
)

func TestProxyGetUnitMeta(t *testing.T) {
	ctx, ds, teardown := setup(t, false)
	defer teardown()

	singleModInfo := internal.ModuleInfo{
		ModulePath:        "example.com/single",
		Version:           "v1.0.0",
		IsRedistributable: true,
		CommitTime:        proxytest.CommitTime,
		HasGoMod:          true,
	}

	for _, test := range []struct {
		path, modulePath, version string
		want                      *internal.UnitMeta
	}{
		{
			path:       "example.com/single",
			modulePath: "example.com/single",
			version:    "v1.0.0",
			want: &internal.UnitMeta{
				ModuleInfo:        singleModInfo,
				IsRedistributable: true,
			},
		},
		{
			path:       "example.com/single/pkg",
			modulePath: "example.com/single",
			version:    "v1.0.0",
			want: &internal.UnitMeta{
				ModuleInfo:        singleModInfo,
				Name:              "pkg",
				IsRedistributable: true,
			},
		},
		{
			path:       "example.com/single/pkg",
			modulePath: internal.UnknownModulePath,
			version:    "v1.0.0",
			want: &internal.UnitMeta{
				ModuleInfo:        singleModInfo,
				Name:              "pkg",
				IsRedistributable: true,
			},
		},
		{
			path:       "example.com/basic",
			modulePath: internal.UnknownModulePath,
			version:    version.Latest,
			want: &internal.UnitMeta{
				ModuleInfo: internal.ModuleInfo{
					ModulePath:        "example.com/basic",
					Version:           "v1.1.0",
					IsRedistributable: true,
					CommitTime:        proxytest.CommitTime,
					HasGoMod:          true,
				},
				Name:              "basic",
				IsRedistributable: true,
			},
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			got, err := ds.GetUnitMeta(ctx, test.path, test.modulePath, test.version)
			if err != nil {
				t.Fatal(err)
			}
			test.want.Path = test.path
			if diff := cmp.Diff(test.want, got, cmpopts.IgnoreFields(internal.ModuleInfo{}, "SourceInfo")); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBypass(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass=%t", bypass), func(t *testing.T) {
			// re-create the data source to get around caching
			ctx, ds, teardown := setup(t, bypass)
			defer teardown()
			for _, test := range []struct {
				path      string
				wantEmpty bool
			}{
				{"example.com/basic", false},
				{"example.com/nonredist/unk", !bypass},
			} {
				t.Run(test.path, func(t *testing.T) {
					um, err := ds.GetUnitMeta(ctx, test.path, internal.UnknownModulePath, "v1.0.0")
					if err != nil {
						t.Fatal(err)
					}
					got, err := ds.GetUnit(ctx, um, 0, internal.BuildContext{})
					if err != nil {
						t.Fatal(err)
					}

					// Assume internal.Module.RemoveNonRedistributableData is correct; we just
					// need to check one value to confirm that it was called.
					if gotEmpty := (got.Documentation == nil); gotEmpty != test.wantEmpty {
						t.Errorf("got empty %t, want %t", gotEmpty, test.wantEmpty)
					}
				})
			}
		})
	}
}

func TestGetLatestInfo(t *testing.T) {
	t.Helper()
	testModules := []*proxytest.Module{
		{
			ModulePath: "foo.com/bar",
			Version:    "v1.1.0",
			Files: map[string]string{
				"baz.go": "package bar",
			},
		},
		{
			ModulePath: "foo.com/bar/v2",
			Version:    "v2.0.5",
		},
		{
			ModulePath: "foo.com/bar/v3",
		},
		{
			ModulePath: "bar.com/foo",
			Version:    "v1.1.0",
			Files: map[string]string{
				"baz.go": "package foo",
			},
		},
		{
			ModulePath: "incompatible.com/bar",
			Version:    "v2.1.1+incompatible",
			Files: map[string]string{
				"baz.go": "package bar",
			},
		},
		{
			ModulePath: "incompatible.com/bar/v3",
		},
	}
	client, teardownProxy := proxytest.SetupTestClient(t, testModules)
	defer teardownProxy()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ds := NewForTesting(client, false)

	for _, test := range []struct {
		fullPath        string
		modulePath      string
		wantModulePath  string
		wantPackagePath string
		wantErr         error
	}{
		{
			fullPath:        "foo.com/bar",
			modulePath:      "foo.com/bar",
			wantModulePath:  "foo.com/bar/v3",
			wantPackagePath: "foo.com/bar/v3",
		},
		{
			fullPath:        "bar.com/foo",
			modulePath:      "bar.com/foo",
			wantModulePath:  "bar.com/foo",
			wantPackagePath: "bar.com/foo",
		},
		{
			fullPath:   "boo.com/far",
			modulePath: "boo.com/far",
			wantErr:    derrors.NotFound,
		},
		{
			fullPath:        "foo.com/bar/baz",
			modulePath:      "foo.com/bar",
			wantModulePath:  "foo.com/bar/v3",
			wantPackagePath: "foo.com/bar/v3",
		},
		{
			fullPath:        "incompatible.com/bar",
			modulePath:      "incompatible.com/bar",
			wantModulePath:  "incompatible.com/bar/v3",
			wantPackagePath: "incompatible.com/bar/v3",
		},
	} {
		gotLatest, err := ds.GetLatestInfo(ctx, test.fullPath, test.modulePath, nil)
		if err != nil {
			if test.wantErr == nil {
				t.Fatalf("got unexpected error %v", err)
			}
			if !errors.Is(err, test.wantErr) {
				t.Errorf("got err = %v, want Is(%v)", err, test.wantErr)
			}
		}
		if gotLatest.MajorModulePath != test.wantModulePath || gotLatest.MajorUnitPath != test.wantPackagePath {
			t.Errorf("ds.GetLatestMajorVersion(%v, %v) = (%v, %v), want = (%v, %v)",
				test.fullPath, test.modulePath, gotLatest.MajorModulePath, gotLatest.MajorUnitPath, test.wantModulePath, test.wantPackagePath)
		}
	}
}
