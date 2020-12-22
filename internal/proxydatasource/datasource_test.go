// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxydatasource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/safehtml/template"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/testing/testhelper"
)

func setup(t *testing.T) (context.Context, *DataSource, func()) {
	t.Helper()
	dochtml.LoadTemplates(template.TrustedSourceFromConstant("../../content/static/html/doc"))

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
	wantLicenseMD = sample.LicenseMetadata[0]
	wantPackage   = internal.Unit{
		UnitMeta: internal.UnitMeta{
			Path:              "foo.com/bar/baz",
			Name:              "baz",
			ModulePath:        "foo.com/bar",
			Version:           "v1.2.0",
			CommitTime:        time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
			Licenses:          []*licenses.Metadata{wantLicenseMD},
			IsRedistributable: true,
		},
		Imports: []string{"net/http"},
		Documentation: &internal.Documentation{
			Synopsis: "Package baz provides a helpful constant.",
			GOOS:     "linux",
			GOARCH:   "amd64",
		},
	}
	wantModuleInfo = internal.ModuleInfo{
		ModulePath:        "foo.com/bar",
		Version:           "v1.2.0",
		CommitTime:        time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
		IsRedistributable: true,
		HasGoMod:          true,
	}
	cmpOpts = append([]cmp.Option{
		cmpopts.IgnoreFields(licenses.License{}, "Contents"),
	}, sample.LicenseCmpOpts...)
)

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

func TestDataSource_GetUnitMeta(t *testing.T) {
	ctx, ds, teardown := setup(t)
	defer teardown()

	for _, test := range []struct {
		path, modulePath, version string
		want                      *internal.UnitMeta
	}{
		{
			path:       "foo.com/bar",
			modulePath: "foo.com/bar",
			version:    "v1.1.0",
			want: &internal.UnitMeta{
				ModulePath:        "foo.com/bar",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			path:       "foo.com/bar/baz",
			modulePath: "foo.com/bar",
			version:    "v1.1.0",
			want: &internal.UnitMeta{
				ModulePath:        "foo.com/bar",
				Name:              "baz",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			path:       "foo.com/bar/baz",
			modulePath: internal.UnknownModulePath,
			version:    "v1.1.0",
			want: &internal.UnitMeta{
				ModulePath:        "foo.com/bar",
				Name:              "baz",
				Version:           "v1.1.0",
				IsRedistributable: true,
			},
		},
		{
			path:       "foo.com/bar/baz",
			modulePath: internal.UnknownModulePath,
			version:    internal.LatestVersion,
			want: &internal.UnitMeta{
				ModulePath:        "foo.com/bar",
				Name:              "baz",
				Version:           "v1.2.0",
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

func TestDataSource_GetLatestInfo(t *testing.T) {
	t.Helper()
	testModules := []*proxy.Module{
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
	client, teardownProxy := proxy.SetupTestClient(t, testModules)
	defer teardownProxy()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ds := New(client)

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
		gotLatest, err := ds.GetLatestInfo(ctx, test.fullPath, test.modulePath)
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
