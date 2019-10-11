// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
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
			IsRedistributable: true,
			Licenses:          transformLicenseMetadata(sample.LicenseMetadata),
		},
	}
	for _, mut := range mutators {
		mut(p)
	}
	p.URL = constructPackageURL(p.Path, p.Module.Path, p.Version)
	p.Module.URL = constructModuleURL(p.Module.Path, p.Version)
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

func TestBreadcrumbPath(t *testing.T) {
	for _, test := range []struct {
		pkgPath, modPath, version string
		want                      string
	}{
		{
			"example.com/blob/s3blob", "example.com", internal.LatestVersion,
			`<a href="/example.com">example.com</a><span class="DetailsHeader-breadcrumbDivider">/</span><a href="/example.com/blob">blob</a><span class="DetailsHeader-breadcrumbDivider">/</span><span class="DetailsHeader-breadcrumbCurrent">s3blob</span>`,
		},
		{
			"example.com", "example.com", internal.LatestVersion,
			`<span class="DetailsHeader-breadcrumbCurrent">example.com</span>`,
		},

		{
			"g/x/tools/go/a", "g/x/tools", internal.LatestVersion,
			`<a href="/g/x/tools">g/x/tools</a><span class="DetailsHeader-breadcrumbDivider">/</span><a href="/g/x/tools/go">go</a><span class="DetailsHeader-breadcrumbDivider">/</span><span class="DetailsHeader-breadcrumbCurrent">a</span>`,
		},
		{
			"golang.org/x/tools", "golang.org/x/tools", internal.LatestVersion,
			`<span class="DetailsHeader-breadcrumbCurrent">golang.org/x/tools</span>`,
		},
		{
			// Special case: stdlib.
			"encoding/json", "std", internal.LatestVersion,
			`<a href="/encoding">encoding</a><span class="DetailsHeader-breadcrumbDivider">/</span><span class="DetailsHeader-breadcrumbCurrent">json</span>`,
		},
		{
			"example.com/blob/s3blob", "example.com", "v1",
			`<a href="/example.com@v1">example.com</a><span class="DetailsHeader-breadcrumbDivider">/</span><a href="/example.com/blob@v1">blob</a><span class="DetailsHeader-breadcrumbDivider">/</span><span class="DetailsHeader-breadcrumbCurrent">s3blob</span>`,
		},
	} {
		t.Run(fmt.Sprintf("%s-%s-%s", test.pkgPath, test.modPath, test.version), func(t *testing.T) {
			got := breadcrumbPath(test.pkgPath, test.modPath, test.version)
			want := `<div class="DetailsHeader-breadcrumb">` + test.want + `</div>`
			if string(got) != want {
				t.Errorf("got:\n%s\n\nwant:\n%s\n", got, want)
			}
		})
	}
}
