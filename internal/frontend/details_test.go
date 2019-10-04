// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/sample"
	"golang.org/x/discovery/internal/stdlib"
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

// firstVersionedPackage is a helper function that returns an
// *internal.VersionedPackage corresponding to the first package in the
// version.
func firstVersionedPackage(v *internal.Version) *internal.VersionedPackage {
	return &internal.VersionedPackage{
		Package:     *v.Packages[0],
		VersionInfo: v.VersionInfo,
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

func TestParsePkgPathModulePathAndVersion(t *testing.T) {
	testCases := []struct {
		name, url, wantModulePath, wantPkgPath, wantVersion string
		wantErr                                             bool
	}{
		{
			name:           "latest",
			url:            "/github.com/hashicorp/vault/api",
			wantModulePath: unknownModulePath,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    internal.LatestVersion,
		},
		{
			name:           "package at version in nested module",
			url:            "/github.com/hashicorp/vault/api@v1.0.3",
			wantModulePath: unknownModulePath,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "package at version in parent module",
			url:            "/github.com/hashicorp/vault@v1.0.3/api",
			wantModulePath: "github.com/hashicorp/vault",
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:           "package at version trailing slash",
			url:            "/github.com/hashicorp/vault/api@v1.0.3/",
			wantModulePath: unknownModulePath,
			wantPkgPath:    "github.com/hashicorp/vault/api",
			wantVersion:    "v1.0.3",
		},
		{
			name:    "invalid url",
			url:     "/",
			wantErr: true,
		},
		{
			name:    "invalid url missing module",
			url:     "@v1.0.0",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u, parseErr := url.Parse(tc.url)
			if parseErr != nil {
				t.Errorf("url.Parse(%q): %v", tc.url, parseErr)
			}

			gotPkg, gotModule, gotVersion, err := parseDetailsURLPath(u.Path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseDetailsURLPath(%q) error = (%v); want error %t)", u, err, tc.wantErr)
			}
			if !tc.wantErr && (tc.wantModulePath != gotModule || tc.wantVersion != gotVersion || tc.wantPkgPath != gotPkg) {
				t.Fatalf("parseDetailsURLPath(%q): %q, %q, %q, %v; want = %q, %q, %q, want err %t",
					u, gotPkg, gotModule, gotVersion, err, tc.wantPkgPath, tc.wantModulePath, tc.wantVersion, tc.wantErr)
			}
		})
	}
}

func TestProcessPackageOrModulePath(t *testing.T) {
	for _, tc := range []struct {
		desc             string
		urlPath          string
		getErr1, getErr2 error

		wantPath, wantVersion string
		wantCode              int
	}{
		{
			desc:        "specific version found",
			urlPath:     "import/path@v1.2.3",
			wantPath:    "import/path",
			wantVersion: "v1.2.3",
			wantCode:    http.StatusOK,
		},
		{
			desc:        "latest version found",
			urlPath:     "import/path",
			wantPath:    "import/path",
			wantVersion: "latest",
			wantCode:    http.StatusOK,
		},
		{
			desc:        "version failed",
			urlPath:     "import/path@v1.2.3",
			getErr1:     context.Canceled,
			wantPath:    "",
			wantVersion: "",
			wantCode:    http.StatusInternalServerError,
		},
		{
			desc:        "version not found, latest found",
			urlPath:     "import/path@v1.2.3",
			getErr1:     derrors.NotFound,
			getErr2:     nil,
			wantPath:    "import/path",
			wantVersion: "v1.2.3",
			wantCode:    http.StatusSeeOther,
		},
		{
			desc:        "version not found, latest not found",
			urlPath:     "import/path@v1.2.3",
			getErr1:     derrors.NotFound,
			getErr2:     derrors.NotFound,
			wantPath:    "",
			wantVersion: "",
			wantCode:    http.StatusNotFound,
		},
		{
			desc:        "version not found, latest error",
			urlPath:     "import/path@v1.2.3",
			getErr1:     derrors.NotFound,
			getErr2:     context.Canceled,
			wantPath:    "",
			wantVersion: "",
			wantCode:    http.StatusNotFound,
		},
		{
			desc:        "excluded",
			urlPath:     "bad/path@v1.2.3",
			wantPath:    "",
			wantVersion: "",
			wantCode:    http.StatusNotFound,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ncalls := 0
			get := func(v string) error {
				ncalls++
				if ncalls == 1 {
					return tc.getErr1
				}
				return tc.getErr2
			}

			pkgPath, _, version, err := parseDetailsURLPath(tc.urlPath)
			if err != nil {
				t.Fatal(err)
			}
			gotCode, _ := fetchPackageOrModule(context.Background(), fakeDataSource{}, "pkg", pkgPath, version, get)
			if gotCode != tc.wantCode {
				t.Fatalf("got status code %d, want %d", gotCode, tc.wantCode)
			}
		})
	}
}

type fakeDataSource struct {
	DataSource
}

func (fakeDataSource) IsExcluded(_ context.Context, path string) (bool, error) {
	return strings.HasPrefix(path, "bad"), nil
}

func TestFileSource(t *testing.T) {
	for _, tc := range []struct {
		modulePath, version, filePath, want string
	}{
		{
			modulePath: sample.ModulePath,
			version:    sample.VersionString,
			filePath:   "LICENSE.txt",
			want:       fmt.Sprintf("%s@%s/%s", sample.ModulePath, sample.VersionString, "LICENSE.txt"),
		},
		{
			modulePath: stdlib.ModulePath,
			version:    "v1.13.0",
			filePath:   "README.md",
			want:       fmt.Sprintf("go.googlesource.com/go/+/refs/tags/%s/%s", "go1.13", "README.md"),
		},
		{
			modulePath: stdlib.ModulePath,
			version:    "v1.13.invalid",
			filePath:   "README.md",
			want:       fmt.Sprintf("go.googlesource.com/go/+/refs/heads/master/%s", "README.md"),
		},
	} {
		t.Run(fmt.Sprintf("%s@%s/%s", tc.modulePath, tc.version, tc.filePath), func(t *testing.T) {
			if got := fileSource(tc.modulePath, tc.version, tc.filePath); got != tc.want {
				t.Errorf("fileSource(%q, %q, %q) = %q; want = %q", tc.modulePath, tc.version, tc.filePath, got, tc.want)
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
