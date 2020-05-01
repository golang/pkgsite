// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/stdlib"
	"golang.org/x/pkgsite/internal/testing/sample"
	"golang.org/x/pkgsite/internal/version"
)

var (
	modulePath1 = "test.com/module"
	modulePath2 = "test.com/module/v2"
	commitTime  = "0 hours ago"
)

func sampleModule(modulePath, version string, versionType version.Type, packages ...*internal.Package) *internal.Module {
	if len(packages) == 0 {
		return sample.Module(modulePath, version, sample.Suffix)
	}
	m := sample.Module(modulePath, version)
	for _, p := range packages {
		sample.AddPackage(m, p)
	}
	return m
}

func versionSummaries(path string, versions []string, linkify func(path, version string) string) []*VersionSummary {
	vs := make([]*VersionSummary, len(versions))
	for i, version := range versions {
		var semver, displayVersion string
		if stdlib.Contains(path) {
			semver = version
			displayVersion = version
		} else {
			semver = version
			displayVersion = formatVersion(semver)
		}
		vs[i] = &VersionSummary{
			TooltipVersion: semver,
			DisplayVersion: displayVersion,
			Link:           linkify(path, version),
			CommitTime:     commitTime,
		}
	}
	return vs
}

func TestFetchModuleVersionDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	info1 := sample.ModuleInfo(modulePath1, "v1.2.1")
	info2 := sample.ModuleInfo(modulePath2, "v2.2.1-alpha.1")
	makeList := func(path, major string, versions []string) *VersionList {
		return &VersionList{
			VersionListKey: VersionListKey{ModulePath: path, Major: major},
			Versions: versionSummaries(path, versions, func(path, version string) string {
				return constructModuleURL(path, version)
			}),
		}
	}

	for _, tc := range []struct {
		name        string
		info        *internal.ModuleInfo
		modules     []*internal.Module
		wantDetails *VersionsDetails
	}{
		{
			name: "want v1 first",
			info: info1,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", version.TypePseudo),
				sampleModule(modulePath1, "v1.2.3", version.TypeRelease),
				sampleModule(modulePath2, "v2.0.0", version.TypeRelease),
				sampleModule(modulePath1, "v1.3.0", version.TypeRelease),
				sampleModule(modulePath1, "v1.2.1", version.TypeRelease),
				sampleModule(modulePath2, "v2.2.1-alpha.1", version.TypePrerelease),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					makeList("test.com/module", "v1", []string{"v1.3.0", "v1.2.3", "v1.2.1"}),
				},
				OtherModules: []*VersionList{
					makeList("test.com/module/v2", "v2", []string{"v2.2.1-alpha.1", "v2.0.0"}),
				},
			},
		},
		{
			name: "want v2 first",
			info: info2,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", version.TypePseudo),
				sampleModule(modulePath1, "v1.2.1", version.TypeRelease),
				sampleModule(modulePath1, "v1.2.3", version.TypeRelease),
				sampleModule(modulePath1, "v2.1.0+incompatible", version.TypeRelease),
				sampleModule(modulePath2, "v2.0.0", version.TypeRelease),
				sampleModule(modulePath2, "v2.2.1-alpha.1", version.TypePrerelease),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					makeList("test.com/module/v2", "v2", []string{"v2.2.1-alpha.1", "v2.0.0"}),
				},
				OtherModules: []*VersionList{
					makeList("test.com/module", "v1", []string{"v2.1.0+incompatible", "v1.2.3", "v1.2.1"}),
				},
			},
		},
		{
			name: "want only pseudo",
			info: info2,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", version.TypePseudo),
				sampleModule(modulePath1, "v0.0.0-20140414041502-4c2ca4d52544", version.TypePseudo),
			},
			wantDetails: &VersionsDetails{
				OtherModules: []*VersionList{
					makeList("test.com/module", "v0", []string{
						"v0.0.0-20140414041502-4c2ca4d52544",
						"v0.0.0-20140414041501-3c2ca4d52544"},
					),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.modules {
				if err := testDB.InsertModule(ctx, v); err != nil {
					t.Fatal(err)
				}
			}

			got, err := fetchModuleVersionsDetails(ctx, testDB, tc.info)
			if err != nil {
				t.Fatalf("fetchModuleVersionsDetails(ctx, db, %v): %v", tc.info, err)
			}
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchModuleVersionsDetails(ctx, db, %v) mismatch (-want +got):\n%s", tc.info, diff)
			}
		})
	}
}

func TestFetchPackageVersionsDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var (
		v2Path = "test.com/module/v2/foo"
		v1Path = "test.com/module/foo"
	)

	pkg1 := &internal.VersionedPackage{
		ModuleInfo: *sample.ModuleInfo(modulePath1, "v1.2.1"),
		Package:    *sample.Package(modulePath1, sample.Suffix),
	}
	pkg2 := &internal.VersionedPackage{
		ModuleInfo: *sample.ModuleInfo(modulePath2, "v2.2.1-alpha.1"),
		Package:    *sample.Package(modulePath2, sample.Suffix),
	}
	nethttpPkg := &internal.VersionedPackage{
		ModuleInfo: *sample.ModuleInfo("std", "v1.12.5"),
		Package:    *sample.Package("std", "net/http"),
	}
	makeList := func(pkgPath, modulePath, major string, versions []string) *VersionList {
		return &VersionList{
			VersionListKey: VersionListKey{ModulePath: modulePath, Major: major},
			Versions: versionSummaries(pkgPath, versions, func(path, version string) string {
				return constructPackageURL(pkgPath, modulePath, version)
			}),
		}
	}

	for _, tc := range []struct {
		name        string
		pkg         *internal.VersionedPackage
		modules     []*internal.Module
		wantDetails *VersionsDetails
	}{
		{
			name: "want stdlib versions",
			pkg:  nethttpPkg,
			modules: []*internal.Module{
				sampleModule("std", "v1.12.5", version.TypeRelease, &nethttpPkg.Package),
				sampleModule("std", "v1.11.6", version.TypeRelease, &nethttpPkg.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					makeList("net/http", "std", "go1", []string{"go1.12.5", "go1.11.6"}),
				},
			},
		},
		{
			name: "want v1 first",
			pkg:  pkg1,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", version.TypePseudo, &pkg2.Package),
				sampleModule(modulePath1, "v1.2.3", version.TypeRelease, &pkg1.Package),
				sampleModule(modulePath2, "v2.0.0", version.TypeRelease, &pkg2.Package),
				sampleModule(modulePath1, "v1.3.0", version.TypeRelease, &pkg1.Package),
				sampleModule(modulePath1, "v1.2.1", version.TypeRelease, &pkg1.Package),
				sampleModule(modulePath2, "v2.2.1-alpha.1", version.TypePrerelease, &pkg2.Package),
				sampleModule("test.com", "v1.2.1", version.TypeRelease, &pkg1.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					makeList(v1Path, modulePath1, "v1", []string{"v1.3.0", "v1.2.3", "v1.2.1"}),
				},
				OtherModules: []*VersionList{
					makeList(v2Path, modulePath2, "v2", []string{"v2.2.1-alpha.1", "v2.0.0"}),
					makeList(v1Path, "test.com", "v1", []string{"v1.2.1"}),
				},
			},
		},
		{
			name: "want v2 first",
			pkg:  pkg2,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", version.TypePseudo, &pkg1.Package),
				sampleModule(modulePath1, "v1.2.1", version.TypeRelease, &pkg1.Package),
				sampleModule(modulePath1, "v1.2.3", version.TypeRelease, &pkg1.Package),
				sampleModule(modulePath1, "v2.1.0+incompatible", version.TypeRelease, &pkg1.Package),
				sampleModule(modulePath2, "v2.0.0", version.TypeRelease, &pkg2.Package),
				sampleModule(modulePath2, "v2.2.1-alpha.1", version.TypePrerelease, &pkg2.Package),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					makeList(v2Path, modulePath2, "v2", []string{"v2.2.1-alpha.1", "v2.0.0"}),
				},
				OtherModules: []*VersionList{
					makeList(v1Path, modulePath1, "v1", []string{"v2.1.0+incompatible", "v1.2.3", "v1.2.1"}),
				},
			},
		},
		{
			name: "want only pseudo",
			pkg:  pkg2,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", version.TypePseudo, &pkg2.Package),
				sampleModule(modulePath1, "v0.0.0-20140414041502-4c2ca4d52544", version.TypePseudo, &pkg2.Package),
			},
			wantDetails: &VersionsDetails{
				OtherModules: []*VersionList{
					makeList(v1Path, modulePath1, "v0", []string{
						"v0.0.0-20140414041502-4c2ca4d52544",
						"v0.0.0-20140414041501-3c2ca4d52544",
					}),
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.modules {
				if err := testDB.InsertModule(ctx, v); err != nil {
					t.Fatal(err)
				}
			}

			got, err := fetchPackageVersionsDetails(ctx, testDB, tc.pkg)
			if err != nil {
				t.Fatalf("fetchPackageVersionsDetails(ctx, db, %v): %v", tc.pkg, err)
			}
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("fetchPackageVersionsDetails(ctx, db, %v) mismatch (-want +got):\n%s", tc.pkg, diff)
			}
		})
	}
}

func TestPathInVersion(t *testing.T) {
	tests := []struct {
		v1Path, modulePath, want string
	}{
		{"foo.com/bar/baz", "foo.com/bar", "foo.com/bar/baz"},
		{"foo.com/bar/baz", "foo.com/bar/v2", "foo.com/bar/v2/baz"},
		{"foo.com/bar/baz", "foo.com/v3", "foo.com/v3/bar/baz"},
		{"foo.com/bar/baz", "foo.com/bar/baz/v3", "foo.com/bar/baz/v3"},
	}

	for _, test := range tests {
		mi := sample.ModuleInfo(test.modulePath, sample.VersionString)
		if got := pathInVersion(test.v1Path, mi); got != test.want {
			t.Errorf("pathInVersion(%q, ModuleInfo{...ModulePath:%q}) = %s, want %v",
				test.v1Path, mi.ModulePath, got, test.want)
		}
	}
}

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		version, want string
	}{
		{"v1.2.3", "v1.2.3"},
		{"v2.0.0", "v2.0.0"},
		{"v1.2.3-alpha.1", "v1.2.3 (alpha.1)"},
		{"v1.0.0-20190311183353-d8887717615a", "v1.0.0 (d888771)"},
		{"v1.2.3-pre.0.20190311183353-d8887717615a", "v1.2.3 (d888771)"},
		{"v1.2.4-0.20190311183353-d8887717615a", "v1.2.4 (d888771)"},
		{"v1.0.0-20190311183353-d88877", "v1.0.0 (d88877)"},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			if got := formatVersion(test.version); got != test.want {
				t.Errorf("formatVersion(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}
