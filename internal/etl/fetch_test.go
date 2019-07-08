// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/discovery/internal"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/dzip"
	"golang.org/x/discovery/internal/license"
	"golang.org/x/discovery/internal/postgres"
	"golang.org/x/discovery/internal/proxy"
	"golang.org/x/discovery/internal/testhelper"
)

func TestSkipBadPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)
	badModule := map[string]string{
		"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
		"README.md":  "This is a readme",
		"LICENSE":    testhelper.MITLicense,
	}
	var bigFile strings.Builder
	bigFile.WriteString("package bar\n")
	bigFile.WriteString("const Bar = 123\n")
	for bigFile.Len() < int(dzip.MaxFileSize) {
		bigFile.WriteString("// All work and no play makes Jack a dull boy.\n")
	}
	badModule["bar/bar.go"] = bigFile.String()
	var (
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
	)
	teardownProxy, client := proxy.SetupTestProxy(t, []*proxy.TestVersion{
		proxy.NewTestVersion(t, modulePath, version, badModule),
	})
	defer teardownProxy(t)

	if err := fetchAndInsertVersion(ctx, modulePath, version, client, testDB); err != nil {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v): %v", modulePath, version, client, testDB, err)
	}

	pkgFoo := modulePath + "/foo"
	if _, err := testDB.GetPackage(ctx, pkgFoo, version); err != nil {
		t.Errorf("testDB.GetPackage(ctx, %q, %q): %v, want nil", pkgFoo, version, err)
	}
	pkgBar := modulePath + "/bar"
	if _, err := testDB.GetPackage(ctx, pkgBar, version); !derrors.IsNotFound(err) {
		t.Errorf("testDB.GetPackage(ctx, %q, %q): %v, want NotFound", pkgBar, version, err)
	}
}

func TestFetch_V1Path(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)
	tearDown, client := proxy.SetupTestProxy(t, []*proxy.TestVersion{
		proxy.NewTestVersion(t, "my.mod/foo", "v1.0.0", map[string]string{
			"foo.go":  "package foo\nconst Foo = 41",
			"LICENSE": testhelper.MITLicense,
		}),
	})
	defer tearDown(t)
	if err := fetchAndInsertVersion(ctx, "my.mod/foo", "v1.0.0", client, testDB); err != nil {
		t.Fatalf("fetchAndInsertVersion: %v", err)
	}
	pkg, err := testDB.GetPackage(ctx, "my.mod/foo", "v1.0.0")
	if err != nil {
		t.Fatalf("GetPackage: %v", err)
	}
	if got, want := pkg.V1Path, "my.mod/foo"; got != want {
		t.Errorf("V1Path = %q, want %q", got, want)
	}
}

func TestReFetch(t *testing.T) {
	// This test checks that re-fetching a version will cause its data to be
	// overwritten.  This is achieved by fetching against two different versions
	// of the (fake) proxy, though in reality the most likely cause of changes to
	// a version is updates to our data model or fetch logic.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	defer postgres.ResetTestDB(testDB, t)

	var (
		modulePath = "github.com/my/module"
		version    = "v1.0.0"
		pkgFoo     = "github.com/my/module/foo"
		pkgBar     = "github.com/my/module/bar"
		foo        = map[string]string{
			"foo/foo.go": "// Package foo\npackage foo\n\nconst Foo = 42",
			"README.md":  "This is a readme",
			"LICENSE":    testhelper.MITLicense,
		}
		bar = map[string]string{
			"bar/bar.go": "// Package bar\npackage bar\n\nconst Bar = 21",
			"README.md":  "This is another readme",
			"COPYING":    testhelper.MITLicense,
		}
	)

	// First fetch and insert a version containing package foo, and verify that
	// foo can be retrieved.
	teardownProxy, client := proxy.SetupTestProxy(t, []*proxy.TestVersion{
		proxy.NewTestVersion(t, modulePath, version, foo),
	})
	defer teardownProxy(t)
	if err := fetchAndInsertVersion(ctx, modulePath, version, client, testDB); err != nil {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v): %v", modulePath, version, client, testDB, err)
	}
	if _, err := testDB.GetPackage(ctx, pkgFoo, version); err != nil {
		t.Errorf("testDB.GetPackage(ctx, %q, %q): %v", pkgFoo, version, err)
	}

	// Now re-fetch and verify that contents were overwritten.
	teardownProxy, client = proxy.SetupTestProxy(t, []*proxy.TestVersion{
		proxy.NewTestVersion(t, modulePath, version, bar),
	})
	defer teardownProxy(t)

	if err := fetchAndInsertVersion(ctx, modulePath, version, client, testDB); err != nil {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v): %v", modulePath, version, client, testDB, err)
	}
	want := &internal.VersionedPackage{
		VersionInfo: internal.VersionInfo{
			ModulePath:     modulePath,
			Version:        version,
			CommitTime:     time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
			ReadmeFilePath: "README.md",
			ReadmeContents: []byte("This is another readme"),
			VersionType:    "release",
		},
		Package: internal.Package{
			Path:              "github.com/my/module/bar",
			Name:              "bar",
			Synopsis:          "Package bar",
			DocumentationHTML: []byte("Bar returns the string &#34;bar&#34;."),
			V1Path:            "github.com/my/module/bar",
			Licenses: []*license.Metadata{
				{Types: []string{"MIT"}, FilePath: "COPYING"},
			},
		},
	}
	got, err := testDB.GetPackage(ctx, pkgBar, version)
	if err != nil {
		t.Fatalf("testDB.GetPackage(ctx, %q, %q): %v", pkgBar, version, err)
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML")); diff != "" {
		t.Errorf("testDB.GetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", pkgBar, version, diff)
	}

	// For good measure, verify that package foo is now NotFound.
	if _, err := testDB.GetPackage(ctx, pkgFoo, version); !derrors.IsNotFound(err) {
		t.Errorf("testDB.GetPackage(ctx, %q, %q): %v, want NotFound", pkgFoo, version, err)
	}
}

func TestFetchAndInsertVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	testCases := []struct {
		modulePath string
		version    string
		pkg        string
		want       *internal.VersionedPackage
	}{
		{
			modulePath: "github.com/my/module",
			version:    "v1.0.0",
			pkg:        "github.com/my/module/bar",
			want: &internal.VersionedPackage{
				VersionInfo: internal.VersionInfo{
					ModulePath:     "github.com/my/module",
					Version:        "v1.0.0",
					CommitTime:     time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
					ReadmeFilePath: "README.md",
					ReadmeContents: []byte("README FILE FOR TESTING."),
					VersionType:    "release",
				},
				Package: internal.Package{
					Path:              "github.com/my/module/bar",
					Name:              "bar",
					Synopsis:          "package bar",
					DocumentationHTML: []byte("Bar returns the string &#34;bar&#34;."),
					V1Path:            "github.com/my/module/bar",
					Licenses: []*license.Metadata{
						{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
					},
				},
			},
		}, {
			modulePath: "nonredistributable.mod/module",
			version:    "v1.0.0",
			pkg:        "nonredistributable.mod/module/bar/baz",
			want: &internal.VersionedPackage{
				VersionInfo: internal.VersionInfo{
					ModulePath:     "nonredistributable.mod/module",
					Version:        "v1.0.0",
					CommitTime:     time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
					ReadmeFilePath: "README.md",
					ReadmeContents: []byte("README FILE FOR TESTING."),
					VersionType:    "release",
				},
				Package: internal.Package{
					Path:              "nonredistributable.mod/module/bar/baz",
					Name:              "baz",
					Synopsis:          "package baz",
					DocumentationHTML: []byte("Baz returns the string &#34;baz&#34;."),
					V1Path:            "nonredistributable.mod/module/bar/baz",
					Licenses: []*license.Metadata{
						{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/LICENSE"},
						{Types: []string{"MIT"}, FilePath: "bar/baz/COPYING"},
					},
				},
			},
		}, {
			modulePath: "nonredistributable.mod/module",
			version:    "v1.0.0",
			pkg:        "nonredistributable.mod/module/foo",
			want: &internal.VersionedPackage{
				VersionInfo: internal.VersionInfo{
					ModulePath:     "nonredistributable.mod/module",
					Version:        "v1.0.0",
					CommitTime:     time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
					ReadmeFilePath: "README.md",
					ReadmeContents: []byte("README FILE FOR TESTING."),
					VersionType:    "release",
				},
				Package: internal.Package{
					Path:              "nonredistributable.mod/module/foo",
					Name:              "foo",
					Synopsis:          "",
					DocumentationHTML: nil,
					V1Path:            "nonredistributable.mod/module/foo",
					Licenses: []*license.Metadata{
						{Types: []string{"BSD-3-Clause"}, FilePath: "LICENSE"},
						{Types: []string{"BSD-0-Clause"}, FilePath: "foo/LICENSE.md"},
					},
				},
			},
		}, {
			modulePath: "std",
			version:    "v1.12.5",
			pkg:        "context",
			want: &internal.VersionedPackage{
				VersionInfo: internal.VersionInfo{
					ModulePath:     "std",
					Version:        "v1.12.5",
					CommitTime:     time.Date(2019, 1, 30, 0, 0, 0, 0, time.UTC),
					VersionType:    "release",
					ReadmeContents: []uint8{},
				},
				Package: internal.Package{
					Path:              "context",
					Name:              "context",
					Synopsis:          "Package context defines the Context type, which carries deadlines, cancelation signals, and other request-scoped values across API boundaries and between processes.",
					DocumentationHTML: nil,
					V1Path:            "context",
					Licenses: []*license.Metadata{
						{
							Types:    []string{"BSD-3-Clause"},
							FilePath: "LICENSE",
						},
					},
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.pkg, func(t *testing.T) {
			defer postgres.ResetTestDB(testDB, t)

			teardownProxy, client := proxy.SetupTestProxy(t, nil)
			defer teardownProxy(t)

			if err := fetchAndInsertVersion(ctx, test.modulePath, test.version, client, testDB); err != nil {
				t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v): %v", test.modulePath, test.version, client, testDB, err)
			}

			gotVersion, err := testDB.GetVersion(ctx, test.modulePath, test.version)
			if err != nil {
				t.Fatalf("testDB.GetVersion(ctx, %q, %q): %v", test.modulePath, test.version, err)
			}
			if diff := cmp.Diff(test.want.VersionInfo, *gotVersion); diff != "" {
				t.Errorf("testDB.GetVersion(ctx, %q, %q) mismatch (-want +got):\n%s", test.modulePath, test.version, diff)
			}

			gotPkg, err := testDB.GetPackage(ctx, test.pkg, test.version)
			if err != nil {
				t.Fatalf("testDB.GetPackage(ctx, %q, %q): %v", test.pkg, test.version, err)
			}

			sort.Slice(gotPkg.Licenses, func(i, j int) bool {
				return gotPkg.Licenses[i].FilePath < gotPkg.Licenses[j].FilePath
			})
			if diff := cmp.Diff(test.want, gotPkg, cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML")); diff != "" {
				t.Errorf("testDB.GetPackage(ctx, %q, %q) mismatch (-want +got):\n%s", test.pkg, test.version, diff)
			}
			if test.want.ModulePath == "std" {
				// Do not validate documentation packages in
				// the std module because it is very large.
				return
			}
			if got, want := gotPkg.DocumentationHTML, test.want.DocumentationHTML; len(want) == 0 && len(got) != 0 {
				t.Errorf("got non-empty documentation but want empty:\ngot: %q\nwant: %q", got, want)
			} else if !bytes.Contains(got, want) {
				t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
			}
		})
	}
}

func TestFetchAndInsertVersionTimeout(t *testing.T) {
	defer postgres.ResetTestDB(testDB, t)

	defer func(oldTimeout time.Duration) {
		fetchTimeout = oldTimeout
	}(fetchTimeout)
	fetchTimeout = 0

	teardownProxy, client := proxy.SetupTestProxy(t, nil)
	defer teardownProxy(t)

	name := "my.mod/version"
	version := "v1.0.0"
	wantErrString := "deadline exceeded"
	err := fetchAndInsertVersion(context.Background(), name, version, client, testDB)
	if err == nil || !strings.Contains(err.Error(), wantErrString) {
		t.Fatalf("fetchAndInsertVersion(%q, %q, %v, %v) returned error %v, want error containing %q",
			name, version, client, testDB, err, wantErrString)
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

			m, v, err := parseModulePathAndVersion(u.Path)
			if test.err != nil {
				if err == nil {
					t.Fatalf("parseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				}
				if test.err.Error() != err.Error() {
					t.Fatalf("parseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
				} else {
					return
				}
			} else if err != nil {
				t.Fatalf("parseModulePathAndVersion(%v) error = (%v); want = (%v)", u, err, test.err)
			}

			if test.module != m || test.version != v {
				t.Fatalf("parseModulePathAndVersion(%v): %q, %q, %v; want = %q, %q, %v",
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
			file:         "github.com/my/module@v1.0.0/README.md",
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
			file:         "github.com/my/module@v1.0.0/LICENSE",
			expectedFile: "github.com/my/module@v1.0.0/LICENSE",
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

func TestExtractReadmeFromZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, test := range []struct {
		name, version, file, wantPath string
		wantContents                  []byte
		err                           error
	}{
		{
			name:         "github.com/my/module",
			version:      "v1.0.0",
			file:         "github.com/my/module@v1.0.0/README.md",
			wantPath:     "README.md",
			wantContents: []byte("README FILE FOR TESTING."),
		},
		{
			name:    "emp.ty/module",
			version: "v1.0.0",
			err:     errReadmeNotFound,
		},
	} {
		t.Run(test.file, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(t, nil)
			defer teardownProxy(t)

			reader, err := client.GetZip(ctx, test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(ctx, %q, %q): %v", test.name, test.version, err)
			}

			gotPath, gotContents, err := extractReadmeFromZip(test.name, test.version, reader)
			if err != nil {
				if test.err == nil || test.err.Error() != err.Error() {
					t.Errorf("extractFile(%q, %q): \n %v, want \n %v",
						fmt.Sprintf("%q %q", test.name, test.version), filepath.Base(test.file), err, test.err)
				} else {
					return
				}
			}

			if test.wantPath != gotPath {
				t.Errorf("extractFile(%q, %q) path = %q, want %q", test.name, test.file, gotPath, test.wantPath)
			}
			if !bytes.Equal(test.wantContents, gotContents) {
				t.Errorf("extractFile(%q, %q) contents = %q, want %q", test.name, test.file, gotContents, test.wantContents)
			}
		})
	}
}

func TestExtractPackagesFromZip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	for _, test := range []struct {
		name     string
		version  string
		packages map[string]*internal.Package
	}{
		{
			name:    "github.com/my/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"foo": &internal.Package{
					Name:              "foo",
					Path:              "github.com/my/module/foo",
					Synopsis:          "package foo",
					DocumentationHTML: []byte("FooBar returns the string &#34;foo bar&#34;."),
					Imports:           []string{"fmt", "github.com/my/module/bar"},
					V1Path:            "github.com/my/module/foo",
				},
				"bar": &internal.Package{
					Name:              "bar",
					Path:              "github.com/my/module/bar",
					Synopsis:          "package bar",
					DocumentationHTML: []byte("Bar returns the string &#34;bar&#34;."),
					Imports:           []string{},
					V1Path:            "github.com/my/module/bar",
				},
			},
		},
		{
			name:    "no.mod/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"p": &internal.Package{
					Name:              "p",
					Path:              "no.mod/module/p",
					Synopsis:          "Package p is inside a module where a go.mod file hasn't been explicitly added yet.",
					DocumentationHTML: []byte("const Year = 2009"),
					Imports:           []string{},
					V1Path:            "no.mod/module/p",
				},
			},
		},
		{
			name:     "emp.ty/module",
			version:  "v1.0.0",
			packages: map[string]*internal.Package{},
		},
		{
			name:    "bad.mod/module",
			version: "v1.0.0",
			packages: map[string]*internal.Package{
				"good": &internal.Package{
					Name:              "good",
					Path:              "bad.mod/module/good",
					Synopsis:          "Package good is inside a module that has bad packages.",
					DocumentationHTML: []byte("const Good = true"),
					Imports:           []string{},
					V1Path:            "bad.mod/module/good",
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			teardownProxy, client := proxy.SetupTestProxy(t, nil)
			defer teardownProxy(t)

			reader, err := client.GetZip(ctx, test.name, test.version)
			if err != nil {
				t.Fatalf("client.GetZip(ctx, %q %q): %v", test.name, test.version, err)
			}

			packages, err := extractPackagesFromZip(test.name, test.version, reader, nil)
			if err != nil && len(test.packages) != 0 {
				t.Fatalf("extractPackagesFromZip(%q, %q, reader, nil): %v", test.name, test.version, err)
			}

			for _, got := range packages {
				want, ok := test.packages[got.Name]
				if !ok {
					t.Errorf("extractPackagesFromZip(%q, %q, reader, nil) returned unexpected package: %q", test.name, test.version, got.Name)
				}

				sort.Strings(got.Imports)

				if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(internal.Package{}, "DocumentationHTML")); diff != "" {
					t.Errorf("extractPackagesFromZip(%q, %q, reader, nil) mismatch (-want +got):\n%s", test.name, test.version, diff)
				}

				if got, want := got.DocumentationHTML, want.DocumentationHTML; len(want) == 0 && len(got) != 0 {
					t.Errorf("got non-empty documentation but want empty:\ngot: %q\nwant: %q", got, want)
				} else if !bytes.Contains(got, want) {
					t.Errorf("got documentation doesn't contain wanted documentation substring:\ngot: %q\nwant (substring): %q", got, want)
				}
			}
		})
	}
}

func TestFetch_parseVersionType(t *testing.T) {
	testCases := []struct {
		name, version   string
		wantVersionType internal.VersionType
		wantErr         bool
	}{
		{
			name:            "pseudo major version",
			version:         "v1.0.0-20190311183353-d8887717615a",
			wantVersionType: internal.VersionTypePseudo,
		},
		{
			name:            "pseudo prerelease version",
			version:         "v1.2.3-pre.0.20190311183353-d8887717615a",
			wantVersionType: internal.VersionTypePseudo,
		},
		{
			name:            "pseudo minor version",
			version:         "v1.2.4-0.20190311183353-d8887717615a",
			wantVersionType: internal.VersionTypePseudo,
		},
		{
			name:            "pseudo version invalid",
			version:         "v1.2.3-20190311183353-d8887717615a",
			wantVersionType: internal.VersionTypePrerelease,
		},
		{
			name:            "valid release",
			version:         "v1.0.0",
			wantVersionType: internal.VersionTypeRelease,
		},
		{
			name:            "valid prerelease",
			version:         "v1.0.0-alpha.1",
			wantVersionType: internal.VersionTypePrerelease,
		},
		{
			name:            "invalid version",
			version:         "not_a_version",
			wantVersionType: "",
			wantErr:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if gotVt, err := ParseVersionType(tc.version); (tc.wantErr == (err != nil)) && tc.wantVersionType != gotVt {
				t.Errorf("parseVersionType(%v) = %v, want %v", tc.version, gotVt, tc.wantVersionType)
			}
		})
	}
}
