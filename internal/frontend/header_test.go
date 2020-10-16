// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/middleware"
	"golang.org/x/pkgsite/internal/testing/sample"
)

func samplePackage(mutators ...func(*Package)) *Package {
	p := &Package{
		Path:              sample.PackagePath,
		IsRedistributable: true,
		Licenses:          transformLicenseMetadata(sample.LicenseMetadata),
		Module: Module{
			DisplayVersion:    sample.VersionString,
			LinkVersion:       sample.VersionString,
			CommitTime:        "0 hours ago",
			ModulePath:        sample.ModulePath,
			IsRedistributable: true,
			Licenses:          transformLicenseMetadata(sample.LicenseMetadata),
		},
	}
	for _, mut := range mutators {
		mut(p)
	}
	p.URL = constructPackageURL(p.Path, p.ModulePath, p.LinkVersion)
	p.Module.URL = constructModuleURL(p.ModulePath, p.LinkVersion)
	p.LatestURL = constructPackageURL(p.Path, p.ModulePath, middleware.LatestMinorVersionPlaceholder)
	p.Module.LatestURL = constructModuleURL(p.ModulePath, middleware.LatestMinorVersionPlaceholder)
	p.Module.LinkVersion = linkVersion(sample.VersionString, sample.ModulePath)
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

func TestCreatePackage(t *testing.T) {

	for _, test := range []struct {
		label       string
		pkg         *internal.PackageMeta
		mi          *internal.ModuleInfo
		linkVersion bool
		wantPkg     *Package
	}{
		{
			label:       "simple package",
			pkg:         sample.PackageMeta(sample.ModulePath + "/" + sample.Suffix),
			mi:          sample.ModuleInfo(sample.ModulePath, sample.VersionString),
			linkVersion: false,
			wantPkg:     samplePackage(),
		},
		{
			label:       "simple package, latest",
			pkg:         sample.PackageMeta(sample.ModulePath + "/" + sample.Suffix),
			mi:          sample.ModuleInfo(sample.ModulePath, sample.VersionString),
			linkVersion: true,
			wantPkg: samplePackage(func(p *Package) {
				p.LinkVersion = internal.LatestVersion
			}),
		},
		{
			label: "command package",
			pkg: func() *internal.PackageMeta {
				pm := sample.PackageMeta(sample.ModulePath + "/" + sample.Suffix)
				pm.Name = "main"
				return pm
			}(),
			mi:          sample.ModuleInfo(sample.ModulePath, sample.VersionString),
			linkVersion: false,
			wantPkg:     samplePackage(),
		},
		{
			label: "v2 command",
			pkg: func() *internal.PackageMeta {
				pm := sample.PackageMeta("pa.th/to/foo/v2/bar")
				pm.Name = "main"
				return pm
			}(),
			mi:          sample.ModuleInfo("pa.th/to/foo/v2", sample.VersionString),
			linkVersion: false,
			wantPkg: samplePackage(func(p *Package) {
				p.Path = "pa.th/to/foo/v2/bar"
				p.ModulePath = "pa.th/to/foo/v2"
			}),
		},
		{
			label: "explicit v1 command",
			pkg: func() *internal.PackageMeta {
				pm := sample.PackageMeta("pa.th/to/foo/v1")
				pm.Name = "main"
				return pm
			}(),
			mi:          sample.ModuleInfo("pa.th/to/foo/v1", sample.VersionString),
			linkVersion: false,
			wantPkg: samplePackage(func(p *Package) {
				p.Path = "pa.th/to/foo/v1"
				p.ModulePath = "pa.th/to/foo/v1"
			}),
		},
	} {
		t.Run(test.label, func(t *testing.T) {
			pm := sample.PackageMeta(test.pkg.Path)
			got, err := createPackage(pm, test.mi, test.linkVersion)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.wantPkg, got, cmp.AllowUnexported(safehtml.Identifier{})); diff != "" {
				t.Errorf("createPackage(%v) mismatch (-want +got):\n%s", test.pkg, diff)
			}
		})
	}
}
