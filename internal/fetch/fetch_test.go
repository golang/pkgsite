// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fetch

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
)

const testTimeout = 5 * time.Second

func TestFetchAndInsertVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testCases := []struct {
		modulePath  string
		version     string
		versionData *internal.Version
		pkg         string
		pkgData     *internal.Package
	}{
		{
			modulePath: "my.mod/module",
			version:    "v1.0.0",
			versionData: &internal.Version{
				Module: &internal.Module{
					Path: "my.mod/module",
				},
				Version:    "v1.0.0",
				CommitTime: time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
				ReadMe:     []byte("README FILE FOR TESTING."),
			},
			pkg: "my.mod/module/bar",
			pkgData: &internal.Package{
				Path:     "my.mod/module/bar",
				Name:     "bar",
				Synopsis: "package bar",
				Licenses: []*internal.LicenseInfo{
					{Type: "BSD-3-Clause", FilePath: "my.mod/module@v1.0.0/LICENSE"},
					{Type: "MIT", FilePath: "my.mod/module@v1.0.0/bar/LICENSE"},
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.modulePath, func(t *testing.T) {
			teardownDB, db := postgres.SetupCleanDB(t)
			defer teardownDB(t)

			teardownProxy, client := proxy.SetupTestProxy(ctx, t)
			defer teardownProxy(t)

			if err := FetchAndInsertVersion(test.modulePath, test.version, client, db); err != nil {
				t.Fatalf("FetchAndInsertVersion(%q, %q, %v, %v): %v", test.modulePath, test.version, client, db, err)
			}

			dbVersion, err := db.GetVersion(ctx, test.modulePath, test.version)
			if err != nil {
				t.Fatalf("db.GetVersion(ctx, %q, %q): %v", test.modulePath, test.version, err)
			}

			// create a clone of dbVersion, as we want to use it for package testing later.
			got := *dbVersion
			// Set CreatedAt and UpdatedAt to nil for testing, since these are
			// set by the database.
			got.CreatedAt = time.Time{}
			got.UpdatedAt = time.Time{}

			// got.CommitTime has a timezone location of +0000, while
			// test.versionData.CommitTime has a timezone location of UTC.
			// These are equal according to time.Equal, but fail for
			// reflect.DeepEqual. Convert the DB time to UTC.
			got.CommitTime = got.CommitTime.UTC()

			if diff := cmp.Diff(got, *test.versionData); diff != "" {
				t.Errorf("db.GetVersion(ctx, %q, %q) mismatch (-got +want):\n%s", test.modulePath, test.version, diff)
			}

			test.pkgData.Version = dbVersion
			// TODO(b/130367504): this shouldn't be necessary
			test.pkgData.Version.Synopsis = test.pkgData.Synopsis
			gotPkg, err := db.GetPackage(ctx, test.pkg, test.version)
			if err != nil {
				t.Fatalf("db.GetPackage(ctx, %q, %q): %v", test.pkg, test.version, err)
			}

			sort.Slice(gotPkg.Licenses, func(i, j int) bool {
				return gotPkg.Licenses[i].Type < gotPkg.Licenses[j].Type
			})
			if diff := cmp.Diff(gotPkg, test.pkgData); diff != "" {
				t.Errorf("db.GetPackage(ctx, %q, %q) mismatch (-got +want):\n%s", test.pkg, test.version, diff)
			}
		})
	}
}

func TestFetchAndInsertVersionTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	teardownDB, db := postgres.SetupCleanDB(t)
	defer teardownDB(t)

	defer func(oldTimeout time.Duration) {
		fetchTimeout = oldTimeout
	}(fetchTimeout)
	fetchTimeout = 0

	teardownProxy, client := proxy.SetupTestProxy(ctx, t)
	defer teardownProxy(t)

	name := "my.mod/version"
	version := "v1.0.0"
	wantErrString := "deadline exceeded"
	err := FetchAndInsertVersion(name, version, client, db)
	if err == nil || !strings.Contains(err.Error(), wantErrString) {
		t.Fatalf("FetchAndInsertVersion(%q, %q, %v, %v) returned error %v, want error containing %q",
			name, version, client, db, err, wantErrString)
	}
}

func TestParseModulePathAndVersion(t *testing.T) {
	testCases := []struct {
		name    string
		url     string
		module  string
		version string
		err     error
	}{
		{
			name:    "ValidFetchURL",
			url:     "https://proxy.com/module/@v/v1.0.0",
			module:  "module",
			version: "v1.0.0",
			err:     nil,
		},
		{
			name: "InvalidFetchURL",
			url:  "https://proxy.com/",
			err:  errors.New(`invalid path: "/"`),
		},
		{
			name: "InvalidFetchURLNoModule",
			url:  "https://proxy.com/@v/version",
			err:  errors.New(`invalid path: "/@v/version"`),
		},
		{
			name: "InvalidFetchURLNoVersion",
			url:  "https://proxy.com/module/@v/",
			err:  errors.New(`invalid path: "/module/@v/"`),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse(test.url)
			if err != nil {
				t.Errorf("url.Parse(%q): %v", test.url, err)
			}

			m, v, err := ParseModulePathAndVersion(u.Path)
			if test.err != nil {
				if err == nil {
					t.Fatalf("ParseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				}
				if test.err.Error() != err.Error() {
					t.Fatalf("ParseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				} else {
					return
				}
			} else if err != nil {
				t.Fatalf("ParseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
			}

			if test.module != m || test.version != v {
				t.Fatalf("ParseModulePathAndVersion(%v): %q, %q, %v; want = %q, %q, %v",
					u, m, v, err, test.module, test.version, test.err)
			}
		})
	}
}

func TestHasFilename(t *testing.T) {
	for _, test := range []struct {
		file         string
		expectedFile string
		want         bool
	}{
		{
			file:         "my.mod/module@v1.0.0/README.md",
			expectedFile: "README.md",
			want:         true,
		},
		{
			file:         "rEaDme",
			expectedFile: "README",
			want:         true,
		}, {
			file:         "README.FOO",
			expectedFile: "README",
			want:         true,
		},
		{
			file:         "FOO_README",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "README_FOO",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "README.FOO.FOO",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "",
			expectedFile: "README",
			want:         false,
		},
		{
			file:         "my.mod/module@v1.0.0/LICENSE",
			expectedFile: "my.mod/module@v1.0.0/LICENSE",
			want:         true,
		},
	} {
		{
			t.Run(test.file, func(t *testing.T) {
				got := hasFilename(test.file, test.expectedFile)
				if got != test.want {
					t.Errorf("hasFilename(%q, %q) = %t: %t", test.file, test.expectedFile, got, test.want)
				}
			})
		}
	}

}

func TestContainsFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, test := range []struct {
		name, version, file string
		want                bool
	}{
		{
			name:    "my.mod/module",
			version: "v1.0.0",
			file:    "README",
			want:    true,
		},
		{
			name:    "my.mod/module",
			version: "v1.0.0",
			file:    "my.mod/module@v1.0.0/LICENSE",
			want:    true,
		},
		{
			name:    "emp.ty/module",
			version: "v1.0.0",
			file:    "README",
			want:    false,
		},
		{
			name:    "emp.ty/module",
			version: "v1.0.0",
			file:    "LICENSE",
			want:    false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(ctx, t)
			defer teardownProxy(t)

			reader, err := client.GetZip(ctx, test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(ctx, %q %q): %v", test.name, test.version, err)
			}

			if got := containsFile(reader, test.file); got != test.want {
				t.Errorf("containsFile(%q, %q) = %t, want %t", fmt.Sprintf("%s %s", test.name, test.version), test.file, got, test.want)
			}
		})
	}
}

func TestExtractFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, test := range []struct {
		name, version, file string
		want                []byte
		err                 error
	}{
		{
			name:    "my.mod/module",
			version: "v1.0.0",
			file:    "my.mod/module@v1.0.0/README.md",
			want:    []byte("README FILE FOR TESTING."),
		},
		{
			name:    "emp.ty/module",
			version: "v1.0.0",
			file:    "empty/nonexistent/README.md",
			err:     errors.New(`zip does not contain "README.md"`),
		},
	} {
		t.Run(test.file, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(ctx, t)
			defer teardownProxy(t)

			reader, err := client.GetZip(ctx, test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(ctx, %q, %q): %v", test.name, test.version, err)
			}

			got, err := extractFile(reader, filepath.Base(test.file))
			if err != nil {
				if test.err == nil || test.err.Error() != err.Error() {
					t.Errorf("extractFile(%q, %q): \n %v, want \n %v",
						fmt.Sprintf("%q %q", test.name, test.version), filepath.Base(test.file), err, test.err)
				} else {
					return
				}
			}

			if !bytes.Equal(test.want, got) {
				t.Errorf("extractFile(%q, %q) = %q, want %q", test.name, test.file, got, test.want)
			}
		})
	}
}

func TestExtractPackagesFromZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, test := range []struct {
		zip      string
		name     string
		version  string
		packages map[string]*internal.Package
	}{
		{
			zip:     "module.zip",
			name:    "my.mod/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"foo": &internal.Package{
					Name:     "foo",
					Path:     "my.mod/module/foo",
					Synopsis: "package foo",
					Imports: []*internal.Import{
						&internal.Import{
							Name: "fmt",
							Path: "fmt",
						},
						&internal.Import{
							Name: "bar",
							Path: "my.mod/module/bar",
						},
					},
					Suffix: "foo",
				},
				"bar": &internal.Package{
					Name:     "bar",
					Path:     "my.mod/module/bar",
					Synopsis: "package bar",
					Suffix:   "bar",
				},
			},
		},
		{
			name:     "emp.ty/module",
			version:  "v1.0.0",
			packages: map[string]*internal.Package{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(ctx, t)
			defer teardownProxy(t)

			reader, err := client.GetZip(ctx, test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(ctx, %q %q): %v", test.name, test.version, err)
			}

			packages, err := extractPackagesFromZip(test.name, test.version, reader, nil)
			if err != nil && len(test.packages) != 0 {
				t.Fatalf("extractPackagesFromZip(%q, %q): %v", test.name, test.zip, err)
			}

			for _, got := range packages {
				want, ok := test.packages[got.Name]
				if !ok {
					t.Errorf("extractPackagesFromZip(%q, %q) returned unexpected package: %q", test.name, test.zip, got.Name)
				}

				sort.Slice(got.Imports, func(i, j int) bool {
					return got.Imports[i].Path < got.Imports[j].Path
				})

				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("extractPackagesFromZip(%q, %q, reader, nil) mismatch (-want +got):\n%s", test.name, test.version, diff)
				}
			}
		})
	}
}

func TestDetectLicenses(t *testing.T) {
	makeLicenses := func(licType, licFile string) []*internal.LicenseInfo {
		return []*internal.LicenseInfo{{Type: licType, FilePath: licFile}}
	}
	testCases := []struct {
		name, zipName string
		want          []*internal.LicenseInfo
	}{
		{
			name:    "valid_license",
			zipName: "license",
			want:    makeLicenses("MIT", "rsc.io/quote@v1.4.1/LICENSE"),
		}, {
			name:    "valid_license_md_format",
			zipName: "licensemd",
			want:    makeLicenses("MIT", "rsc.io/quote@v1.4.1/LICENSE.md"),
		},
		{
			name:    "valid_license_copying",
			zipName: "copying",
			want:    makeLicenses("Apache-2.0", "golang.org/x/text@v0.0.3/COPYING"),
		}, {
			name:    "valid_license_copying_md",
			zipName: "copyingmd",
			want:    makeLicenses("Apache-2.0", "golang.org/x/text@v0.0.3/COPYING.md"),
		}, {
			name:    "multiple_licenses",
			zipName: "multiplelicenses",
			want: []*internal.LicenseInfo{
				{Type: "MIT", FilePath: "rsc.io/quote@v1.4.1/LICENSE"},
				{Type: "MIT", FilePath: "rsc.io/quote@v1.4.1/bar/LICENSE.md"},
				{Type: "Apache-2.0", FilePath: "rsc.io/quote@v1.4.1/foo/COPYING"},
				{Type: "Apache-2.0", FilePath: "rsc.io/quote@v1.4.1/foo/COPYING.md"},
			},
		}, {
			name:    "low_coverage_license",
			zipName: "lowcoveragelicenses",
		}, {
			name:    "no_license",
			zipName: "nolicense",
		}, {
			name:    "no_license",
			zipName: "nolicense",
		}, {
			name:    "vendor_license_should_ignore",
			zipName: "vendorlicense",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			testDir := filepath.Join("testdata/licenses", test.zipName)
			cleanUpZip, err := proxy.ZipFiles(testDir+".zip", testDir, "")
			defer cleanUpZip()
			if err != nil {
				t.Fatalf("proxy.ZipFiles(%q): %v", test.zipName, err)
			}

			if _, err := os.Stat(testDir + ".zip"); err != nil {
				t.Fatalf("os.Stat(%q): %v", testDir+".zip", err)
			}

			rc, err := zip.OpenReader(testDir + ".zip")
			if err != nil {
				t.Fatalf("zip.OpenReader(%q): %v", test.zipName, err)
			}
			defer rc.Close()
			z := &rc.Reader

			got, err := detectLicenses(z)
			if err != nil {
				t.Errorf("detectLicenses(z): %v", err)
			}
			var gotFiles []*internal.LicenseInfo
			for _, l := range got {
				gotFiles = append(gotFiles, &l.LicenseInfo)
			}
			if diff := cmp.Diff(gotFiles, test.want); diff != "" {
				t.Errorf("detectLicense(z) mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestFetch_parseVersion(t *testing.T) {
	testCases := []struct {
		name, version   string
		wantVersionType internal.VersionType
		wantErr         bool
	}{
		{
			name:            "valid_pseudo-version",
			version:         "v1.0.0-20190311183353-d8887717615a",
			wantVersionType: internal.VersionTypePseudo,
		},
		{
			name:            "invalid_pseudo-version_future_date",
			version:         "v1.0.0-40000311183353-d8887717615a",
			wantVersionType: internal.VersionTypePrerelease,
		},
		{
			name:            "valid_release",
			version:         "v1.0.0",
			wantVersionType: internal.VersionTypeRelease,
		},
		{
			name:            "valid_release",
			version:         "v1.0.0-alpha.1",
			wantVersionType: internal.VersionTypePrerelease,
		},
		{
			name:            "invalid_version",
			version:         "not_a_version",
			wantVersionType: "",
			wantErr:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if gotVt, err := parseVersion(tc.version); (tc.wantErr == (err != nil)) && tc.wantVersionType != gotVt {
				t.Errorf("parseVersion(%v) = %v, want %v", tc.version, gotVt, tc.wantVersionType)
			}
		})
	}
}
