// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/sample"
)

func samplePackage(mutators ...func(*Package)) *Package {
	p := &Package{
		Path:              sample.PackagePath,
		Suffix:            sample.PackageName,
		Synopsis:          sample.Synopsis,
		IsRedistributable: true,
		Licenses:          transformLicenseMetadata(sample.LicenseMetadata),
		Module: Module{
			Version:           sample.VersionString,
			CommitTime:        "0 hours ago",
			Path:              sample.ModulePath,
			RepositoryURL:     sample.RepositoryURL,
			IsRedistributable: true,
			Licenses:          transformLicenseMetadata(sample.LicenseMetadata),
		},
	}
	for _, mut := range mutators {
		mut(p)
	}
	return p
}

func TestElapsedTime(t *testing.T) {
	now := sample.NowTruncated()
	testCases := []struct {
		name        string
		date        time.Time
		elapsedTime string
	}{
		{
			name:        "one_hour_ago",
			date:        now.Add(time.Hour * -1),
			elapsedTime: "1 hour ago",
		},
		{
			name:        "hours_ago",
			date:        now.Add(time.Hour * -2),
			elapsedTime: "2 hours ago",
		},
		{
			name:        "today",
			date:        now.Add(time.Hour * -8),
			elapsedTime: "today",
		},
		{
			name:        "one_day_ago",
			date:        now.Add(time.Hour * 24 * -1),
			elapsedTime: "1 day ago",
		},
		{
			name:        "days_ago",
			date:        now.Add(time.Hour * 24 * -5),
			elapsedTime: "5 days ago",
		},
		{
			name:        "more_than_6_days_ago",
			date:        now.Add(time.Hour * 24 * -14),
			elapsedTime: now.Add(time.Hour * 24 * -14).Format("Jan _2, 2006"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			elapsedTime := elapsedTime(tc.date)

			if elapsedTime != tc.elapsedTime {
				t.Errorf("elapsedTime(%q) = %s, want %s", tc.date, elapsedTime, tc.elapsedTime)
			}
		})
	}
}

// firstVersionedPackage is a helper function that returns an
// *internal.VersionedPackage corresponding to the first package in the
// version.
func firstVersionedPackage(v *internal.Version) *internal.VersionedPackage {
	return &internal.VersionedPackage{
		Package:     *v.Packages[0],
		VersionInfo: v.VersionInfo,
	}
}

func TestFetchModuleDetails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	tc := struct {
		name        string
		version     *internal.Version
		wantDetails *ModuleDetails
	}{
		name:    "want expected module details",
		version: sample.Version(),
		wantDetails: &ModuleDetails{
			ModulePath: sample.ModulePath,
			Version:    sample.VersionString,
			Packages:   []*Package{samplePackage()},
		},
	}

	if err := testDB.InsertVersion(ctx, tc.version); err != nil {
		t.Fatal(err)
	}

	got, err := fetchModuleDetails(ctx, testDB, &tc.version.VersionInfo)
	if err != nil {
		t.Fatalf("fetchModuleDetails(ctx, db, %q, %q) = %v err = %v, want %v",
			tc.version.Packages[0].Path, tc.version.Version, got, err, tc.wantDetails)
	}

	if diff := cmp.Diff(tc.wantDetails, got); diff != "" {
		t.Errorf("fetchModuleDetails(ctx, %q, %q) mismatch (-want +got):\n%s", tc.version.Packages[0].Path, tc.version.Version, diff)
	}
}

func TestCreatePackageHeader(t *testing.T) {
	for _, tc := range []struct {
		label   string
		pkg     *internal.VersionedPackage
		wantPkg *Package
	}{
		{
			label:   "simple package",
			pkg:     sample.VersionedPackage(),
			wantPkg: samplePackage(),
		},
		{
			label: "command package",
			pkg: func() *internal.VersionedPackage {
				vp := sample.VersionedPackage()
				vp.Name = "main"
				return vp
			}(),
			wantPkg: samplePackage(),
		},
		{
			label: "v2 command",
			pkg: func() *internal.VersionedPackage {
				vp := sample.VersionedPackage()
				vp.Name = "main"
				vp.Path = "pa.th/to/foo/v2/bar"
				vp.ModulePath = "pa.th/to/foo/v2"
				return vp
			}(),
			wantPkg: samplePackage(func(p *Package) {
				p.Path = "pa.th/to/foo/v2/bar"
				p.Suffix = "bar"
				p.Module.Path = "pa.th/to/foo/v2"
			}),
		},
		{
			label: "explicit v1 command",
			pkg: func() *internal.VersionedPackage {
				vp := sample.VersionedPackage()
				vp.Name = "main"
				vp.Path = "pa.th/to/foo/v1"
				vp.ModulePath = "pa.th/to/foo/v1"
				return vp
			}(),
			wantPkg: samplePackage(func(p *Package) {
				p.Path = "pa.th/to/foo/v1"
				p.Suffix = "foo (root)"
				p.Module.Path = "pa.th/to/foo/v1"
			}),
		},
	} {

		t.Run(tc.label, func(t *testing.T) {
			got, err := createPackage(&tc.pkg.Package, &tc.pkg.VersionInfo)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantPkg, got); diff != "" {
				t.Errorf("createPackage(%v) mismatch (-want +got):\n%s", tc.pkg, diff)
			}
		})
	}
}

func TestParseModulePathAndVersion(t *testing.T) {
	testCases := []struct {
		name        string
		url         string
		wantModule  string
		wantVersion string
		wantErr     bool
	}{
		{
			name:        "valid_url",
			url:         "https://discovery.com/test.module@v1.0.0",
			wantModule:  "test.module",
			wantVersion: "v1.0.0",
		},
		{
			name:        "valid_url_with_tab",
			url:         "https://discovery.com/test.module@v1.0.0?tab=docs",
			wantModule:  "test.module",
			wantVersion: "v1.0.0",
		},
		{
			name:        "valid_url_missing_version",
			url:         "https://discovery.com/module",
			wantModule:  "module",
			wantVersion: internal.LatestVersion,
		},
		{
			name:    "invalid_url",
			url:     "https://discovery.com/",
			wantErr: true,
		},
		{
			name:    "invalid_url_missing_module",
			url:     "https://discovery.com@v1.0.0",
			wantErr: true,
		},
		{
			name:        "invalid_version",
			url:         "https://discovery.com/module@v1.0.0invalid",
			wantModule:  "module",
			wantVersion: "v1.0.0invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, parseErr := url.Parse(tc.url)
			if parseErr != nil {
				t.Errorf("url.Parse(%q): %v", tc.url, parseErr)
			}

			gotModule, gotVersion, err := parseModulePathAndVersion(u.Path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseModulePathAndVersion(%v) error = (%v); want error %t)", u, err, tc.wantErr)
			}
			if !tc.wantErr && (tc.wantModule != gotModule || tc.wantVersion != gotVersion) {
				t.Fatalf("parseModulePathAndVersion(%v): %q, %q, %v; want = %q, %q, want err %t",
					u, gotModule, gotVersion, err, tc.wantModule, tc.wantVersion, tc.wantErr)
			}
		})
	}
}
