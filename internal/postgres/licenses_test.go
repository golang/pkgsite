// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/pkgsite/internal/derrors"
	"golang.org/x/pkgsite/internal/licenses"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func TestGetLicenses(t *testing.T) {
	testModule := sample.Module(sample.ModulePath, "v1.2.3", "A/B")
	mit := &licenses.Metadata{Types: []string{"MIT"}, FilePath: "LICENSE"}
	bsd := &licenses.Metadata{Types: []string{"BSD-3-Clause"}, FilePath: "A/B/LICENSE"}

	mitLicense := &licenses.License{Metadata: mit}
	bsdLicense := &licenses.License{Metadata: bsd}
	testModule.Licenses = []*licenses.License{bsdLicense, mitLicense}
	sort.Slice(testModule.Directories, func(i, j int) bool {
		return testModule.Directories[i].Path < testModule.Directories[j].Path
	})

	// github.com/valid/module_name
	testModule.Directories[0].Licenses = []*licenses.Metadata{mit}
	// github.com/valid/module_name/A
	testModule.Directories[1].Licenses = []*licenses.Metadata{mit}
	// github.com/valid/module_name/A/B
	testModule.Directories[2].Licenses = []*licenses.Metadata{mit, bsd}

	tests := []struct {
		err      error
		name     string
		fullPath string
		want     []*licenses.License
	}{
		{name: "empty path", err: derrors.InvalidArgument},
		{name: "module root", fullPath: sample.ModulePath, want: []*licenses.License{testModule.Licenses[1]}},
		{name: "package without license", fullPath: sample.ModulePath + "/A", want: []*licenses.License{testModule.Licenses[1]}},
		{name: "package with additional license", fullPath: sample.ModulePath + "/A/B", want: testModule.Licenses},
	}

	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout*5)
	defer cancel()
	if err := testDB.InsertModule(ctx, testModule); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := testDB.GetLicenses(ctx, test.fullPath, sample.ModulePath, testModule.Version)
			if !errors.Is(err, test.err) {
				t.Fatal(err)
			}

			sort.Slice(got, func(i, j int) bool {
				return got[i].FilePath < got[j].FilePath
			})
			sort.Slice(test.want, func(i, j int) bool {
				return test.want[i].FilePath < test.want[j].FilePath
			})
			for i := range got {
				sort.Strings(got[i].Types)
			}
			for i := range test.want {
				sort.Strings(test.want[i].Types)
			}

			cmpopt := cmpopts.IgnoreFields(licenses.License{}, "Contents")
			if diff := cmp.Diff(test.want, got, cmpopt); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLegacyGetModuleLicenses(t *testing.T) {
	modulePath := "test.module"
	testModule := sample.Module(modulePath, "v1.2.3", "", "foo", "bar")
	testModule.LegacyPackages[0].Licenses = []*licenses.Metadata{{Types: []string{"ISC"}, FilePath: "LICENSE"}}
	testModule.LegacyPackages[1].Licenses = []*licenses.Metadata{{Types: []string{"MIT"}, FilePath: "foo/LICENSE"}}
	testModule.LegacyPackages[2].Licenses = []*licenses.Metadata{{Types: []string{"GPL2"}, FilePath: "bar/LICENSE.txt"}}

	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testModule.Licenses = nil
	for _, p := range testModule.LegacyPackages {
		testModule.Licenses = append(testModule.Licenses, &licenses.License{
			Metadata: p.Licenses[0],
			Contents: []byte(`Lorem Ipsum`),
		})
	}

	if err := testDB.InsertModule(ctx, testModule); err != nil {
		t.Fatal(err)
	}

	got, err := testDB.LegacyGetModuleLicenses(ctx, modulePath, testModule.Version)
	if err != nil {
		t.Fatal(err)
	}
	// We only want the top-level license.
	wantLicenses := []*licenses.License{testModule.Licenses[0]}
	if diff := cmp.Diff(wantLicenses, got); diff != "" {
		t.Errorf("testDB.LegacyGetModuleLicenses(ctx, %q, %q) mismatch (-want +got):\n%s", modulePath, testModule.Version, diff)
	}
}

func TestLegacyGetPackageLicenses(t *testing.T) {
	modulePath := "test.module"
	testModule := sample.Module(modulePath, "v1.2.3", "", "foo")
	testModule.LegacyPackages[0].Licenses = nil
	testModule.LegacyPackages[1].Licenses = sample.LicenseMetadata

	tests := []struct {
		label, pkgPath string
		wantLicenses   []*licenses.License
	}{
		{
			label:        "package with licenses",
			pkgPath:      "test.module/foo",
			wantLicenses: sample.Licenses,
		}, {
			label:        "package with no licenses",
			pkgPath:      "test.module",
			wantLicenses: nil,
		},
	}

	defer ResetTestDB(testDB, t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := testDB.InsertModule(ctx, testModule); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			got, err := testDB.LegacyGetPackageLicenses(ctx, test.pkgPath, modulePath, testModule.Version)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.wantLicenses, got); diff != "" {
				t.Errorf("testDB.GetLicenses(ctx, %q, %q) mismatch (-want +got):\n%s", test.pkgPath, testModule.Version, diff)
			}
		})
	}
}
