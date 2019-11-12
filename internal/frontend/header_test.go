// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/cascadia"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/net/html"
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
			got, err := createPackage(&tc.pkg.Package, &tc.pkg.VersionInfo, false)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantPkg, got); diff != "" {
				t.Errorf("createPackage(%v) mismatch (-want +got):\n%s", tc.pkg, diff)
			}
		})
	}
}

// none returns a validator that checks no elements matching selector exist.
func none(selector string) validator {
	sel := mustParseSelector(selector)
	return func(n *html.Node) error {
		if sel.Match(n) || cascadia.Query(n, sel) != nil {
			return fmt.Errorf("%q matched one or more elements", selector)
		}
		return nil
	}
}

func TestBreadcrumbPath(t *testing.T) {
	for _, test := range []struct {
		pkgPath, modPath, version string
		want                      validator
	}{
		{
			"example.com/blob/s3blob", "example.com", internal.LatestVersion,
			in("",
				inAt("a", 0, href("/example.com"), text("example.com")),
				inAt("a", 1, href("/example.com/blob"), text("blob")),
				in("span.DetailsHeader-breadcrumbCurrent", text("s3blob"))),
		},
		{
			"example.com", "example.com", internal.LatestVersion,
			in("",
				none("a"),
				in("span.DetailsHeader-breadcrumbCurrent", text("example.com"))),
		},

		{
			"g/x/tools/go/a", "g/x/tools", internal.LatestVersion,
			in("",
				inAt("a", 0, href("/g/x/tools"), text("g/x/tools")),
				inAt("a", 1, href("/g/x/tools/go"), text("go")),
				in("span.DetailsHeader-breadcrumbCurrent", text("a"))),
		},
		{
			"golang.org/x/tools", "golang.org/x/tools", internal.LatestVersion,
			in("",
				none("a"),
				in("span.DetailsHeader-breadcrumbCurrent", text("golang.org/x/tools"))),
		},
		{
			// Special case: stdlib.
			"encoding/json", "std", internal.LatestVersion,
			in("",
				in("a", href("/encoding"), text("encoding")),
				in("span.DetailsHeader-breadcrumbCurrent", text("json"))),
		},
		{
			"example.com/blob/s3blob", "example.com", "v1",
			in("",
				inAt("a", 0, href("/example.com@v1"), text("example.com")),
				inAt("a", 1, href("/example.com/blob@v1"), text("blob")),
				in("span.DetailsHeader-breadcrumbCurrent", text("s3blob"))),
		},
	} {
		t.Run(fmt.Sprintf("%s-%s-%s", test.pkgPath, test.modPath, test.version), func(t *testing.T) {
			want := in("div.DetailsHeader-breadcrumb",
				test.want,
				in("button#DetailsHeader-copyPath",
					attr("aria-label", "Copy path to clipboard"),
					in("svg > title", text("Copy path to clipboard"))),
				in("input#DetailsHeader-path",
					attr("role", "presentation"),
					attr("tabindex", "-1"),
					attr("value", test.pkgPath)))

			got := breadcrumbPath(test.pkgPath, test.modPath, test.version)
			doc, err := html.Parse(strings.NewReader(string(got)))
			if err != nil {
				t.Fatal(err)
			}
			if err := want(doc); err != nil {
				t.Error(err)
			}
		})
	}
}
