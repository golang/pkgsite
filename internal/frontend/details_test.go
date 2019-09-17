// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

func TestParsePathAndVersion(t *testing.T) {
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

			gotModule, gotVersion, err := parsePathAndVersion(u.Path, "pkg")
			if (err != nil) != tc.wantErr {
				t.Fatalf("parsePathAndVersion(%v) error = (%v); want error %t)", u, err, tc.wantErr)
			}
			if !tc.wantErr && (tc.wantModule != gotModule || tc.wantVersion != gotVersion) {
				t.Fatalf("parsePathAndVersion(%v): %q, %q, %v; want = %q, %q, want err %t",
					u, gotModule, gotVersion, err, tc.wantModule, tc.wantVersion, tc.wantErr)
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

			path, version, err := parsePathAndVersion(tc.urlPath, "pkg")
			if err != nil {
				t.Fatal(err)
			}
			gotCode, _ := fetchPackageOrModule("pkg", path, version, get)
			if gotCode != tc.wantCode {
				t.Fatalf("got status code %d, want %d", gotCode, tc.wantCode)
			}
		})
	}
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
