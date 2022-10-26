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
	"golang.org/x/pkgsite/internal/vulns"
	"golang.org/x/vuln/osv"
)

var (
	modulePath1 = "test.com/module"
	modulePath2 = "test.com/module/v2"
)

func sampleModule(modulePath, version string, versionType version.Type, packages ...*internal.Unit) *internal.Module {
	if len(packages) == 0 {
		return sample.Module(modulePath, version, sample.Suffix)
	}
	m := sample.Module(modulePath, version)
	for _, p := range packages {
		sample.AddUnit(m, p)
	}
	return m
}

func versionSummaries(path string, versions []string, linkify func(path, version string) string) []*VersionSummary {
	vs := make([]*VersionSummary, len(versions))
	for i, version := range versions {
		vs[i] = &VersionSummary{
			Version:    version,
			Link:       linkify(path, version),
			CommitTime: absoluteTime(sample.CommitTime),
			IsMinor:    isMinor(version),
		}
	}
	return vs
}

func TestFetchPackageVersionsDetails(t *testing.T) {
	var (
		v2Path = "test.com/module/v2/foo"
		v1Path = "test.com/module/foo"
	)

	pkg1 := &internal.Unit{
		UnitMeta: *sample.UnitMeta(
			modulePath1+"/"+sample.Suffix,
			modulePath1,
			"v1.2.1",
			sample.Suffix,
			true),
		Documentation: []*internal.Documentation{sample.Doc},
	}
	pkg2 := &internal.Unit{
		UnitMeta: *sample.UnitMeta(
			modulePath2+"/"+sample.Suffix,
			modulePath2,
			"v1.2.1-alpha.1",
			sample.Suffix,
			true),
		Documentation: []*internal.Documentation{sample.Doc},
	}
	nethttpPkg := &internal.Unit{
		UnitMeta: *sample.UnitMeta(
			"net/http",
			"std",
			"v1.12.5",
			"http",
			true),
		Documentation: []*internal.Documentation{sample.Doc},
	}
	makeList := func(pkgPath, modulePath, major string, versions []string, incompatible bool) *VersionList {
		return &VersionList{
			VersionListKey: VersionListKey{ModulePath: modulePath, Major: major, Incompatible: incompatible},
			Versions: versionSummaries(pkgPath, versions, func(path, version string) string {
				return constructUnitURL(pkgPath, modulePath, version)
			}),
		}
	}

	vulnFixedVersion := "1.2.3"
	vulnEntry := &osv.Entry{
		Details: "vuln",
		Affected: []osv.Affected{{
			Package: osv.Package{
				Name: modulePath1,
			},
			Ranges: []osv.AffectsRange{{
				Type:   osv.TypeSemver,
				Events: []osv.RangeEvent{{Introduced: "1.2.0"}, {Fixed: vulnFixedVersion}},
			}},
			EcosystemSpecific: osv.EcosystemSpecific{
				Imports: []osv.EcosystemSpecificImport{{
					Path: v1Path,
				}},
			},
		}},
	}
	getVulnEntries := func(_ context.Context, m string) ([]*osv.Entry, error) {
		if m == modulePath1 {
			return []*osv.Entry{vulnEntry}, nil
		}
		return nil, nil
	}

	for _, tc := range []struct {
		name        string
		pkg         *internal.Unit
		modules     []*internal.Module
		wantDetails *VersionsDetails
	}{
		{
			name: "want stdlib versions",
			pkg:  nethttpPkg,
			modules: []*internal.Module{
				sampleModule("std", "v1.12.5", version.TypeRelease, nethttpPkg),
				sampleModule("std", "v1.11.6", version.TypeRelease, nethttpPkg),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					makeList("net/http", "std", "go1", []string{"go1.12.5", "go1.11.6"}, false),
				},
			},
		},
		{
			name: "want v1 first",
			pkg:  pkg1,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", version.TypePseudo, pkg2),
				sampleModule(modulePath1, "v1.2.3", version.TypeRelease, pkg1),
				sampleModule(modulePath1, "v2.1.0+incompatible", version.TypeRelease, pkg1),
				sampleModule(modulePath2, "v2.0.0", version.TypeRelease, pkg2),
				sampleModule(modulePath1, "v1.3.0", version.TypeRelease, pkg1),
				sampleModule(modulePath1, "v1.2.1", version.TypeRelease, pkg1),
				sampleModule(modulePath2, "v2.2.1-alpha.1", version.TypePrerelease, pkg2),
				sampleModule("test.com", "v1.2.1", version.TypeRelease, pkg1),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					func() *VersionList {
						vl := makeList(v1Path, modulePath1, "v1", []string{"v1.3.0", "v1.2.3", "v1.2.1"}, false)
						vl.Versions[2].Vulns = []vulns.Vuln{{
							Details: vulnEntry.Details,
						}}
						return vl
					}(),
				},
				IncompatibleModules: []*VersionList{
					makeList(v1Path, modulePath1, "v2", []string{"v2.1.0+incompatible"}, true),
				},
				OtherModules: []string{"test.com", modulePath2},
			},
		},
		{
			name: "want v2 first",
			pkg:  pkg2,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041502-3c2ca4d52544", version.TypePseudo, pkg1),
				sampleModule(modulePath1, "v1.2.1", version.TypeRelease, pkg1),
				sampleModule(modulePath1, "v1.2.3", version.TypeRelease, pkg1),
				sampleModule(modulePath1, "v2.1.0+incompatible", version.TypeRelease, pkg1),
				sampleModule(modulePath2, "v2.0.0", version.TypeRelease, pkg2),
				sampleModule(modulePath2, "v2.2.1-alpha.1", version.TypePrerelease, pkg2),
			},
			wantDetails: &VersionsDetails{
				ThisModule: []*VersionList{
					makeList(v2Path, modulePath2, "v2", []string{"v2.2.1-alpha.1", "v2.0.0"}, false),
				},
				OtherModules: []string{modulePath1},
			},
		},
		{
			name: "want only pseudo",
			pkg:  pkg2,
			modules: []*internal.Module{
				sampleModule(modulePath1, "v0.0.0-20140414041501-3c2ca4d52544", version.TypePseudo, pkg2),
				sampleModule(modulePath1, "v0.0.0-20140414041502-4c2ca4d52544", version.TypePseudo, pkg2),
			},
			wantDetails: &VersionsDetails{
				OtherModules: []string{modulePath1},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout*2)
			defer cancel()
			defer postgres.ResetTestDB(testDB, t)

			for _, v := range tc.modules {
				postgres.MustInsertModule(ctx, t, testDB, v)
			}

			got, err := fetchVersionsDetails(ctx, testDB, &tc.pkg.UnitMeta, getVulnEntries)
			if err != nil {
				t.Fatalf("fetchVersionsDetails(ctx, db, %q, %q): %v", tc.pkg.Path, tc.pkg.ModulePath, err)
			}
			for _, vl := range tc.wantDetails.ThisModule {
				for _, v := range vl.Versions {
					v.CommitTime = absoluteTime(tc.modules[0].CommitTime)
				}
			}
			if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
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
		{"v1.2.3-alpha.1", "v1.2.3-alpha.1"},
		{"v1.0.0-20190311183353-d8887717615a", "v1.0.0-...-d888771"},
		{"v1.2.3-pre.0.20190311183353-d8887717615a", "v1.2.3-pre.0...-d888771"},
		{"v1.2.4-0.20190311183353-d8887717615a", "v1.2.4-0...-d888771"},
		{"v1.0.0-20190311183353-d88877", "v1.0.0-...-d88877"},
		{"v1.0.0-longprereleasestring", "v1.0.0-longprereleases..."},
		{"v1.0.0-pre-release.0.20200420093620-87861123c523", "v1.0.0-pre-rele...-8786112"},
		{"v0.0.0-20190101-123456789012", "v0.0.0-20190101-123456..."}, // prelease version that looks like pseudoversion
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			if got := formatVersion(test.version); got != test.want {
				t.Errorf("formatVersion(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}

func TestPseudoVersionBase(t *testing.T) {
	tests := []struct {
		version, want string
	}{
		{"v1.0.0-20190311183353-d8887717615a", "v1.0.0-"},
		{"v1.2.3-pre.0.20190311183353-d8887717615a", "v1.2.3-pre.0"},
		{"v1.2.4-0.20190311183353-d8887717615a", "v1.2.4-0"},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			if got := pseudoVersionBase(test.version); got != test.want {
				t.Errorf("pseudoVersionBase(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}

func TestIsMinor(t *testing.T) {
	for _, test := range []struct {
		version string
		want    bool
	}{
		{"v0.5.0", true},
		{"v1.0.0-pre", false},
		{"v1.0.0", true},
		{"v1.0.1", false},
		{"v2.0.0+incompatible", false},
		{"v1.0.0-20190311183353-d8887717615a", false},
		{"v1.2.3-pre.0.20190311183353-d8887717615a", false},
		{"v1.2.4-0.20190311183353-d8887717615a", false},
	} {
		t.Run(test.version, func(t *testing.T) {
			if got := isMinor(test.version); got != test.want {
				t.Errorf("isMinor(%q) = %t, want %t", test.version, got, test.want)
			}
		})
	}
}

func TestDisplayVersion(t *testing.T) {
	for _, test := range []struct {
		name             string
		fullPath         string
		requestedVersion string
		resolvedVersion  string
		want             string
	}{
		{
			"std @ master",
			stdlib.ModulePath,
			version.Master,
			stdlib.TestMasterVersion,
			"master (89fb59e)",
		},
		{
			"std @ latest is master",
			stdlib.ModulePath,
			version.Latest,
			stdlib.TestMasterVersion,
			"master (89fb59e)",
		},
		{
			"std @ latest is go1.16",
			stdlib.ModulePath,
			version.Latest,
			"v1.16.0",
			"go1.16",
		},
		{
			"std @ go1.16",
			stdlib.ModulePath,
			"v1.16.0",
			"v1.16.0",
			"go1.16",
		},
		{
			"std @ dev.fuzz",
			stdlib.ModulePath,
			"dev.fuzz",
			stdlib.TestMasterVersion,
			"dev.fuzz (89fb59e)",
		},
		{
			"github.com path @ latest is v1.5.2",
			sample.ModulePath,
			version.Latest,
			"v1.5.2",
			"v1.5.2",
		},
		{
			"github.com path @ master is v1.5.2",
			sample.ModulePath,
			version.Master,
			"v1.5.2",
			"v1.5.2",
		},
		{
			"github.com path @ v1.5.2",
			sample.ModulePath,
			"v1.5.2",
			"v1.5.2",
			"v1.5.2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := displayVersion(test.fullPath, test.requestedVersion, test.resolvedVersion); got != test.want {
				t.Errorf("displayVersion(%q, %q, %q) = %q, want %q",
					test.fullPath, test.requestedVersion, test.resolvedVersion, got, test.want)
			}
		})
	}
}

func TestLinkVersion(t *testing.T) {
	for _, test := range []struct {
		name             string
		fullPath         string
		requestedVersion string
		resolvedVersion  string
		want             string
	}{
		{
			"std @ master",
			stdlib.ModulePath,
			version.Master,
			stdlib.TestMasterVersion,
			version.Master,
		},
		{
			"std @ latest is master",
			stdlib.ModulePath,
			version.Latest,
			stdlib.TestMasterVersion,
			version.Master,
		},
		{
			"std @ latest is go1.16",
			stdlib.ModulePath,
			version.Latest,
			"v1.16.0",
			"go1.16",
		},
		{
			"std @ go1.16",
			stdlib.ModulePath,
			"v1.16.0",
			"v1.16.0",
			"go1.16",
		},
		{
			"std @ dev.fuzz",
			stdlib.ModulePath,
			"dev.fuzz",
			stdlib.TestMasterVersion,
			"dev.fuzz",
		},
		{
			"github.com path @ latest is v1.5.2",
			sample.ModulePath,
			version.Latest,
			"v1.5.2",
			"v1.5.2",
		},
		{
			"github.com path @ master is v1.5.2",
			sample.ModulePath,
			version.Master,
			"v1.5.2",
			"v1.5.2",
		},
		{
			"github.com path @ v1.5.2",
			sample.ModulePath,
			"v1.5.2",
			"v1.5.2",
			"v1.5.2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linkVersion(test.fullPath, test.requestedVersion, test.resolvedVersion); got != test.want {
				t.Errorf("linkVersion(%q, %q, %q) = %q, want %q",
					test.fullPath, test.requestedVersion, test.resolvedVersion, got, test.want)
			}
		})
	}
}
